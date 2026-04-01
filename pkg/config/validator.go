package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// validNameRe enforces DNS label-style VM names.
var validNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{0,61}[a-z0-9]$|^[a-z0-9]$`)

func Validate(vm *VirtualMachine) error {
	var errs []string

	if vm.Metadata.Name == "" {
		errs = append(errs, "metadata.name is required")
	} else if !validNameRe.MatchString(vm.Metadata.Name) {
		errs = append(errs, "metadata.name must be lowercase alphanumeric and hyphens only (max 63 chars)")
	}

	if vm.Spec.CPU.Cores == 0 {
		errs = append(errs, "spec.cpu.cores must be >= 1")
	} else if vm.Spec.CPU.Cores > 256 {
		errs = append(errs, "spec.cpu.cores must be <= 256")
	}

	if vm.Spec.Memory == "" {
		errs = append(errs, "spec.memory is required")
	} else if err := validateMemory(vm.Spec.Memory); err != nil {
		errs = append(errs, fmt.Sprintf("spec.memory: %v", err))
	}

	for i, disk := range vm.Spec.Disks {
		prefix := fmt.Sprintf("spec.disks[%d] (%s)", i, disk.Name)
		if disk.Name == "" {
			errs = append(errs, fmt.Sprintf("spec.disks[%d].name is required", i))
		}
		if disk.Source.Image == "" && disk.Source.ContainerImage == "" {
			errs = append(errs, fmt.Sprintf("%s: source.image or source.containerImage is required", prefix))
		}
		if disk.Source.Image != "" && disk.Source.ContainerImage != "" {
			errs = append(errs, fmt.Sprintf("%s: only one of source.image or source.containerImage may be set", prefix))
		}
		if disk.Source.Image != "" {
			if _, err := os.Stat(disk.Source.Image); os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("%s: disk image not found: %s", prefix, disk.Source.Image))
			}
		}
		if disk.Size != "" {
			if _, err := ParseQuantityBytes(disk.Size); err != nil {
				errs = append(errs, fmt.Sprintf("%s: invalid size %q (%v)", prefix, disk.Size, err))
			}
		}
		if disk.Bus != "" && disk.Bus != "virtio" && disk.Bus != "sata" && disk.Bus != "scsi" {
			errs = append(errs, fmt.Sprintf("%s: invalid bus %q (must be virtio, sata, or scsi)", prefix, disk.Bus))
		}
	}

	for i, net := range vm.Spec.Networks {
		if net.Name == "" {
			errs = append(errs, fmt.Sprintf("spec.networks[%d].name is required", i))
		}
		if net.Type != "" && net.Type != "masquerade" {
			errs = append(errs, fmt.Sprintf("spec.networks[%d] (%s): invalid type %q (must be masquerade)", i, net.Name, net.Type))
		}
		for j, pf := range net.PortForwards {
			prefix := fmt.Sprintf("spec.networks[%d].portForwards[%d]", i, j)
			if pf.HostPort < 1 || pf.HostPort > 65535 {
				errs = append(errs, fmt.Sprintf("%s: hostPort %d is out of range (1-65535)", prefix, pf.HostPort))
			}
			if pf.VMPort < 1 || pf.VMPort > 65535 {
				errs = append(errs, fmt.Sprintf("%s: vmPort %d is out of range (1-65535)", prefix, pf.VMPort))
			}
			if pf.Protocol != "" && pf.Protocol != "tcp" && pf.Protocol != "udp" {
				errs = append(errs, fmt.Sprintf("%s: invalid protocol %q (must be tcp or udp)", prefix, pf.Protocol))
			}
		}
	}

	if ci := vm.Spec.CloudInit; ci != nil {
		if ci.Password == "" && len(ci.SSHKeys) == 0 {
			errs = append(errs, "spec.cloudInit: at least one of password or sshKeys must be set")
		}
	}

	if vm.Spec.Console != nil {
		ct := vm.Spec.Console.Type
		if ct != "" && ct != "vnc" && ct != "serial" && ct != "auto" {
			errs = append(errs, fmt.Sprintf("spec.console.type: invalid value %q (must be vnc, serial, or auto)", ct))
		}
		if vm.Spec.Console.Port != 0 && (vm.Spec.Console.Port < 1 || vm.Spec.Console.Port > 65535) {
			errs = append(errs, "spec.console.port must be between 1 and 65535")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid VM config:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func validateMemory(mem string) error {
	if _, err := ParseQuantityBytes(mem); err != nil {
		return fmt.Errorf("%v (expected e.g. 2Gi, 512Mi)", err)
	}
	return nil
}
