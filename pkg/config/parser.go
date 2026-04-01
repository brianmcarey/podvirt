package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func LoadFromFile(path string) (*VirtualMachine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var vm VirtualMachine
	if err := yaml.Unmarshal(data, &vm); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	applyDefaults(&vm)

	return &vm, nil
}

func LoadFromFlags(name, memory string, cpus int, diskPath string, containerImage string) (*VirtualMachine, error) {
	if name == "" {
		return nil, fmt.Errorf("--name is required when not using --config")
	}

	vm := &VirtualMachine{
		APIVersion: "podvirt.io/v1alpha1",
		Kind:       "VirtualMachine",
		Metadata:   Metadata{Name: name},
		Spec: VMSpec{
			CPU:    CPUSpec{Cores: uint32(cpus)},
			Memory: memory,
		},
	}

	setLocalDisk(vm, diskPath)
	setContainerDisk(vm, containerImage)

	vm.Spec.Networks = []NetworkSpec{{Name: "default", Type: "masquerade"}}

	applyDefaults(vm)
	return vm, nil
}

func MergeFlags(vm *VirtualMachine, name, memory string, cpus int, diskPath string, containerImage string) {
	if name != "" {
		vm.Metadata.Name = name
	}
	if memory != "" {
		vm.Spec.Memory = memory
	}
	if cpus > 0 {
		vm.Spec.CPU.Cores = uint32(cpus)
	}
	setLocalDisk(vm, diskPath)
	setContainerDisk(vm, containerImage)
}

func setLocalDisk(vm *VirtualMachine, diskPath string) {
	if diskPath == "" {
		return
	}

	for i := range vm.Spec.Disks {
		if vm.Spec.Disks[i].Source.Image != "" {
			vm.Spec.Disks[i].Name = "disk0"
			vm.Spec.Disks[i].Source = DiskSource{Image: diskPath}
			vm.Spec.Disks[i].Bus = "virtio"
			return
		}
	}

	vm.Spec.Disks = append(vm.Spec.Disks, DiskSpec{
		Name:   "disk0",
		Source: DiskSource{Image: diskPath},
		Bus:    "virtio",
	})
}

func setContainerDisk(vm *VirtualMachine, containerImage string) {
	if containerImage == "" {
		return
	}

	for i := range vm.Spec.Disks {
		if vm.Spec.Disks[i].Name == "containerdisk" || vm.Spec.Disks[i].Source.ContainerImage != "" {
			vm.Spec.Disks[i].Name = "containerdisk"
			vm.Spec.Disks[i].Source = DiskSource{ContainerImage: containerImage}
			vm.Spec.Disks[i].Bus = "virtio"
			return
		}
	}

	vm.Spec.Disks = append(vm.Spec.Disks, DiskSpec{
		Name:   "containerdisk",
		Source: DiskSource{ContainerImage: containerImage},
		Bus:    "virtio",
	})
}

func applyDefaults(vm *VirtualMachine) {
	if vm.APIVersion == "" {
		vm.APIVersion = "podvirt.io/v1alpha1"
	}
	if vm.Kind == "" {
		vm.Kind = "VirtualMachine"
	}
	if vm.Spec.CPU.Cores == 0 {
		vm.Spec.CPU.Cores = 1
	}
	if vm.Spec.Memory == "" {
		vm.Spec.Memory = "1Gi"
	}
	for i := range vm.Spec.Disks {
		if vm.Spec.Disks[i].Bus == "" {
			vm.Spec.Disks[i].Bus = "virtio"
		}
	}
	if len(vm.Spec.Networks) == 0 {
		vm.Spec.Networks = []NetworkSpec{{Name: "default", Type: "masquerade"}}
	}
	if vm.Spec.Console == nil {
		vm.Spec.Console = &ConsoleSpec{Type: "vnc", Port: 5900}
	}
	if vm.Spec.CloudInit != nil {
		if vm.Spec.CloudInit.User == "" {
			vm.Spec.CloudInit.User = DefaultCloudInitUser
		}
		if vm.Spec.CloudInit.Password == "" {
			vm.Spec.CloudInit.Password = DefaultCloudInitPassword
		}
	}
}
