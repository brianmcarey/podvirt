package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/brianmcarey/podvirt/pkg/config"
	"github.com/brianmcarey/podvirt/pkg/converter"
	"github.com/brianmcarey/podvirt/pkg/podman"
	"github.com/brianmcarey/podvirt/pkg/util"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/spf13/cobra"
)

type createOptions struct {
	configFile     string
	name           string
	memory         string
	diskSize       string
	disk           string
	image          string
	launcherImage  string
	password       string
	user           string
	cpus           int
	portStrs       []string
	sshKeyFiles    []string
	cpusChanged    bool
	memoryChanged  bool
	sshKeyExplicit bool
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new virtual machine",
	Long: `Create a new VM from a YAML config file or CLI flags.
The VM is created as a virt-launcher Podman container but not started.

Example:
  podvirt create --config vm.yaml
  podvirt create --name myvm --cpus 2 --memory 2Gi --disk /path/to/disk.qcow2
  podvirt create --name myvm --image quay.io/containerdisks/fedora:43 \
      --ssh-key ~/.ssh/id_ed25519.pub --port 2222:22`,
	RunE: runCreate,
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().StringP("config", "c", "", "Path to VM YAML config file")
	createCmd.Flags().String("name", "", "VM name")
	createCmd.Flags().Int("cpus", 1, "Number of vCPUs")
	createCmd.Flags().String("memory", "1Gi", "Memory size (e.g. 1Gi, 512Mi)")
	createCmd.Flags().String("disk", "", "Root disk image path")
	createCmd.Flags().String("disk-size", "", "Desired size for the root/first disk (e.g. 20Gi)")
	createCmd.Flags().String("image", "", "Container disk image (e.g. quay.io/containerdisks/fedora:43)")
	createCmd.Flags().String("launcher-image", util.DefaultLauncherImage, "virt-launcher container image to use")
	createCmd.Flags().StringArrayP("port", "p", nil, "Port forward: hostPort:vmPort[/proto] (e.g. 2222:22 or 8080:80/tcp)")
	createCmd.Flags().StringArray("ssh-key", nil, "SSH public key file to inject (auto-detects ~/.ssh/id_*.pub if not set)")
	createCmd.Flags().String("password", "", "Set user password via cloud-init")
	createCmd.Flags().String("user", "", "Username for cloud-init (e.g. fedora, ubuntu)")
}

func runCreate(cmd *cobra.Command, args []string) error {
	opts, err := parseCreateOptions(cmd)
	if err != nil {
		return err
	}

	vm, err := loadCreateVM(opts)
	if err != nil {
		return err
	}

	if err := applyCreatePorts(vm, opts.portStrs); err != nil {
		return err
	}

	if err := applyCloudInitFlags(vm, opts); err != nil {
		return err
	}
	if err := applyCreateDiskSize(vm, opts.diskSize); err != nil {
		return err
	}

	if err := extractContainerDisks(vm); err != nil {
		return err
	}

	if err := config.Validate(vm); err != nil {
		return err
	}

	client, err := newPodmanClient()
	if err != nil {
		return err
	}

	exists, err := client.ExistsVM(vm.Metadata.Name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("VM %q already exists (use 'podvirt delete %s' to remove it)", vm.Metadata.Name, vm.Metadata.Name)
	}

	fmt.Printf("Pulling virt-launcher image (if needed)...\n")
	if err := client.EnsureImage(opts.launcherImage); err != nil {
		return err
	}
	if err := prepareSizedDisks(client, vm, opts.launcherImage); err != nil {
		return err
	}

	result, err := converter.ToVMI(vm)
	if err != nil {
		return fmt.Errorf("converting VM config: %w", err)
	}

	fmt.Printf("Creating VM %q...\n", vm.Metadata.Name)
	id, err := client.CreateVM(vm.Metadata.Name, opts.launcherImage, result)
	if err != nil {
		return err
	}

	printCreateSummary(vm, result, id)
	return nil
}

