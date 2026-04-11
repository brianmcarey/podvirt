package podman

import (
	"archive/tar"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/brianmcarey/podvirt/pkg/converter"
	"github.com/brianmcarey/podvirt/pkg/util"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/specgen"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	nettypes "go.podman.io/common/libnetwork/types"
)

//go:embed scripts/qemu-wrapper.sh
var qemuWrapper string

// qemuConf is written to /etc/libvirt/qemu.conf inside the container.
// It preserves all settings from the virt-launcher image's qemu.conf and
// adds podvirt-specific overrides.
const qemuConf = `# podvirt-managed qemu.conf
# Base settings preserved from quay.io/kubevirt/virt-launcher:v1.8.1
stdio_handler = "logd"
vnc_listen = "0.0.0.0"
vnc_tls = 0
vnc_sasl = 0
namespaces = [ ]
cgroup_controllers = [ ]

user = "root"
group = "root"
dynamic_ownership = 0
remember_owner = 0
`

// writeQemuSupport creates the QEMU support directory containing:
//   - qemu-kvm-wrapper: shell script bind-mounted over /usr/libexec/qemu-kvm;
//   - qemu.conf: libvirt qemu driver config (user/group/security settings)
//
// Also creates a persistent capability cache directory (~/.cache/podvirt/qemu-caps)
// which is bind-mounted at /var/cache/libvirt/qemu/capabilities/ to skip QEMU
// capability probes on repeat runs (each probe takes ~40s in D-state).
func writeQemuSupport() (string, string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", "", fmt.Errorf("finding user cache dir: %w", err)
	}
	supportDir := filepath.Join(cacheDir, "podvirt", "qemu-support")
	if err := os.MkdirAll(supportDir, 0755); err != nil {
		return "", "", fmt.Errorf("creating QEMU support dir: %w", err)
	}
	capsDir := filepath.Join(cacheDir, "podvirt", "qemu-caps")
	if err := os.MkdirAll(capsDir, 0755); err != nil {
		return "", "", fmt.Errorf("creating QEMU capabilities cache dir: %w", err)
	}

	wrapperPath := filepath.Join(supportDir, "qemu-kvm-wrapper")
	existing, readErr := os.ReadFile(wrapperPath)
	if readErr != nil || string(existing) != qemuWrapper {
		if err := os.WriteFile(wrapperPath, []byte(qemuWrapper), 0755); err != nil {
			return "", "", fmt.Errorf("writing QEMU wrapper: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(supportDir, "qemu.conf"), []byte(qemuConf), 0644); err != nil {
		return "", "", fmt.Errorf("writing qemu.conf: %w", err)
	}
	return supportDir, capsDir, nil
}

// Only used when PODVIRT_DEBUG=1.
func libvirtLogsDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cacheDir, "podvirt", "libvirt-logs")
	return dir, os.MkdirAll(dir, 0755)
}

type VMInfo struct {
	Name              string
	ContainerID       string
	State             string
	Image             string
	SSHUser           string
	SSHKeysConfigured bool
	PortMappings      []nettypes.PortMapping
}

// ContainerName returns the Podman container name for a given VM name.
func ContainerName(vmName string) string {
	return util.ContainerPrefix + vmName
}

