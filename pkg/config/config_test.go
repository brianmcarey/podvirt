package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_ValidMinimal(t *testing.T) {
	vm := &VirtualMachine{
		Metadata: Metadata{Name: "my-vm"},
		Spec: VMSpec{
			CPU:    CPUSpec{Cores: 1},
			Memory: "512Mi",
		},
	}
	if err := Validate(vm); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidate_MissingName(t *testing.T) {
	vm := &VirtualMachine{
		Spec: VMSpec{CPU: CPUSpec{Cores: 1}, Memory: "1Gi"},
	}
	if err := Validate(vm); err == nil {
		t.Error("expected error for missing name")
	}
}

func TestValidate_InvalidNames(t *testing.T) {
	cases := []string{
		"",
		"My-VM",
		"-leadinghyphen",
		"trailing-",
		"has spaces",
		"has_underscore",
		"aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffffffggg", // 65 chars
	}
	for _, name := range cases {
		vm := &VirtualMachine{
			Metadata: Metadata{Name: name},
			Spec:     VMSpec{CPU: CPUSpec{Cores: 1}, Memory: "1Gi"},
		}
		if err := Validate(vm); err == nil {
			t.Errorf("expected error for name %q", name)
		}
	}
}

func TestValidate_ValidNames(t *testing.T) {
	cases := []string{
		"vm",
		"my-vm",
		"fedora39",
		"a",
		"test-vm-123",
		"aaaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffff", // 61 chars
	}
	for _, name := range cases {
		vm := &VirtualMachine{
			Metadata: Metadata{Name: name},
			Spec:     VMSpec{CPU: CPUSpec{Cores: 1}, Memory: "1Gi"},
		}
		if err := Validate(vm); err != nil {
			t.Errorf("expected no error for name %q, got: %v", name, err)
		}
	}
}

func TestValidate_CPULimits(t *testing.T) {
	cases := []struct {
		cores   uint32
		wantErr bool
	}{
		{0, true},
		{1, false},
		{256, false},
		{257, true},
	}
	for _, c := range cases {
		vm := &VirtualMachine{
			Metadata: Metadata{Name: "vm"},
			Spec:     VMSpec{CPU: CPUSpec{Cores: c.cores}, Memory: "1Gi"},
		}
		err := Validate(vm)
		if c.wantErr && err == nil {
			t.Errorf("cores=%d: expected error", c.cores)
		}
		if !c.wantErr && err != nil {
			t.Errorf("cores=%d: unexpected error: %v", c.cores, err)
		}
	}
}

func TestValidate_MemoryFormats(t *testing.T) {
	valid := []string{"512Mi", "1Gi", "2Gi", "4096Mi", "1024Ki", "1G", "1M", "1024"}
	for _, m := range valid {
		vm := &VirtualMachine{
			Metadata: Metadata{Name: "vm"},
			Spec:     VMSpec{CPU: CPUSpec{Cores: 1}, Memory: m},
		}
		if err := Validate(vm); err != nil {
			t.Errorf("memory=%q: unexpected error: %v", m, err)
		}
	}

	invalid := []string{"", "0Gi", "abc", "1TB", "Gi", "-1Gi"}
	for _, m := range invalid {
		vm := &VirtualMachine{
			Metadata: Metadata{Name: "vm"},
			Spec:     VMSpec{CPU: CPUSpec{Cores: 1}, Memory: m},
		}
		if err := Validate(vm); err == nil {
			t.Errorf("memory=%q: expected error", m)
		}
	}
}

func TestValidate_DiskBusTypes(t *testing.T) {
	valid := []string{"virtio", "sata", "scsi", ""}
	for _, bus := range valid {
		vm := &VirtualMachine{
			Metadata: Metadata{Name: "vm"},
			Spec: VMSpec{
				CPU:    CPUSpec{Cores: 1},
				Memory: "1Gi",
				Disks: []DiskSpec{{
					Name:   "d0",
					Source: DiskSource{ContainerImage: "quay.io/containerdisks/fedora:43"},
					Bus:    bus,
				}},
			},
		}
		if err := Validate(vm); err != nil {
			t.Errorf("bus=%q: unexpected error: %v", bus, err)
		}
	}

	vm := &VirtualMachine{
		Metadata: Metadata{Name: "vm"},
		Spec: VMSpec{
			CPU:    CPUSpec{Cores: 1},
			Memory: "1Gi",
			Disks: []DiskSpec{{
				Name:   "d0",
				Source: DiskSource{ContainerImage: "quay.io/containerdisks/fedora:43"},
				Bus:    "ide",
			}},
		},
	}
	if err := Validate(vm); err == nil {
		t.Error("bus=ide: expected error")
	}
}

func TestValidate_DiskMissingSource(t *testing.T) {
	vm := &VirtualMachine{
		Metadata: Metadata{Name: "vm"},
		Spec: VMSpec{
			CPU:    CPUSpec{Cores: 1},
			Memory: "1Gi",
			Disks:  []DiskSpec{{Name: "d0", Source: DiskSource{}}},
		},
	}
	if err := Validate(vm); err == nil {
		t.Error("expected error for disk with no source")
	}
}

func TestValidate_DiskBothSources(t *testing.T) {
	vm := &VirtualMachine{
		Metadata: Metadata{Name: "vm"},
		Spec: VMSpec{
			CPU:    CPUSpec{Cores: 1},
			Memory: "1Gi",
			Disks: []DiskSpec{{
				Name: "d0",
				Source: DiskSource{
					Image:          "/some/path.qcow2",
					ContainerImage: "quay.io/containerdisks/fedora:43",
				},
			}},
		},
	}
	if err := Validate(vm); err == nil {
		t.Error("expected error for disk with both image and containerImage set")
	}
}

func TestValidate_DiskLocalImageExists(t *testing.T) {
	// Create a temp file to simulate a disk image.
	f, err := os.CreateTemp(t.TempDir(), "disk-*.qcow2")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	vm := &VirtualMachine{
		Metadata: Metadata{Name: "vm"},
		Spec: VMSpec{
			CPU:    CPUSpec{Cores: 1},
			Memory: "1Gi",
			Disks:  []DiskSpec{{Name: "d0", Source: DiskSource{Image: f.Name()}}},
		},
	}
	if err := Validate(vm); err != nil {
		t.Errorf("unexpected error for existing disk image: %v", err)
	}
}

func TestValidate_DiskLocalImageMissing(t *testing.T) {
	vm := &VirtualMachine{
		Metadata: Metadata{Name: "vm"},
		Spec: VMSpec{
			CPU:    CPUSpec{Cores: 1},
			Memory: "1Gi",
			Disks:  []DiskSpec{{Name: "d0", Source: DiskSource{Image: "/does/not/exist.qcow2"}}},
		},
	}
	if err := Validate(vm); err == nil {
		t.Error("expected error for missing disk image")
	}
}

func TestValidate_DiskSizeFormats(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "disk-*.qcow2")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	valid := []string{"20Gi", "50G", "1048576"}
	for _, size := range valid {
		vm := &VirtualMachine{
			Metadata: Metadata{Name: "vm"},
			Spec: VMSpec{
				CPU:    CPUSpec{Cores: 1},
				Memory: "1Gi",
				Disks:  []DiskSpec{{Name: "d0", Source: DiskSource{Image: f.Name()}, Size: size}},
			},
		}
		if err := Validate(vm); err != nil {
			t.Errorf("size=%q: unexpected error: %v", size, err)
		}
	}

	invalid := []string{"", "0Gi", "abc", "1TB", "-1Gi"}
	for _, size := range invalid {
		vm := &VirtualMachine{
			Metadata: Metadata{Name: "vm"},
			Spec: VMSpec{
				CPU:    CPUSpec{Cores: 1},
				Memory: "1Gi",
				Disks:  []DiskSpec{{Name: "d0", Source: DiskSource{Image: f.Name()}, Size: size}},
			},
		}
		if size == "" {
			continue
		}
		if err := Validate(vm); err == nil {
			t.Errorf("size=%q: expected error", size)
		}
	}
}