func parseCreateOptions(cmd *cobra.Command) (*createOptions, error) {
	configFile, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, fmt.Errorf("reading --config: %w", err)
	}
	name, err := cmd.Flags().GetString("name")
	if err != nil {
		return nil, fmt.Errorf("reading --name: %w", err)
	}
	cpus, err := cmd.Flags().GetInt("cpus")
	if err != nil {
		return nil, fmt.Errorf("reading --cpus: %w", err)
	}
	memory, err := cmd.Flags().GetString("memory")
	if err != nil {
		return nil, fmt.Errorf("reading --memory: %w", err)
	}
	diskSize, err := cmd.Flags().GetString("disk-size")
	if err != nil {
		return nil, fmt.Errorf("reading --disk-size: %w", err)
	}
	disk, err := cmd.Flags().GetString("disk")
	if err != nil {
		return nil, fmt.Errorf("reading --disk: %w", err)
	}
	image, err := cmd.Flags().GetString("image")
	if err != nil {
		return nil, fmt.Errorf("reading --image: %w", err)
	}
	launcherImage, err := cmd.Flags().GetString("launcher-image")
	if err != nil {
		return nil, fmt.Errorf("reading --launcher-image: %w", err)
	}
	portStrs, err := cmd.Flags().GetStringArray("port")
	if err != nil {
		return nil, fmt.Errorf("reading --port: %w", err)
	}
	sshKeyFiles, err := cmd.Flags().GetStringArray("ssh-key")
	if err != nil {
		return nil, fmt.Errorf("reading --ssh-key: %w", err)
	}
	password, err := cmd.Flags().GetString("password")
	if err != nil {
		return nil, fmt.Errorf("reading --password: %w", err)
	}
	user, err := cmd.Flags().GetString("user")
	if err != nil {
		return nil, fmt.Errorf("reading --user: %w", err)
	}

	return &createOptions{
		configFile:     configFile,
		name:           name,
		memory:         memory,
		diskSize:       diskSize,
		disk:           disk,
		image:          image,
		launcherImage:  launcherImage,
		password:       password,
		user:           user,
		cpus:           cpus,
		portStrs:       portStrs,
		sshKeyFiles:    sshKeyFiles,
		cpusChanged:    cmd.Flags().Changed("cpus"),
		memoryChanged:  cmd.Flags().Changed("memory"),
		sshKeyExplicit: cmd.Flags().Changed("ssh-key"),
	}, nil
}

func applyCreateDiskSize(vm *config.VirtualMachine, diskSize string) error {
	if diskSize == "" {
		return nil
	}
	if len(vm.Spec.Disks) == 0 {
		return fmt.Errorf("--disk-size requires a root disk from --disk, --image, or config")
	}
	vm.Spec.Disks[0].Size = diskSize
	return nil
}

func loadCreateVM(opts *createOptions) (*config.VirtualMachine, error) {
	if opts.configFile != "" {
		vm, err := config.LoadFromFile(opts.configFile)
		if err != nil {
			return nil, err
		}

		cpusOverride := 0
		if opts.cpusChanged {
			cpusOverride = opts.cpus
		}
		memOverride := ""
		if opts.memoryChanged {
			memOverride = opts.memory
		}
		config.MergeFlags(vm, opts.name, memOverride, cpusOverride, opts.disk, opts.image)
		return vm, nil
	}

	return config.LoadFromFlags(opts.name, opts.memory, opts.cpus, opts.disk, opts.image)
}

func applyCreatePorts(vm *config.VirtualMachine, portStrs []string) error {
	if len(portStrs) == 0 {
		return nil
	}

	pfs, err := parsePortForwards(portStrs)
	if err != nil {
		return err
	}
	applyPortForwards(vm, pfs)
	return nil
}

