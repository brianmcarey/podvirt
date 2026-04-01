package podman

import (
	"testing"

	"github.com/brianmcarey/podvirt/pkg/converter"
	"github.com/brianmcarey/podvirt/pkg/util"
)

func TestContainerName(t *testing.T) {
	cases := []struct {
		vmName   string
		wantName string
	}{
		{"my-vm", util.ContainerPrefix + "my-vm"},
		{"fedora", util.ContainerPrefix + "fedora"},
		{"test-123", util.ContainerPrefix + "test-123"},
	}
	for _, c := range cases {
		got := ContainerName(c.vmName)
		if got != c.wantName {
			t.Errorf("ContainerName(%q) = %q, want %q", c.vmName, got, c.wantName)
		}
	}
}

func TestVMNameFromContainer(t *testing.T) {
	cases := []struct {
		names    []string
		wantName string
	}{
		{[]string{util.ContainerPrefix + "fedora-vm"}, "fedora-vm"},
		{[]string{"/" + util.ContainerPrefix + "fedora-vm"}, "fedora-vm"}, // podman list leading slash
		{[]string{"unrelated"}, "unrelated"},                              // no prefix, returns as-is
		{[]string{}, ""},
	}
	for _, c := range cases {
		got := vmNameFromContainer(c.names)
		if got != c.wantName {
			t.Errorf("vmNameFromContainer(%v) = %q, want %q", c.names, got, c.wantName)
		}
	}
}

func TestBuildMounts_DiskOnly(t *testing.T) {
	hostMounts := map[string]string{
		"/home/user/disk.qcow2": "/var/lib/podvirt/disks/disk.qcow2",
	}
	mounts := buildMounts(hostMounts, "/tmp/qemu-support", "/tmp/qemu-caps", "/tmp/logs")

	if len(mounts) != 8 {
		t.Fatalf("expected 8 mounts (tmpfs + qemu-support + qemu.conf + wrapper + real + caps + logs + disk), got %d", len(mounts))
	}

	var foundDisk, foundTmpfs bool
	for _, m := range mounts {
		if m.Source == "/home/user/disk.qcow2" {
			foundDisk = true
			if m.Destination != "/var/lib/podvirt/disks/disk.qcow2" {
				t.Errorf("disk mount destination = %q, want %q", m.Destination, "/var/lib/podvirt/disks/disk.qcow2")
			}
		}
		if m.Type == "tmpfs" && m.Destination == "/var/run/libvirt" {
			foundTmpfs = true
		}
	}
	if !foundDisk {
		t.Error("expected a mount for the disk image")
	}
	if !foundTmpfs {
		t.Error("expected a tmpfs mount for /var/run/libvirt")
	}
}

func TestBuildMounts_NoDisks(t *testing.T) {
	mounts := buildMounts(nil, "/tmp/qemu-support", "/tmp/qemu-caps", "/tmp/logs")
	// Should have tmpfs, support dir, qemu.conf, wrapper, real binary, caps dir, logs dir
	if len(mounts) != 7 {
		t.Fatalf("expected 7 mounts (tmpfs + qemu-support + qemu.conf + wrapper + real + caps + logs), got %d", len(mounts))
	}
	var foundTmpfs bool
	for _, m := range mounts {
		if m.Type == "tmpfs" && m.Destination == "/var/run/libvirt" {
			foundTmpfs = true
		}
	}
	if !foundTmpfs {
		t.Errorf("expected libvirt tmpfs mount")
	}
}

func TestBuildMounts_SELinuxLabel(t *testing.T) {
	mounts := buildMounts(map[string]string{"/a": "/b"}, "/tmp/qemu-support", "/tmp/qemu-caps", "/tmp/logs")
	for _, m := range mounts {
		if m.Type == "tmpfs" {
			continue
		}
		found := false
		for _, o := range m.Options {
			if o == "z" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("mount %q → %q missing SELinux 'z' option", m.Source, m.Destination)
		}
	}
}

func TestBuildMounts_ContainerDiskOnly(t *testing.T) {
	result := &converter.VMIResult{
		VMIJSON:    `{}`,
		HostMounts: map[string]string{},
	}
	mounts := buildMounts(result.HostMounts, "/tmp/qemu-support", "/tmp/qemu-caps", "/tmp/logs")
	if len(mounts) != 7 {
		t.Fatalf("expected 7 mounts (tmpfs + qemu-support + qemu.conf + wrapper + real + caps + logs), got %d", len(mounts))
	}
	var foundTmpfs bool
	for _, m := range mounts {
		if m.Type == "tmpfs" {
			foundTmpfs = true
		}
	}
	if !foundTmpfs {
		t.Fatalf("expected tmpfs mount")
	}
}

