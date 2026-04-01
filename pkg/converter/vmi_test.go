package converter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brianmcarey/podvirt/pkg/config"
)

func baseVM() *config.VirtualMachine {
	return &config.VirtualMachine{
		APIVersion: "podvirt.io/v1alpha1",
		Kind:       "VirtualMachine",
		Metadata:   config.Metadata{Name: "test-vm"},
		Spec: config.VMSpec{
			CPU:    config.CPUSpec{Cores: 2},
			Memory: "2Gi",
		},
	}
}

func TestToVMI_BasicStructure(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "rootdisk",
		Source: config.DiskSource{Image: "/fake/disk.img"},
		Bus:    "virtio",
	}}
	vm.Spec.Networks = []config.NetworkSpec{{Name: "default", Type: "masquerade"}}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.VMIJSON == "" {
		t.Fatal("VMIJSON is empty")
	}

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(result.VMIJSON), &obj); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if obj["apiVersion"] != "kubevirt.io/v1" {
		t.Errorf("expected apiVersion kubevirt.io/v1, got %v", obj["apiVersion"])
	}
	if obj["kind"] != "VirtualMachineInstance" {
		t.Errorf("expected kind VirtualMachineInstance, got %v", obj["kind"])
	}
}

func TestToVMI_CPUAndMemory(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "disk0",
		Source: config.DiskSource{Image: "/fake/disk.img"},
	}}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]interface{}
	json.Unmarshal([]byte(result.VMIJSON), &obj)

	spec := obj["spec"].(map[string]interface{})
	domain := spec["domain"].(map[string]interface{})

	cpu := domain["cpu"].(map[string]interface{})
	if cpu["cores"].(float64) != 2 {
		t.Errorf("expected 2 cores, got %v", cpu["cores"])
	}

	resources := domain["resources"].(map[string]interface{})
	requests := resources["requests"].(map[string]interface{})
	if requests["memory"] != "2Gi" {
		t.Errorf("expected memory 2Gi, got %v", requests["memory"])
	}
}

func TestToVMI_ContainerDisk(t *testing.T) {
	// ContainerImage sources are extracted to local files before ToVMI is called.
	// Verify that a pre-extracted containerdisk path produces a HostDisk volume.
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "rootdisk",
		Source: config.DiskSource{Image: "/fake/extracted-disk.img"},
		Bus:    "virtio",
	}}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.HostMounts) != 1 {
		t.Errorf("expected 1 host mount, got %v", result.HostMounts)
	}

	var obj map[string]interface{}
	json.Unmarshal([]byte(result.VMIJSON), &obj)

	spec := obj["spec"].(map[string]interface{})
	volumes := spec["volumes"].([]interface{})
	if len(volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(volumes))
	}
	vol := volumes[0].(map[string]interface{})
	if vol["hostDisk"] == nil {
		t.Errorf("expected hostDisk volume, got %v", vol)
	}
}

func TestToVMI_HostDiskMount(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "rootdisk",
		Source: config.DiskSource{Image: "/var/lib/vms/fedora.qcow2"},
		Bus:    "virtio",
	}}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.HostMounts) != 1 {
		t.Fatalf("expected 1 host mount, got %d", len(result.HostMounts))
	}
	containerPath, ok := result.HostMounts["/var/lib/vms/fedora.qcow2"]
	if !ok {
		t.Fatal("expected mount for /var/lib/vms/fedora.qcow2")
	}
	if containerPath != virtLauncherDiskBase+"/rootdisk/fedora.qcow2" {
		t.Errorf("unexpected container path: %q", containerPath)
	}

	var obj map[string]interface{}
	json.Unmarshal([]byte(result.VMIJSON), &obj)
	spec := obj["spec"].(map[string]interface{})
	volumes := spec["volumes"].([]interface{})
	vol := volumes[0].(map[string]interface{})
	hd := vol["hostDisk"].(map[string]interface{})
	if hd["type"] != "DiskOrCreate" {
		t.Errorf("expected hostDisk type DiskOrCreate, got %v", hd["type"])
	}
}