func applyCloudInitFlags(vm *config.VirtualMachine, opts *createOptions) error {
	sshKeys, err := resolveSSHKeys(opts.sshKeyFiles, opts.sshKeyExplicit)
	if err != nil {
		return err
	}
	if len(sshKeys) == 0 && opts.password == "" {
		return nil
	}

	if vm.Spec.CloudInit == nil {
		vm.Spec.CloudInit = &config.CloudInitSpec{}
	}
	vm.Spec.CloudInit.SSHKeys = append(vm.Spec.CloudInit.SSHKeys, sshKeys...)
	if opts.password != "" {
		vm.Spec.CloudInit.Password = opts.password
	} else if vm.Spec.CloudInit.Password == "" {
		vm.Spec.CloudInit.Password = config.DefaultCloudInitPassword
	}
	if opts.user != "" {
		vm.Spec.CloudInit.User = opts.user
	} else if vm.Spec.CloudInit.User == "" {
		vm.Spec.CloudInit.User = config.DefaultCloudInitUser
	}
	return nil
}

func printCreateSummary(vm *config.VirtualMachine, result *converter.VMIResult, id string) {
	fmt.Printf("VM %q created (container ID: %s)\n", vm.Metadata.Name, id[:12])
	for _, disk := range vm.Spec.Disks {
		if disk.Size != "" {
			fmt.Printf("  Disk %q size: %s\n", disk.Name, disk.Size)
		}
	}
	if len(result.PortForwards) > 0 {
		for _, pf := range result.PortForwards {
			proto := pf.Protocol
			if proto == "" {
				proto = "tcp"
			}
			fmt.Printf("  Port forward: localhost:%d → VM:%d/%s\n", pf.HostPort, pf.VMPort, proto)
		}
	}
	fmt.Printf("Start it with: podvirt start %s\n", vm.Metadata.Name)
}

type qemuImgInfo struct {
	Format      string `json:"format"`
	VirtualSize uint64 `json:"virtual-size"`
}

func prepareSizedDisks(client *podman.Client, vm *config.VirtualMachine, launcherImage string) error {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("finding user cache dir: %w", err)
	}
	baseDir := filepath.Join(cacheDir, "podvirt", "resized-disks", vm.Metadata.Name)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return fmt.Errorf("creating resized disk cache dir: %w", err)
	}

	for i := range vm.Spec.Disks {
		disk := &vm.Spec.Disks[i]
		if disk.Size == "" {
			continue
		}
		if disk.Source.Image == "" {
			return fmt.Errorf("disk %q: sizing requires a local image path after container disk extraction", disk.Name)
		}
		sizedPath, err := buildSizedDiskImage(client, launcherImage, baseDir, disk)
		if err != nil {
			return err
		}
		disk.Source.Image = sizedPath
	}
	return nil
}

func buildSizedDiskImage(client *podman.Client, launcherImage, cacheDir string, disk *config.DiskSpec) (string, error) {
	sizeBytes, err := config.ParseQuantityBytes(disk.Size)
	if err != nil {
		return "", fmt.Errorf("disk %q: parsing size %q: %w", disk.Name, disk.Size, err)
	}

	info, err := inspectDiskImage(client, launcherImage, disk.Source.Image)
	if err != nil {
		return "", fmt.Errorf("disk %q: inspecting source image: %w", disk.Name, err)
	}
	if sizeBytes < info.VirtualSize {
		return "", fmt.Errorf("disk %q: requested size %s is smaller than source image virtual size", disk.Name, disk.Size)
	}

	destPath := filepath.Join(cacheDir, disk.Name+".qcow2")
	if err := os.RemoveAll(destPath); err != nil {
		return "", fmt.Errorf("disk %q: removing previous resized disk: %w", disk.Name, err)
	}

	fmt.Printf("Preparing resized disk %q (%s)...\n", disk.Name, disk.Size)
	if err := runQemuImg(client, launcherImage, disk.Source.Image, destPath, "convert", "-U", "-f", info.Format, "-O", "qcow2", sourceMountPath(disk.Source.Image), destMountPath(destPath)); err != nil {
		return "", fmt.Errorf("disk %q: copying source image for resize: %w", disk.Name, err)
	}
	if sizeBytes > info.VirtualSize {
		if err := runQemuImg(client, launcherImage, disk.Source.Image, destPath, "resize", destMountPath(destPath), strconv.FormatUint(sizeBytes, 10)); err != nil {
			return "", fmt.Errorf("disk %q: resizing copied image: %w", disk.Name, err)
		}
	}
	return destPath, nil
}