// ensureQemuBinary extracts /usr/libexec/qemu-kvm from the launcher image into
// supportDir/qemu-kvm-real if it is not already present.
func (c *Client) ensureQemuBinary(launcherImage, supportDir string) error {
	realBinaryPath := filepath.Join(supportDir, "qemu-kvm-real")
	imageStampPath := filepath.Join(supportDir, "qemu-kvm-real.image")

	// Re-extract if the binary is missing or was extracted from a different image.
	if stamp, err := os.ReadFile(imageStampPath); err == nil && string(stamp) == launcherImage {
		if _, err := os.Stat(realBinaryPath); err == nil {
			return nil
		}
	}
	os.Remove(realBinaryPath)
	if capsDir, err := os.UserCacheDir(); err == nil {
		capsCacheDir := filepath.Join(capsDir, "podvirt", "qemu-caps")
		if entries, err := os.ReadDir(capsCacheDir); err == nil {
			for _, e := range entries {
				if filepath.Ext(e.Name()) == ".xml" {
					os.Remove(filepath.Join(capsCacheDir, e.Name()))
				}
			}
		}
	}

	s := &specgen.SpecGenerator{
		ContainerBasicConfig: specgen.ContainerBasicConfig{
			Command: []string{"true"},
		},
		ContainerStorageConfig: specgen.ContainerStorageConfig{
			Image: launcherImage,
		},
		ContainerHealthCheckConfig: specgen.ContainerHealthCheckConfig{
			HealthLogDestination: "local",
		},
	}
	resp, err := containers.CreateWithSpec(c.ctx, s, nil)
	if err != nil {
		return fmt.Errorf("creating temporary container to extract QEMU binary: %w", err)
	}
	defer func() {
		opts := &containers.RemoveOptions{}
		opts.WithForce(true)
		containers.Remove(c.ctx, resp.ID, opts) //nolint:errcheck
	}()

	pr, pw := io.Pipe()
	copyFunc, err := containers.CopyToArchive(c.ctx, resp.ID, "/usr/libexec/qemu-kvm", pw)
	if err != nil {
		pr.Close()
		pw.Close()
		return fmt.Errorf("copying QEMU binary from image: %w", err)
	}
	go func() {
		pw.CloseWithError(copyFunc())
	}()
	defer pr.Close()

	tr := tar.NewReader(pr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading QEMU binary archive: %w", err)
		}
		if filepath.Base(hdr.Name) == "qemu-kvm" {
			f, err := os.OpenFile(realBinaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return fmt.Errorf("creating qemu-kvm-real: %w", err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				os.Remove(realBinaryPath)
				return fmt.Errorf("writing qemu-kvm-real: %w", err)
			}
			f.Close()
			if err := os.WriteFile(imageStampPath, []byte(launcherImage), 0644); err != nil {
				return fmt.Errorf("writing qemu-kvm-real image stamp: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("qemu-kvm not found in image %s", launcherImage)
}

func (c *Client) CreateVM(vmName, launcherImage string, result *converter.VMIResult) (string, error) {
	qemuSupportDir, capsDir, logsDir, err := c.restoreRuntimeSupport(launcherImage, os.Getenv("PODVIRT_DEBUG") == "1")
	if err != nil {
		return "", fmt.Errorf("setting up runtime support for VM %q: %w", vmName, err)
	}

	privileged := true
	s := &specgen.SpecGenerator{
		ContainerBasicConfig: specgen.ContainerBasicConfig{
			Name: ContainerName(vmName),
			Env: map[string]string{
				"STANDALONE_VMI": result.VMIJSON,
				"PODVIRT_DEBUG":  os.Getenv("PODVIRT_DEBUG"),
			},
			Labels: map[string]string{
				"app":              "podvirt",
				"podvirt/vm":       vmName,
				"podvirt/ssh-user": result.SSHUser,
				"podvirt/ssh-keys": strconv.FormatBool(result.SSHKeysConfigured),
			},
			// Pass --uid so virt-launcher creates /var/run/kubevirt-private/<uid>/
			// before QEMU starts binding serial/VNC sockets inside that directory.
			Command: []string{
				"--uid", result.UID,
				"--name", vmName,
				"--namespace", "default",
			},
			// passt requires two sysctls that the KubeVirt passt CNI plugin
			// normally sets in the pod's network namespace:
			//   ip_unprivileged_port_start=0  lets passt (UID 107) bind to port 22
			//   ping_group_range="107 107"    lets the virt-launcher user send ICMP
			Sysctl: map[string]string{
				"net.ipv4.ip_unprivileged_port_start": "0",
				"net.ipv4.ping_group_range":           "107 107",
			},
		},
		ContainerStorageConfig: specgen.ContainerStorageConfig{
			Image:  launcherImage,
			Mounts: buildMounts(result.HostMounts, qemuSupportDir, capsDir, logsDir),
			Devices: []spec.LinuxDevice{
				{Path: "/dev/kvm", Type: "c"},
				{Path: "/dev/net/tun", Type: "c"},
			},
		},
		ContainerSecurityConfig: specgen.ContainerSecurityConfig{
			Privileged: &privileged,
			CapAdd: []string{
				"NET_ADMIN",
				"SYS_ADMIN",
			},
		},
		ContainerCgroupConfig: specgen.ContainerCgroupConfig{
			CgroupNS: specgen.Namespace{NSMode: specgen.Host},
		},
		ContainerHealthCheckConfig: specgen.ContainerHealthCheckConfig{
			HealthLogDestination: "local",
		},
		ContainerNetworkConfig: specgen.ContainerNetworkConfig{
			PortMappings: buildPortMappings(result),
		},
	}

	resp, err := containers.CreateWithSpec(c.ctx, s, nil)
	if err != nil {
		return "", fmt.Errorf("creating container for VM %q: %w", vmName, err)
	}
	return resp.ID, nil
}

func (c *Client) StartVM(vmName string) error {
	info, err := containers.Inspect(c.ctx, ContainerName(vmName), nil)
	if err != nil {
		return fmt.Errorf("inspecting VM %q before start: %w", vmName, err)
	}
	launcherImage := info.Config.Image
	if launcherImage == "" {
		return fmt.Errorf("inspecting VM %q before start: container image is empty", vmName)
	}
	if _, _, _, err := c.restoreRuntimeSupport(launcherImage, false); err != nil {
		return fmt.Errorf("restoring runtime support for VM %q: %w", vmName, err)
	}
	if err := containers.Start(c.ctx, ContainerName(vmName), nil); err != nil {
		return fmt.Errorf("starting VM %q: %w", vmName, err)
	}
	return nil
}

func (c *Client) restoreRuntimeSupport(launcherImage string, includeLogs bool) (string, string, string, error) {
	qemuSupportDir, capsDir, err := writeQemuSupport()
	if err != nil {
		return "", "", "", err
	}
	if _, err := libvirtLogsDir(); err != nil {
		return "", "", "", err
	}
	logsDir := ""
	if includeLogs {
		logsDir, _ = libvirtLogsDir()
	}
	if err := c.ensureQemuBinary(launcherImage, qemuSupportDir); err != nil {
		return "", "", "", err
	}
	return qemuSupportDir, capsDir, logsDir, nil
}

func (c *Client) StopVM(vmName string, timeoutSecs uint) error {
	opts := &containers.StopOptions{}
	opts.WithTimeout(timeoutSecs)
	if err := containers.Stop(c.ctx, ContainerName(vmName), opts); err != nil {
		return fmt.Errorf("stopping VM %q: %w", vmName, err)
	}
	return nil
}

func (c *Client) RemoveVM(vmName string, force bool) error {
	opts := &containers.RemoveOptions{}
	opts.WithForce(force)
	if _, err := containers.Remove(c.ctx, ContainerName(vmName), opts); err != nil {
		return fmt.Errorf("removing VM %q: %w", vmName, err)
	}
	return nil
}

func (c *Client) InspectVM(vmName string) (*VMInfo, error) {
	opts := &containers.ListOptions{}
	opts.WithAll(true)
	opts.WithFilters(map[string][]string{
		"name": {ContainerName(vmName)},
	})

	list, err := containers.List(c.ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("inspecting VM %q: %w", vmName, err)
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("VM %q not found", vmName)
	}
	ct := list[0]
	return &VMInfo{
		Name:              vmName,
		ContainerID:       ct.ID,
		State:             ct.State,
		Image:             ct.Image,
		SSHUser:           ct.Labels["podvirt/ssh-user"],
		SSHKeysConfigured: ct.Labels["podvirt/ssh-keys"] == "true",
		PortMappings:      ct.Ports,
	}, nil
}

func (c *Client) ListVMs() ([]VMInfo, error) {
	all := true
	opts := &containers.ListOptions{}
	opts.WithAll(all)
	opts.WithFilters(map[string][]string{
		"label": {"app=podvirt"},
	})

	list, err := containers.List(c.ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("listing VMs: %w", err)
	}

	var vms []VMInfo
	for _, c := range list {
		vms = append(vms, VMInfo{
			Name:              vmNameFromContainer(c.Names),
			ContainerID:       c.ID,
			State:             c.State,
			Image:             c.Image,
			SSHUser:           c.Labels["podvirt/ssh-user"],
			SSHKeysConfigured: c.Labels["podvirt/ssh-keys"] == "true",
			PortMappings:      c.Ports,
		})
	}
	return vms, nil
}

func (c *Client) ExistsVM(vmName string) (bool, error) {
	exists, err := containers.Exists(c.ctx, ContainerName(vmName), nil)
	if err != nil {
		return false, fmt.Errorf("checking VM %q: %w", vmName, err)
	}
	return exists, nil
}

func buildMounts(hostMounts map[string]string, qemuSupportDir string, capsDir string, logsDir string) []spec.Mount {
	mounts := []spec.Mount{
		{
			Type:        "tmpfs",
			Destination: "/var/run/libvirt",
			Options:     []string{"rw", "size=67108864"}, // 64 MiB
		},
		// The support dir holds qemu-kvm-wrapper, qemu.conf, and qemu-kvm-real.
		{
			Type:        "bind",
			Source:      qemuSupportDir,
			Destination: "/usr/local/lib/podvirt",
			Options:     []string{"bind", "ro", "z"},
		},
		// Override qemu.conf so virtqemud uses root user, no security driver, etc.
		// Must be rw: virt-launcher appends hugepages/virtiofsd settings via O_APPEND.
		{
			Type:        "bind",
			Source:      filepath.Join(qemuSupportDir, "qemu.conf"),
			Destination: "/etc/libvirt/qemu.conf",
			Options:     []string{"bind", "z"},
		},
		// Override the QEMU binary with our wrapper. libvirt calls /usr/libexec/qemu-kvm
		// for both capability probes and domain launches;
		{
			Type:        "bind",
			Source:      filepath.Join(qemuSupportDir, "qemu-kvm-wrapper"),
			Destination: "/usr/libexec/qemu-kvm",
			Options:     []string{"bind", "z"},
		},
		// The real QEMU binary, placed in the same directory as the wrapper so
		// QEMU's argv[0]-based module-path relocation resolves correctly.
		{
			Type:        "bind",
			Source:      filepath.Join(qemuSupportDir, "qemu-kvm-real"),
			Destination: "/usr/libexec/qemu-kvm-real",
			Options:     []string{"bind", "ro", "z"},
		},
		// Persistent capability cache: libvirt probes QEMU capabilities once and
		// stores the result here.
		{
			Type:        "bind",
			Source:      capsDir,
			Destination: "/var/cache/libvirt/qemu/capabilities",
			Options:     []string{"bind", "z"},
		},
		// Expose libvirt's serial console log for host-side diagnostics.
		{
			Type:        "bind",
			Source:      logsDir,
			Destination: "/var/log/libvirt/qemu",
			Options:     []string{"bind", "z"},
		},
	}

	if logsDir == "" {
		mounts = mounts[:len(mounts)-1]
	}

	for hostPath, containerPath := range hostMounts {
		mounts = append(mounts, spec.Mount{
			Type:        "bind",
			Source:      hostPath,
			Destination: containerPath,
			Options:     []string{"bind", "z"},
		})
	}

	return mounts
}

// buildPortMappings converts VMIResult port forwards to Podman port mappings.
func buildPortMappings(result *converter.VMIResult) []nettypes.PortMapping {
	var pm []nettypes.PortMapping
	for _, pf := range result.PortForwards {
		proto := pf.Protocol
		if proto == "" {
			proto = "tcp"
		}
		pm = append(pm, nettypes.PortMapping{
			HostPort:      uint16(pf.HostPort),
			ContainerPort: uint16(pf.VMPort),
			Protocol:      strings.ToLower(proto),
		})
	}
	return pm
}

func vmNameFromContainer(names []string) string {
	for _, n := range names {
		n = strings.TrimPrefix(n, "/")
		if strings.HasPrefix(n, util.ContainerPrefix) {
			return strings.TrimPrefix(n, util.ContainerPrefix)
		}
	}
	return strings.Join(names, ",")
}
