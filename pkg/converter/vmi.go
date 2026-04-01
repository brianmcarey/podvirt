package converter

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/brianmcarey/podvirt/pkg/config"
	"github.com/brianmcarey/podvirt/pkg/util"
	"gopkg.in/yaml.v3"
)

const virtLauncherDiskBase = "/var/run/kubevirt-private/vmi-disks"

// VMIResult holds the JSON-encoded VMI spec and the set of volume mounts
// (host path → container path) required to make disk images available.
type VMIResult struct {
	VMIJSON           string
	HostMounts        map[string]string
	PortForwards      []config.PortForward
	UID               string
	SSHUser           string
	SSHKeysConfigured bool
}

func ToVMI(vm *config.VirtualMachine) (*VMIResult, error) {
	vmi, mounts, portForwards, err := buildVMI(vm)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(vmi)
	if err != nil {
		return nil, fmt.Errorf("marshalling VMI spec: %w", err)
	}

	sshUser := ""
	sshKeysConfigured := false
	if vm.Spec.CloudInit != nil {
		sshUser = effectiveCloudInitUser(vm.Spec.CloudInit)
		sshKeysConfigured = len(vm.Spec.CloudInit.SSHKeys) > 0
	}

	return &VMIResult{
		VMIJSON:           string(data),
		HostMounts:        mounts,
		PortForwards:      portForwards,
		UID:               vmUID(vm.Metadata.Name),
		SSHUser:           sshUser,
		SSHKeysConfigured: sshKeysConfigured,
	}, nil
}

type vmiObject struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Metadata   vmiMetadata `json:"metadata"`
	Spec       vmiSpec     `json:"spec"`
}

type vmiMetadata struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	UID         string            `json:"uid"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type vmiSpec struct {
	Domain   vmiDomain    `json:"domain"`
	Volumes  []vmiVolume  `json:"volumes,omitempty"`
	Networks []vmiNetwork `json:"networks,omitempty"`
}

type vmiDomain struct {
	CPU       vmiCPU       `json:"cpu"`
	Resources vmiResources `json:"resources"`
	Devices   vmiDevices   `json:"devices"`
	Firmware  *vmiFirmware `json:"firmware,omitempty"`
}

type vmiCPU struct {
	Cores   uint32 `json:"cores"`
	Sockets uint32 `json:"sockets,omitempty"`
	Threads uint32 `json:"threads,omitempty"`
}

type vmiResources struct {
	Requests map[string]string `json:"requests"`
}

type vmiDevices struct {
	Disks                    []vmiDisk      `json:"disks,omitempty"`
	Interfaces               []vmiInterface `json:"interfaces,omitempty"`
	AutoattachGraphicsDevice *bool          `json:"autoattachGraphicsDevice,omitempty"`
}

type vmiDisk struct {
	Name string        `json:"name"`
	Disk vmiDiskTarget `json:"disk"`
}

type vmiDiskTarget struct {
	Bus string `json:"bus"`
}

type vmiInterface struct {
	Name    string      `json:"name"`
	Binding *vmiBinding `json:"binding,omitempty"`
	Ports   []vmiPort   `json:"ports,omitempty"`
}

type vmiBinding struct {
	Name string `json:"name"`
}

type vmiEmptyStruct struct{}

type vmiPort struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol,omitempty"`
}

type vmiVolume struct {
	Name             string               `json:"name"`
	HostDisk         *vmiHostDisk         `json:"hostDisk,omitempty"`
	ContainerDisk    *vmiContainerDisk    `json:"containerDisk,omitempty"`
	CloudInitNoCloud *vmiCloudInitNoCloud `json:"cloudInitNoCloud,omitempty"`
}

type vmiHostDisk struct {
	Path     string `json:"path"`
	Type     string `json:"type"`
	Capacity string `json:"capacity,omitempty"`
}

type vmiContainerDisk struct {
	Image           string `json:"image"`
	ImagePullPolicy string `json:"imagePullPolicy,omitempty"`
}

type vmiCloudInitNoCloud struct {
	UserData string `json:"userData"`
}

type vmiNetwork struct {
	Name string          `json:"name"`
	Pod  *vmiEmptyStruct `json:"pod,omitempty"`
}

type vmiFirmware struct {
	Bootloader *vmiBootloader `json:"bootloader,omitempty"`
}

type vmiBootloader struct {
	Kernel  string `json:"kernel,omitempty"`
	Initrd  string `json:"initrd,omitempty"`
	Cmdline string `json:"cmdline,omitempty"`
}