func TestToVMI_HostDiskMount_Qcow2ContentWithImgExtension(t *testing.T) {
	dir := t.TempDir()
	hostPath := filepath.Join(dir, "noble-server-cloudimg-amd64.img")
	if err := os.WriteFile(hostPath, []byte{'Q', 'F', 'I', 0xfb, 0x00}, 0644); err != nil {
		t.Fatalf("writing test qcow2 image: %v", err)
	}

	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "rootdisk",
		Source: config.DiskSource{Image: hostPath},
		Bus:    "virtio",
	}}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	containerPath, ok := result.HostMounts[hostPath]
	if !ok {
		t.Fatalf("expected mount for %q", hostPath)
	}
	want := virtLauncherDiskBase + "/rootdisk/noble-server-cloudimg-amd64.qcow2"
	if containerPath != want {
		t.Fatalf("container path = %q, want %q", containerPath, want)
	}
}

func TestToVMI_NetworkMasquerade(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "d0",
		Source: config.DiskSource{Image: "/fake/disk.img"},
	}}
	vm.Spec.Networks = []config.NetworkSpec{{Name: "default", Type: "masquerade"}}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]interface{}
	json.Unmarshal([]byte(result.VMIJSON), &obj)
	spec := obj["spec"].(map[string]interface{})
	domain := spec["domain"].(map[string]interface{})
	devices := domain["devices"].(map[string]interface{})
	interfaces := devices["interfaces"].([]interface{})
	iface := interfaces[0].(map[string]interface{})
	binding, ok := iface["binding"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected binding field on masquerade network interface, got: %v", iface)
	}
	if binding["name"] != "passt" {
		t.Errorf("expected binding name 'passt', got %q", binding["name"])
	}
}

func TestToVMI_BootSpec(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "d0",
		Source: config.DiskSource{Image: "/fake/disk.img"},
	}}
	vm.Spec.Boot = &config.BootSpec{
		Kernel:  "/boot/vmlinuz",
		Initrd:  "/boot/initrd.img",
		Cmdline: "console=ttyS0",
	}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]interface{}
	json.Unmarshal([]byte(result.VMIJSON), &obj)
	spec := obj["spec"].(map[string]interface{})
	domain := spec["domain"].(map[string]interface{})
	firmware, ok := domain["firmware"].(map[string]interface{})
	if !ok {
		t.Fatal("expected firmware field in domain")
	}
	bootloader := firmware["bootloader"].(map[string]interface{})
	if bootloader["kernel"] != "/boot/vmlinuz" {
		t.Errorf("unexpected kernel: %v", bootloader["kernel"])
	}
	if bootloader["cmdline"] != "console=ttyS0" {
		t.Errorf("unexpected cmdline: %v", bootloader["cmdline"])
	}
}

func TestToVMI_NoBootSpec(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "d0",
		Source: config.DiskSource{Image: "/fake/disk.img"},
	}}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]interface{}
	json.Unmarshal([]byte(result.VMIJSON), &obj)
	spec := obj["spec"].(map[string]interface{})
	domain := spec["domain"].(map[string]interface{})
	if _, ok := domain["firmware"]; ok {
		t.Error("firmware field should not be present when no BootSpec")
	}
}

func TestToVMI_PortForwards_InVMI(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "d0",
		Source: config.DiskSource{Image: "/fake/disk.img"},
	}}
	vm.Spec.Networks = []config.NetworkSpec{{
		Name: "default",
		Type: "masquerade",
		PortForwards: []config.PortForward{
			{HostPort: 2222, VMPort: 22, Protocol: "tcp"},
			{HostPort: 8080, VMPort: 80},
		},
	}}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.PortForwards) != 2 {
		t.Fatalf("expected 2 port forwards in result, got %d", len(result.PortForwards))
	}
	if result.PortForwards[0].HostPort != 2222 || result.PortForwards[0].VMPort != 22 {
		t.Errorf("unexpected first port forward: %+v", result.PortForwards[0])
	}

	var obj map[string]interface{}
	json.Unmarshal([]byte(result.VMIJSON), &obj)
	spec := obj["spec"].(map[string]interface{})
	domain := spec["domain"].(map[string]interface{})
	devices := domain["devices"].(map[string]interface{})
	interfaces := devices["interfaces"].([]interface{})
	iface := interfaces[0].(map[string]interface{})

	ports, ok := iface["ports"].([]interface{})
	if !ok || len(ports) != 2 {
		t.Fatalf("expected 2 ports on masquerade interface, got %v", iface["ports"])
	}
	p0 := ports[0].(map[string]interface{})
	if p0["port"].(float64) != 22 {
		t.Errorf("expected port 22, got %v", p0["port"])
	}
	if p0["protocol"] != "TCP" {
		t.Errorf("expected protocol TCP, got %v", p0["protocol"])
	}
}

