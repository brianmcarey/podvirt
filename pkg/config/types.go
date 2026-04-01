package config

const (
	DefaultCloudInitUser     = "podvirt"
	DefaultCloudInitPassword = "podvirt"
)

type VirtualMachine struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       VMSpec   `yaml:"spec"`
}

type Metadata struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

type VMSpec struct {
	CPU       CPUSpec        `yaml:"cpu"`
	Memory    string         `yaml:"memory"`
	Disks     []DiskSpec     `yaml:"disks,omitempty"`
	Networks  []NetworkSpec  `yaml:"networks,omitempty"`
	Boot      *BootSpec      `yaml:"boot,omitempty"`
	Console   *ConsoleSpec   `yaml:"console,omitempty"`
	CloudInit *CloudInitSpec `yaml:"cloudInit,omitempty"`
}

type CPUSpec struct {
	Cores   uint32 `yaml:"cores"`
	Sockets uint32 `yaml:"sockets,omitempty"`
	Threads uint32 `yaml:"threads,omitempty"`
}

type DiskSpec struct {
	Name   string     `yaml:"name"`
	Source DiskSource `yaml:"source"`
	Size   string     `yaml:"size,omitempty"`
	Bus    string     `yaml:"bus,omitempty"`
}

type DiskSource struct {
	// Image is a path to a local disk image file (raw or qcow2).
	Image string `yaml:"image,omitempty"`
	// ContainerImage is a container registry URL for a container disk.
	ContainerImage string `yaml:"containerImage,omitempty"`
}

type NetworkSpec struct {
	Name         string        `yaml:"name"`
	Type         string        `yaml:"type,omitempty"`
	PortForwards []PortForward `yaml:"portForwards,omitempty"`
}

type PortForward struct {
	HostPort int    `yaml:"hostPort"`
	VMPort   int    `yaml:"vmPort"`
	Protocol string `yaml:"protocol,omitempty"`
}

type BootSpec struct {
	Kernel  string `yaml:"kernel,omitempty"`
	Initrd  string `yaml:"initrd,omitempty"`
	Cmdline string `yaml:"cmdline,omitempty"`
}

type ConsoleSpec struct {
	Type string `yaml:"type,omitempty"`
	Port int    `yaml:"port,omitempty"`
}

type CloudInitSpec struct {
	// Defaults to "podvirt" if empty.
	User string `yaml:"user,omitempty"`
	// Defaults to "podvirt" if empty.
	Password string   `yaml:"password,omitempty"`
	SSHKeys  []string `yaml:"sshKeys,omitempty"`
}