func TestValidate_NetworkTypes(t *testing.T) {
	valid := []string{"masquerade", ""}
	for _, nt := range valid {
		vm := &VirtualMachine{
			Metadata: Metadata{Name: "vm"},
			Spec: VMSpec{
				CPU:      CPUSpec{Cores: 1},
				Memory:   "1Gi",
				Networks: []NetworkSpec{{Name: "default", Type: nt}},
			},
		}
		if err := Validate(vm); err != nil {
			t.Errorf("network type=%q: unexpected error: %v", nt, err)
		}
	}

	for _, nt := range []string{"bridge", "sr-iov"} {
		vm := &VirtualMachine{
			Metadata: Metadata{Name: "vm"},
			Spec: VMSpec{
				CPU:      CPUSpec{Cores: 1},
				Memory:   "1Gi",
				Networks: []NetworkSpec{{Name: "default", Type: nt}},
			},
		}
		if err := Validate(vm); err == nil {
			t.Errorf("network type=%s: expected error", nt)
		}
	}
}

func TestValidate_ConsoleTypes(t *testing.T) {
	valid := []string{"vnc", "serial", "auto", ""}
	for _, ct := range valid {
		vm := &VirtualMachine{
			Metadata: Metadata{Name: "vm"},
			Spec: VMSpec{
				CPU:     CPUSpec{Cores: 1},
				Memory:  "1Gi",
				Console: &ConsoleSpec{Type: ct, Port: 5900},
			},
		}
		if err := Validate(vm); err != nil {
			t.Errorf("console type=%q: unexpected error: %v", ct, err)
		}
	}

	vm := &VirtualMachine{
		Metadata: Metadata{Name: "vm"},
		Spec: VMSpec{
			CPU:     CPUSpec{Cores: 1},
			Memory:  "1Gi",
			Console: &ConsoleSpec{Type: "rdp"},
		},
	}
	if err := Validate(vm); err == nil {
		t.Error("console type=rdp: expected error")
	}
}