func inspectDiskImage(client *podman.Client, launcherImage, sourcePath string) (*qemuImgInfo, error) {
	out, err := runQemuImgOutput(client, launcherImage, sourcePath, sourcePath, "info", "-U", "--output=json", sourceMountPath(sourcePath))
	if err != nil {
		return nil, err
	}
	var info qemuImgInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("parsing qemu-img info output: %w", err)
	}
	if info.Format == "" || info.VirtualSize == 0 {
		return nil, fmt.Errorf("unexpected qemu-img info output")
	}
	return &info, nil
}

func runQemuImg(client *podman.Client, launcherImage, sourcePath, destPath string, qemuArgs ...string) error {
	_, err := runQemuImgOutput(client, launcherImage, sourcePath, destPath, qemuArgs...)
	return err
}

func runQemuImgOutput(client *podman.Client, launcherImage, sourcePath, destPath string, qemuArgs ...string) ([]byte, error) {
	sourceDir := filepath.Dir(sourcePath)
	destDir := filepath.Dir(destPath)
	mounts := []spec.Mount{{
		Type:        "bind",
		Source:      sourceDir,
		Destination: "/mnt/src",
		Options:     []string{"bind", "ro", "z"},
	}, {
		Type:        "bind",
		Source:      destDir,
		Destination: "/mnt/dst",
		Options:     []string{"bind", "z"},
	}}

	stdout, stderr, err := client.RunOneShotContainer(launcherImage, mounts, []string{"qemu-img"}, qemuArgs)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = strings.TrimSpace(stdout)
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%v: %s", err, msg)
	}
	return []byte(stdout), nil
}

func sourceMountPath(sourcePath string) string {
	return filepath.Join("/mnt/src", filepath.Base(sourcePath))
}

func destMountPath(destPath string) string {
	return filepath.Join("/mnt/dst", filepath.Base(destPath))
}

func parsePortForwards(portStrs []string) ([]config.PortForward, error) {
	var pfs []config.PortForward
	for _, s := range portStrs {
		proto := "tcp"
		if idx := strings.LastIndex(s, "/"); idx >= 0 {
			proto = strings.ToLower(s[idx+1:])
			s = s[:idx]
		}
		parts := strings.SplitN(s, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --port %q: expected hostPort:vmPort[/proto]", s)
		}
		hostPort, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid --port %q: host port must be a number", s)
		}
		vmPort, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid --port %q: VM port must be a number", s)
		}
		pfs = append(pfs, config.PortForward{HostPort: hostPort, VMPort: vmPort, Protocol: proto})
	}
	return pfs, nil
}

func applyPortForwards(vm *config.VirtualMachine, pfs []config.PortForward) {
	for i := range vm.Spec.Networks {
		if vm.Spec.Networks[i].Type == "masquerade" {
			vm.Spec.Networks[i].PortForwards = append(vm.Spec.Networks[i].PortForwards, pfs...)
			return
		}
	}
	if len(vm.Spec.Networks) > 0 {
		vm.Spec.Networks[0].Type = "masquerade"
		vm.Spec.Networks[0].PortForwards = append(vm.Spec.Networks[0].PortForwards, pfs...)
	} else {
		vm.Spec.Networks = []config.NetworkSpec{{
			Name:         "default",
			Type:         "masquerade",
			PortForwards: pfs,
		}}
	}
}