func TestToVMI_PortForwards_EmptyTypeUsesPasst(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "d0",
		Source: config.DiskSource{Image: "/fake/disk.img"},
	}}
	vm.Spec.Networks = []config.NetworkSpec{{
		Name: "default",
		PortForwards: []config.PortForward{
			{HostPort: 2222, VMPort: 22},
		},
	}}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.PortForwards) != 1 {
		t.Fatalf("expected 1 port forward for default masquerade network, got %d", len(result.PortForwards))
	}

	var obj map[string]interface{}
	json.Unmarshal([]byte(result.VMIJSON), &obj)
	spec := obj["spec"].(map[string]interface{})
	domain := spec["domain"].(map[string]interface{})
	devices := domain["devices"].(map[string]interface{})
	interfaces := devices["interfaces"].([]interface{})
	iface := interfaces[0].(map[string]interface{})

	binding, ok := iface["binding"].(map[string]interface{})
	if !ok || binding["name"] != "passt" {
		t.Fatalf("expected passt binding for default network, got %v", iface)
	}
}

func TestToVMI_CloudInit_DiskAndVolumeAdded(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "rootdisk",
		Source: config.DiskSource{Image: "/fake/disk.img"},
	}}
	vm.Spec.CloudInit = &config.CloudInitSpec{
		Password: "secret",
		SSHKeys:  []string{"ssh-ed25519 AAAA..."},
	}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]interface{}
	json.Unmarshal([]byte(result.VMIJSON), &obj)
	spec := obj["spec"].(map[string]interface{})

	domain := spec["domain"].(map[string]interface{})
	devices := domain["devices"].(map[string]interface{})
	disks := devices["disks"].([]interface{})
	if len(disks) != 2 {
		t.Fatalf("expected 2 disks (rootdisk + cloudinitdisk), got %d", len(disks))
	}

	volumes := spec["volumes"].([]interface{})
	if len(volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(volumes))
	}

	var ciVol map[string]interface{}
	for _, v := range volumes {
		vm := v.(map[string]interface{})
		if vm["name"] == "cloudinitdisk" {
			ciVol = vm
		}
	}
	if ciVol == nil {
		t.Fatal("cloudinitdisk volume not found")
	}
	ci := ciVol["cloudInitNoCloud"].(map[string]interface{})
	userData := ci["userData"].(string)
	if !strings.Contains(userData, "#cloud-config") {
		t.Error("expected #cloud-config header in userData")
	}
	if !strings.Contains(userData, "name: podvirt") {
		t.Error("expected default podvirt user in userData")
	}
	if !strings.Contains(userData, "plain_text_passwd: secret") {
		t.Error("expected explicit password in userData")
	}
	if !strings.Contains(userData, "sudo: ALL=(ALL) ALL") {
		t.Error("expected sudo privileges in userData")
	}
	if result.SSHUser != config.DefaultCloudInitUser {
		t.Errorf("expected default SSH user %q, got %q", config.DefaultCloudInitUser, result.SSHUser)
	}
}

func TestToVMI_NoCloudInit_NoExtraDisks(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "rootdisk",
		Source: config.DiskSource{Image: "/fake/disk.img"},
	}}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]interface{}
	json.Unmarshal([]byte(result.VMIJSON), &obj)
	spec := obj["spec"].(map[string]interface{})
	domain := spec["domain"].(map[string]interface{})
	devices := domain["devices"].(map[string]interface{})
	disks := devices["disks"].([]interface{})
	if len(disks) != 1 {
		t.Errorf("expected 1 disk when no cloud-init, got %d", len(disks))
	}
}