func TestValidate_PortForwards_Valid(t *testing.T) {
	vm := &VirtualMachine{
		Metadata: Metadata{Name: "vm"},
		Spec: VMSpec{
			CPU:    CPUSpec{Cores: 1},
			Memory: "1Gi",
			Networks: []NetworkSpec{{
				Name: "default",
				Type: "masquerade",
				PortForwards: []PortForward{
					{HostPort: 2222, VMPort: 22, Protocol: "tcp"},
					{HostPort: 8080, VMPort: 80, Protocol: "udp"},
					{HostPort: 9090, VMPort: 9090}, // protocol defaults to tcp
				},
			}},
		},
	}
	if err := Validate(vm); err != nil {
		t.Errorf("expected no error for valid port forwards, got: %v", err)
	}
}

func TestValidate_PortForwards_InvalidHostPort(t *testing.T) {
	cases := []int{0, -1, 65536, 99999}
	for _, hp := range cases {
		vm := &VirtualMachine{
			Metadata: Metadata{Name: "vm"},
			Spec: VMSpec{
				CPU:    CPUSpec{Cores: 1},
				Memory: "1Gi",
				Networks: []NetworkSpec{{
					Name: "default",
					Type: "masquerade",
					PortForwards: []PortForward{
						{HostPort: hp, VMPort: 22, Protocol: "tcp"},
					},
				}},
			},
		}
		if err := Validate(vm); err == nil {
			t.Errorf("hostPort=%d: expected error", hp)
		}
	}
}

