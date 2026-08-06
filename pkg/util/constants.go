package util

const (
	// DefaultLauncherImage is the virt-launcher image used when none is specified.
	DefaultLauncherImage = "quay.io/kubevirt/virt-launcher:v1.9.0"

	// ContainerPrefix is prepended to VM names to identify podvirt-managed containers.
	ContainerPrefix = "podvirt-"
)
