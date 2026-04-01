package util

const (
	// DefaultLauncherImage is the virt-launcher image used when none is specified.
	DefaultLauncherImage = "quay.io/kubevirt/virt-launcher:v1.7.0"

	// ContainerPrefix is prepended to VM names to identify podvirt-managed containers.
	ContainerPrefix = "podvirt-"
)