func TestBuildUserData_PasswordOnly(t *testing.T) {
	ci := &config.CloudInitSpec{Password: "mypass"}
	ud, err := buildUserData(ci)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(ud, "#cloud-config\n") {
		t.Error("expected #cloud-config header")
	}
	if !strings.Contains(ud, "name: podvirt") {
		t.Error("expected default podvirt user")
	}
	if !strings.Contains(ud, "plain_text_passwd: mypass") {
		t.Error("expected plain_text_passwd field")
	}
	if !strings.Contains(ud, "sudo: ALL=(ALL) ALL") {
		t.Error("expected sudo privileges")
	}
	if !strings.Contains(ud, "ssh_pwauth: true") {
		t.Error("expected ssh_pwauth: true")
	}
	if !strings.Contains(ud, "lock_passwd: false") {
		t.Error("expected lock_passwd: false")
	}
}

func TestBuildUserData_SSHKeyNoUser(t *testing.T) {
	ci := &config.CloudInitSpec{SSHKeys: []string{"ssh-ed25519 AAAAB3Nz"}}
	ud, err := buildUserData(ci)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(ud, "name: podvirt") {
		t.Error("expected default podvirt user")
	}
	if !strings.Contains(ud, "plain_text_passwd: podvirt") {
		t.Error("expected default podvirt password")
	}
	if !strings.Contains(ud, "ssh_authorized_keys:") || !strings.Contains(ud, "ssh-ed25519 AAAAB3Nz") {
		t.Error("expected key in user ssh_authorized_keys")
	}
	if !strings.Contains(ud, "users:") {
		t.Error("expected users: block")
	}
}

func TestBuildUserData_SSHKeyWithUser(t *testing.T) {
	ci := &config.CloudInitSpec{
		User:    "fedora",
		SSHKeys: []string{"ssh-ed25519 AAAAB3Nz"},
	}
	ud, err := buildUserData(ci)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(ud, "name: fedora") {
		t.Error("expected explicit user in users block")
	}
	if !strings.Contains(ud, "plain_text_passwd: podvirt") {
		t.Error("expected default password for explicit user")
	}
	if !strings.Contains(ud, "ssh_authorized_keys:") || !strings.Contains(ud, "ssh-ed25519 AAAAB3Nz") {
		t.Error("expected key in output")
	}
	if !strings.Contains(ud, "users:") {
		t.Error("expected users: block")
	}
}

func TestBuildUserData_MultipleKeys(t *testing.T) {
	ci := &config.CloudInitSpec{
		SSHKeys: []string{"ssh-ed25519 KEY1", "ssh-rsa KEY2"},
	}
	ud, err := buildUserData(ci)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(ud, "ssh-ed25519 KEY1") || !strings.Contains(ud, "ssh-rsa KEY2") {
		t.Error("expected both keys in userData")
	}
	if !strings.Contains(ud, "plain_text_passwd: podvirt") {
		t.Error("expected default password in userData")
	}
}

func TestToVMI_NoDiskSource(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "bad-disk",
		Source: config.DiskSource{},
	}}

	_, err := ToVMI(vm)
	if err == nil {
		t.Error("expected error for disk with no source")
	}
}

func TestToVMI_MultipleDisksMixedSources(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{
		{
			Name:   "containerdisk",
			Source: config.DiskSource{Image: "/fake/disk.img"},
		},
		{
			Name:   "datadisk",
			Source: config.DiskSource{Image: "/mnt/data.qcow2"},
		},
	}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.HostMounts) != 2 {
		t.Errorf("expected 2 host mounts, got %d", len(result.HostMounts))
	}

	var obj map[string]interface{}
	json.Unmarshal([]byte(result.VMIJSON), &obj)
	spec := obj["spec"].(map[string]interface{})
	volumes := spec["volumes"].([]interface{})
	if len(volumes) != 2 {
		t.Errorf("expected 2 volumes, got %d", len(volumes))
	}
}

func TestToVMI_MetadataNamespace(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []config.DiskSpec{{
		Name:   "rootdisk",
		Source: config.DiskSource{Image: "/fake/disk.img"},
	}}

	result, err := ToVMI(vm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]interface{}
	json.Unmarshal([]byte(result.VMIJSON), &obj)
	meta := obj["metadata"].(map[string]interface{})
	if meta["namespace"] != "default" {
		t.Errorf("expected metadata.namespace=default, got %v", meta["namespace"])
	}
}