func vmUID(name string) string {
	h := md5.Sum([]byte("podvirt:" + name))
	h[6] = (h[6] & 0x0f) | 0x30
	h[8] = (h[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

func buildVMI(vm *config.VirtualMachine) (*vmiObject, map[string]string, []config.PortForward, error) {
	mounts := make(map[string]string)

	cpu := vmiCPU{Cores: vm.Spec.CPU.Cores}
	if vm.Spec.CPU.Sockets > 0 {
		cpu.Sockets = vm.Spec.CPU.Sockets
	}
	if vm.Spec.CPU.Threads > 0 {
		cpu.Threads = vm.Spec.CPU.Threads
	}

	var disks []vmiDisk
	var volumes []vmiVolume

	for _, d := range vm.Spec.Disks {
		bus := d.Bus
		if bus == "" {
			bus = "virtio"
		}
		disks = append(disks, vmiDisk{
			Name: d.Name,
			Disk: vmiDiskTarget{Bus: bus},
		})

		switch {
		case d.Source.Image != "":
			containerPath := filepath.Join(virtLauncherDiskBase, d.Name, containerDiskFilename(d.Source.Image))
			mounts[d.Source.Image] = containerPath
			volumes = append(volumes, vmiVolume{
				Name: d.Name,
				HostDisk: &vmiHostDisk{
					Path: containerPath,
					Type: "DiskOrCreate",
				},
			})
		default:
			return nil, nil, nil, fmt.Errorf("disk %q has no source", d.Name)
		}
	}

	if ci := vm.Spec.CloudInit; ci != nil {
		userData, err := buildUserData(ci)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("building cloud-init user data: %w", err)
		}
		disks = append(disks, vmiDisk{
			Name: "cloudinitdisk",
			Disk: vmiDiskTarget{Bus: "virtio"},
		})
		volumes = append(volumes, vmiVolume{
			Name:             "cloudinitdisk",
			CloudInitNoCloud: &vmiCloudInitNoCloud{UserData: userData},
		})
	}

	var interfaces []vmiInterface
	var networks []vmiNetwork
	var portForwards []config.PortForward

	for _, n := range vm.Spec.Networks {
		iface := vmiInterface{
			Name:    n.Name,
			Binding: &vmiBinding{Name: "passt"},
		}
		for _, pf := range n.PortForwards {
			proto := pf.Protocol
			if proto == "" {
				proto = "TCP"
			}
			iface.Ports = append(iface.Ports, vmiPort{
				Port:     pf.VMPort,
				Protocol: strings.ToUpper(proto),
			})
			portForwards = append(portForwards, pf)
		}
		interfaces = append(interfaces, iface)
		networks = append(networks, vmiNetwork{Name: n.Name, Pod: &vmiEmptyStruct{}})
	}

	vmi := &vmiObject{
		APIVersion: "kubevirt.io/v1",
		Kind:       "VirtualMachineInstance",
		Metadata: vmiMetadata{
			Name:      vm.Metadata.Name,
			Namespace: "default",
			UID:       vmUID(vm.Metadata.Name),
			// placePCIDevicesOnRootComplex=true causes virt-launcher's
			// calculateHotplugPortCount to return 0, which skips the
			// two-call virDomainDefineXML path in WithNetworkIfacesResources.
			// Without this, the second call gets a new random UUID from libvirt
			// (the XML has no <uuid> element) and fails with "domain already exists".
			Annotations: map[string]string{
				"kubevirt.io/placePCIDevicesOnRootComplex": "true",
			},
		},
		Spec: vmiSpec{
			Domain: vmiDomain{
				CPU: cpu,
				Resources: vmiResources{
					Requests: map[string]string{"memory": vm.Spec.Memory},
				},
				Devices: vmiDevices{
					Disks:                    disks,
					Interfaces:               interfaces,
					AutoattachGraphicsDevice: boolPtr(false),
				},
				Firmware: bootFirmware(vm.Spec.Boot),
			},
			Volumes:  volumes,
			Networks: networks,
		},
	}

	return vmi, mounts, portForwards, nil
}

func containerDiskFilename(hostPath string) string {
	base := filepath.Base(hostPath)
	if !util.IsQcow2Image(hostPath) || strings.EqualFold(filepath.Ext(base), ".qcow2") {
		return base
	}

	ext := filepath.Ext(base)
	if ext == "" {
		return base + ".qcow2"
	}
	return strings.TrimSuffix(base, ext) + ".qcow2"
}

func bootFirmware(boot *config.BootSpec) *vmiFirmware {
	if boot == nil || (boot.Kernel == "" && boot.Initrd == "" && boot.Cmdline == "") {
		return nil
	}
	return &vmiFirmware{
		Bootloader: &vmiBootloader{
			Kernel:  boot.Kernel,
			Initrd:  boot.Initrd,
			Cmdline: boot.Cmdline,
		},
	}
}

type cloudConfig struct {
	Users     []cloudUser `yaml:"users,omitempty"`
	SSHPwauth bool        `yaml:"ssh_pwauth,omitempty"`
}

type cloudUser struct {
	Name              string   `yaml:"name"`
	SSHAuthorizedKeys []string `yaml:"ssh_authorized_keys,omitempty"`
	PlainTextPasswd   string   `yaml:"plain_text_passwd,omitempty"`
	LockPasswd        *bool    `yaml:"lock_passwd,omitempty"`
	Sudo              string   `yaml:"sudo,omitempty"`
}

func buildUserData(ci *config.CloudInitSpec) (string, error) {
	cc := cloudConfig{}
	cu := cloudUser{Name: effectiveCloudInitUser(ci)}
	password := effectiveCloudInitPassword(ci)

	if password != "" {
		cu.PlainTextPasswd = password
		cu.LockPasswd = boolPtr(false)
		cu.Sudo = "ALL=(ALL) ALL"
		cc.SSHPwauth = true
	}

	if len(ci.SSHKeys) > 0 {
		cu.SSHAuthorizedKeys = ci.SSHKeys
	}
	cc.Users = []cloudUser{cu}

	out, err := yaml.Marshal(cc)
	if err != nil {
		return "", fmt.Errorf("marshalling cloud-init config: %w", err)
	}
	return "#cloud-config\n" + string(out), nil
}

func boolPtr(b bool) *bool { return &b }

func effectiveCloudInitUser(ci *config.CloudInitSpec) string {
	if ci != nil && ci.User != "" {
		return ci.User
	}
	return config.DefaultCloudInitUser
}

func effectiveCloudInitPassword(ci *config.CloudInitSpec) string {
	if ci != nil && ci.Password != "" {
		return ci.Password
	}
	return config.DefaultCloudInitPassword
}