func TestValidate_PortForwards_InvalidVMPort(t *testing.T) {
	vm := &VirtualMachine{
		Metadata: Metadata{Name: "vm"},
		Spec: VMSpec{
			CPU:    CPUSpec{Cores: 1},
			Memory: "1Gi",
			Networks: []NetworkSpec{{
				Name: "default",
				Type: "masquerade",
				PortForwards: []PortForward{
					{HostPort: 2222, VMPort: 0, Protocol: "tcp"},
				},
			}},
		},
	}
	if err := Validate(vm); err == nil {
		t.Error("vmPort=0: expected error")
	}
}

func TestValidate_PortForwards_InvalidProtocol(t *testing.T) {
	vm := &VirtualMachine{
		Metadata: Metadata{Name: "vm"},
		Spec: VMSpec{
			CPU:    CPUSpec{Cores: 1},
			Memory: "1Gi",
			Networks: []NetworkSpec{{
				Name: "default",
				Type: "masquerade",
				PortForwards: []PortForward{
					{HostPort: 2222, VMPort: 22, Protocol: "sctp"},
				},
			}},
		},
	}
	if err := Validate(vm); err == nil {
		t.Error("protocol=sctp: expected error")
	}
}

func TestValidate_CloudInit_Valid(t *testing.T) {
	cases := []struct {
		name string
		ci   CloudInitSpec
	}{
		{"password only", CloudInitSpec{Password: "secret"}},
		{"key only", CloudInitSpec{SSHKeys: []string{"ssh-ed25519 AAAA..."}}},
		{"both", CloudInitSpec{Password: "secret", SSHKeys: []string{"ssh-ed25519 AAAA..."}}},
		{"with user", CloudInitSpec{User: "fedora", Password: "secret"}},
	}
	for _, c := range cases {
		ci := c.ci
		vm := &VirtualMachine{
			Metadata: Metadata{Name: "vm"},
			Spec: VMSpec{
				CPU:       CPUSpec{Cores: 1},
				Memory:    "1Gi",
				CloudInit: &ci,
			},
		}
		if err := Validate(vm); err != nil {
			t.Errorf("%s: unexpected error: %v", c.name, err)
		}
	}
}

func TestValidate_CloudInit_NeitherPasswordNorKey(t *testing.T) {
	vm := &VirtualMachine{
		Metadata: Metadata{Name: "vm"},
		Spec: VMSpec{
			CPU:       CPUSpec{Cores: 1},
			Memory:    "1Gi",
			CloudInit: &CloudInitSpec{User: "fedora"},
		},
	}
	if err := Validate(vm); err == nil {
		t.Error("expected error when cloud-init has neither password nor ssh keys")
	}
}

func TestValidate_NoCloudInit_Valid(t *testing.T) {
	vm := &VirtualMachine{
		Metadata: Metadata{Name: "vm"},
		Spec: VMSpec{
			CPU:       CPUSpec{Cores: 1},
			Memory:    "1Gi",
			CloudInit: nil,
		},
	}
	if err := Validate(vm); err != nil {
		t.Errorf("nil cloud-init: unexpected error: %v", err)
	}
}