func resolveSSHKeys(files []string, explicit bool) ([]string, error) {
	if len(files) == 0 && !explicit {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, nil
		}
		candidates := []string{
			home + "/.ssh/id_ed25519.pub",
			home + "/.ssh/id_rsa.pub",
			home + "/.ssh/id_ecdsa.pub",
		}
		for _, p := range candidates {
			data, err := os.ReadFile(p)
			if err == nil {
				key := strings.TrimSpace(string(data))
				if key != "" {
					fmt.Printf("Auto-injecting SSH key: %s\n", p)
					return []string{key}, nil
				}
			}
		}
		return nil, nil
	}

	var keys []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("reading SSH key file %q: %w", f, err)
		}
		keys = append(keys, strings.TrimSpace(string(data)))
	}
	return keys, nil
}

func extractContainerDisks(vm *config.VirtualMachine) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home dir: %w", err)
	}
	cacheBase := filepath.Join(home, ".cache", "podvirt", "containerdisks")

	for i, disk := range vm.Spec.Disks {
		if disk.Source.ContainerImage == "" {
			continue
		}
		image := disk.Source.ContainerImage
		sanitized := strings.NewReplacer("/", "_", ":", "_", "@", "_").Replace(image)
		destDir := filepath.Join(cacheBase, sanitized)

		diskFile, err := findExtractedDisk(destDir)
		if err != nil {
			return err
		}
		if diskFile != "" {
			fmt.Printf("Using cached containerdisk: %s\n", diskFile)
		} else {
			diskFile, err = pullAndExtract(image, destDir)
			if err != nil {
				return err
			}
		}

		vm.Spec.Disks[i].Source.Image = diskFile
		vm.Spec.Disks[i].Source.ContainerImage = ""
	}
	return nil
}

func pullAndExtract(image, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("creating containerdisk cache dir: %w", err)
	}

	fmt.Printf("Pulling containerdisk image %s...\n", image)
	pull := exec.Command("podman", "pull", image)
	pull.Stdout = os.Stdout
	pull.Stderr = os.Stderr
	if err := pull.Run(); err != nil {
		return "", fmt.Errorf("pulling containerdisk %q: %w", image, err)
	}

	fmt.Printf("Extracting disk from %s...\n", image)
	create := exec.Command("podman", "create", image)
	out, err := create.Output()
	if err != nil {
		return "", fmt.Errorf("creating temp container from %q: %w", image, err)
	}
	cid := strings.TrimSpace(string(out))

	defer func() {
		if err := exec.Command("podman", "rm", cid).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cleaning up temporary container %q failed: %v\n", cid, err)
		}
	}()

	cp := exec.Command("podman", "cp", cid+":/disk/.", destDir)
	cp.Stdout = os.Stdout
	cp.Stderr = os.Stderr
	if err := cp.Run(); err != nil {
		return "", fmt.Errorf("extracting disk from containerdisk %q: %w", image, err)
	}

	diskFile, err := findExtractedDisk(destDir)
	if err != nil {
		return "", err
	}
	if diskFile == "" {
		return "", fmt.Errorf("no disk file found after extracting containerdisk %q (expected disk.img or disk.qcow2 in %s)", image, destDir)
	}
	fmt.Printf("Containerdisk extracted to: %s\n", diskFile)
	return diskFile, nil
}

func findExtractedDisk(dir string) (string, error) {
	imgPath := filepath.Join(dir, "disk.img")
	qcow2Path := filepath.Join(dir, "disk.qcow2")

	if _, err := os.Stat(qcow2Path); err == nil {
		return qcow2Path, nil
	}
	if _, err := os.Stat(imgPath); err == nil {
		if util.IsQcow2Image(imgPath) {
			if err := os.Rename(imgPath, qcow2Path); err != nil {
				return "", fmt.Errorf("renaming extracted qcow2 image %q to %q: %w", imgPath, qcow2Path, err)
			}
			return qcow2Path, nil
		}
		return imgPath, nil
	}
	return "", nil
}