func TestLoadFromFile_ParsesYAML(t *testing.T) {
	yaml := `
apiVersion: podvirt.io/v1alpha1
kind: VirtualMachine
metadata:
  name: test-vm
spec:
  cpu:
    cores: 2
  memory: 2Gi
  networks:
    - name: default
      type: masquerade
`
	f, err := os.CreateTemp(t.TempDir(), "vm-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(yaml)
	f.Close()

	vm, err := LoadFromFile(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vm.APIVersion != "podvirt.io/v1alpha1" {
		t.Errorf("expected apiVersion podvirt.io/v1alpha1, got %q", vm.APIVersion)
	}
	if vm.Kind != "VirtualMachine" {
		t.Errorf("expected kind VirtualMachine, got %q", vm.Kind)
	}
	if vm.Metadata.Name != "test-vm" {
		t.Errorf("expected name test-vm, got %q", vm.Metadata.Name)
	}
	if vm.Spec.CPU.Cores != 2 {
		t.Errorf("expected 2 cores, got %d", vm.Spec.CPU.Cores)
	}
	if len(vm.Spec.Networks) != 1 || vm.Spec.Networks[0].Type != "masquerade" {
		t.Errorf("expected masquerade network, got %+v", vm.Spec.Networks)
	}
}

func TestLoadFromFile_NotFound(t *testing.T) {
	_, err := LoadFromFile("/does/not/exist.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadFromFile_AppliesDefaults(t *testing.T) {
	yaml := `
metadata:
  name: minimal-vm
`
	f, err := os.CreateTemp(t.TempDir(), "vm-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(yaml)
	f.Close()

	vm, err := LoadFromFile(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vm.Spec.CPU.Cores != 1 {
		t.Errorf("expected default 1 core, got %d", vm.Spec.CPU.Cores)
	}
	if vm.Spec.Memory != "1Gi" {
		t.Errorf("expected default memory 1Gi, got %q", vm.Spec.Memory)
	}
	if len(vm.Spec.Networks) != 1 || vm.Spec.Networks[0].Type != "masquerade" {
		t.Errorf("expected default masquerade network, got %+v", vm.Spec.Networks)
	}
	if vm.Spec.Console == nil || vm.Spec.Console.Type != "vnc" {
		t.Error("expected default console type vnc")
	}
}

func TestLoadFromFile_ParsesDiskSize(t *testing.T) {
	diskPath := filepath.Join(t.TempDir(), "disk.qcow2")
	if err := os.WriteFile(diskPath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	yaml := fmt.Sprintf(`
metadata:
  name: sized-vm
spec:
  disks:
    - name: rootdisk
      source:
        image: %s
      size: 20Gi
`, diskPath)
	f, err := os.CreateTemp(t.TempDir(), "vm-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(yaml)
	f.Close()

	vm, err := LoadFromFile(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vm.Spec.Disks) != 1 {
		t.Fatalf("expected 1 disk, got %d", len(vm.Spec.Disks))
	}
	if vm.Spec.Disks[0].Size != "20Gi" {
		t.Fatalf("expected disk size 20Gi, got %q", vm.Spec.Disks[0].Size)
	}
}

func TestLoadFromFile_DefaultsCloudInitUser(t *testing.T) {
	yaml := `
metadata:
  name: cloudinit-vm
spec:
  cloudInit:
    sshKeys:
      - ssh-ed25519 AAAA... user@host
`
	f, err := os.CreateTemp(t.TempDir(), "vm-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(yaml)
	f.Close()

	vm, err := LoadFromFile(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vm.Spec.CloudInit == nil {
		t.Fatal("expected cloud-init config")
	}
	if vm.Spec.CloudInit.User != DefaultCloudInitUser {
		t.Fatalf("expected default cloud-init user %q, got %q", DefaultCloudInitUser, vm.Spec.CloudInit.User)
	}
	if vm.Spec.CloudInit.Password != DefaultCloudInitPassword {
		t.Fatalf("expected default cloud-init password %q, got %q", DefaultCloudInitPassword, vm.Spec.CloudInit.Password)
	}
}

func TestLoadFromFlags_Basic(t *testing.T) {
	vm, err := LoadFromFlags("flag-vm", "2Gi", 4, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vm.Metadata.Name != "flag-vm" {
		t.Errorf("expected flag-vm, got %q", vm.Metadata.Name)
	}
	if vm.Spec.CPU.Cores != 4 {
		t.Errorf("expected 4 cores, got %d", vm.Spec.CPU.Cores)
	}
	if vm.Spec.Memory != "2Gi" {
		t.Errorf("expected 2Gi, got %q", vm.Spec.Memory)
	}
	if len(vm.Spec.Networks) != 1 || vm.Spec.Networks[0].Type != "masquerade" {
		t.Errorf("expected default masquerade network, got %+v", vm.Spec.Networks)
	}
}

func TestLoadFromFlags_MissingName(t *testing.T) {
	_, err := LoadFromFlags("", "1Gi", 1, "", "")
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestLoadFromFlags_WithContainerImage(t *testing.T) {
	vm, err := LoadFromFlags("img-vm", "1Gi", 1, "", "quay.io/containerdisks/fedora:43")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vm.Spec.Disks) != 1 {
		t.Fatalf("expected 1 disk, got %d", len(vm.Spec.Disks))
	}
	if vm.Spec.Disks[0].Source.ContainerImage != "quay.io/containerdisks/fedora:43" {
		t.Errorf("unexpected container image: %q", vm.Spec.Disks[0].Source.ContainerImage)
	}
}

func TestLoadFromFlags_WithDiskPath(t *testing.T) {
	dir := t.TempDir()
	disk := filepath.Join(dir, "disk.qcow2")
	os.WriteFile(disk, []byte{}, 0644)

	vm, err := LoadFromFlags("disk-vm", "1Gi", 1, disk, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vm.Spec.Disks) != 1 {
		t.Fatalf("expected 1 disk, got %d", len(vm.Spec.Disks))
	}
	if vm.Spec.Disks[0].Source.Image != disk {
		t.Errorf("unexpected disk path: %q", vm.Spec.Disks[0].Source.Image)
	}
}

func TestMergeFlags_OverridesName(t *testing.T) {
	vm, _ := LoadFromFlags("original", "1Gi", 1, "", "")
	MergeFlags(vm, "overridden", "", 0, "", "")
	if vm.Metadata.Name != "overridden" {
		t.Errorf("expected overridden, got %q", vm.Metadata.Name)
	}
}

func TestMergeFlags_ZeroValuesDoNotOverride(t *testing.T) {
	vm, _ := LoadFromFlags("keep-me", "4Gi", 8, "", "")
	MergeFlags(vm, "", "", 0, "", "")
	if vm.Metadata.Name != "keep-me" {
		t.Errorf("name should not be overridden, got %q", vm.Metadata.Name)
	}
	if vm.Spec.CPU.Cores != 8 {
		t.Errorf("cores should not be overridden, got %d", vm.Spec.CPU.Cores)
	}
	if vm.Spec.Memory != "4Gi" {
		t.Errorf("memory should not be overridden, got %q", vm.Spec.Memory)
	}
}

func TestMergeFlags_ReplacesExistingLocalDisk(t *testing.T) {
	dir := t.TempDir()
	diskA := filepath.Join(dir, "a.qcow2")
	diskB := filepath.Join(dir, "b.qcow2")
	if err := os.WriteFile(diskA, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(diskB, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	vm, _ := LoadFromFlags("vm", "1Gi", 1, diskA, "")
	MergeFlags(vm, "", "", 0, diskB, "")

	if len(vm.Spec.Disks) != 1 {
		t.Fatalf("expected 1 disk, got %d", len(vm.Spec.Disks))
	}
	if vm.Spec.Disks[0].Name != "disk0" {
		t.Fatalf("expected replaced disk to be named disk0, got %q", vm.Spec.Disks[0].Name)
	}
	if vm.Spec.Disks[0].Source.Image != diskB {
		t.Fatalf("expected replacement disk image %q, got %q", diskB, vm.Spec.Disks[0].Source.Image)
	}
}

func TestMergeFlags_ReplacesExistingContainerDisk(t *testing.T) {
	vm, _ := LoadFromFlags("vm", "1Gi", 1, "", "quay.io/containerdisks/fedora:43")

	MergeFlags(vm, "", "", 0, "", "quay.io/containerdisks/ubuntu:24.04")

	containerDiskCount := 0
	for _, disk := range vm.Spec.Disks {
		if disk.Name == "containerdisk" {
			containerDiskCount++
			if disk.Source.ContainerImage != "quay.io/containerdisks/ubuntu:24.04" {
				t.Fatalf("expected container image to be replaced, got %q", disk.Source.ContainerImage)
			}
		}
	}
	if containerDiskCount != 1 {
		t.Fatalf("expected exactly 1 containerdisk entry, got %d", containerDiskCount)
	}
}
