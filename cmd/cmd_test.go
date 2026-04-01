package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/brianmcarey/podvirt/pkg/config"
	"gopkg.in/yaml.v3"
)

func TestPrintListTable_empty(t *testing.T) {
	var buf bytes.Buffer
	printListTable(&buf, nil)
	if !strings.Contains(buf.String(), "No VMs found") {
		t.Errorf("expected 'No VMs found', got %q", buf.String())
	}
}

func TestPrintListTable_withEntries(t *testing.T) {
	entries := []vmListEntry{
		{Name: "fedora-vm", State: "running", ContainerID: "abc123def456", CPUs: 2, MemoryMiB: 1024},
		{Name: "minimal-vm", State: "stopped", ContainerID: "111222333444", CPUs: 1, MemoryMiB: 0},
	}
	var buf bytes.Buffer
	printListTable(&buf, entries)
	out := buf.String()

	for _, want := range []string{"fedora-vm", "running", "minimal-vm", "stopped", "NAME", "STATE"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in table output, got:\n%s", want, out)
		}
	}
}

func TestPrintListTable_memoryDash(t *testing.T) {
	entries := []vmListEntry{
		{Name: "vm1", State: "stopped", ContainerID: "aabbccddee00", CPUs: 0, MemoryMiB: 0},
	}
	var buf bytes.Buffer
	printListTable(&buf, entries)
	// When CPUs and MemoryMiB are zero, columns should show "-"
	out := buf.String()
	if strings.Count(out, "-") < 2 {
		t.Errorf("expected '-' placeholders for zero CPU/memory, got:\n%s", out)
	}
}

func TestVMListEntryJSON_roundtrip(t *testing.T) {
	orig := vmListEntry{
		Name:        "test-vm",
		State:       "running",
		ContainerID: "abcdef123456",
		CPUs:        4,
		MemoryMiB:   2048,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var got vmListEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, orig) {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", got, orig)
	}
}

func TestVMListEntryJSON_fieldNames(t *testing.T) {
	e := vmListEntry{Name: "my-vm", State: "stopped", ContainerID: "111", CPUs: 1, MemoryMiB: 512}
	data, _ := json.Marshal(e)
	s := string(data)
	for _, key := range []string{`"name"`, `"state"`, `"containerID"`, `"cpus"`, `"memoryMiB"`} {
		if !strings.Contains(s, key) {
			t.Errorf("expected JSON key %s in %s", key, s)
		}
	}
}

func TestVMListEntryYAML_roundtrip(t *testing.T) {
	orig := vmListEntry{
		Name:        "yaml-vm",
		State:       "running",
		ContainerID: "deadbeef0000",
		CPUs:        2,
		MemoryMiB:   4096,
	}
	data, err := yaml.Marshal(orig)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var got vmListEntry
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, orig) {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", got, orig)
	}
}

func TestCommandsRegistered(t *testing.T) {
	want := []string{"create", "start", "stop", "list", "status", "delete", "console", "ssh", "clean-cache", "version"}
	registered := map[string]bool{}
	for _, sub := range rootCmd.Commands() {
		registered[sub.Name()] = true
	}
	for _, name := range want {
		if !registered[name] {
			t.Errorf("subcommand %q not registered on root", name)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	var buf bytes.Buffer
	versionCmd.SetOut(&buf)
	appVersion = "v1.2.3"

	versionCmd.Run(versionCmd, nil)

	if got := strings.TrimSpace(buf.String()); got != "podvirt version v1.2.3" {
		t.Fatalf("unexpected version output: %q", got)
	}
}

func TestStopFlagDefaults(t *testing.T) {
	timeout, err := stopCmd.Flags().GetInt("timeout")
	if err != nil {
		t.Fatalf("timeout flag missing: %v", err)
	}
	if timeout != 5 {
		t.Errorf("expected default timeout=5, got %d", timeout)
	}
	graceful, err := stopCmd.Flags().GetBool("graceful")
	if err != nil {
		t.Fatalf("graceful flag missing: %v", err)
	}
	if graceful {
		t.Error("expected graceful default to be false")
	}
}

func TestStartFlagDefaults(t *testing.T) {
	wait, err := startCmd.Flags().GetBool("wait")
	if err != nil {
		t.Fatalf("wait flag missing: %v", err)
	}
	if wait {
		t.Error("expected wait default to be false")
	}

	waitTimeout, err := startCmd.Flags().GetInt("wait-timeout")
	if err != nil {
		t.Fatalf("wait-timeout flag missing: %v", err)
	}
	if waitTimeout != 120 {
		t.Errorf("expected default wait-timeout=120, got %d", waitTimeout)
	}
}

func TestListFlagDefaults(t *testing.T) {
	out, err := listCmd.Flags().GetString("output")
	if err != nil {
		t.Fatalf("output flag missing: %v", err)
	}
	if out != "table" {
		t.Errorf("expected default output=table, got %q", out)
	}
}

func TestCreateFlagDefaults(t *testing.T) {
	cpus, err := createCmd.Flags().GetInt("cpus")
	if err != nil {
		t.Fatalf("cpus flag missing: %v", err)
	}
	if cpus != 1 {
		t.Errorf("expected default cpus=1, got %d", cpus)
	}
	mem, err := createCmd.Flags().GetString("memory")
	if err != nil {
		t.Fatalf("memory flag missing: %v", err)
	}
	if mem != "1Gi" {
		t.Errorf("expected default memory=1Gi, got %q", mem)
	}
	diskSize, err := createCmd.Flags().GetString("disk-size")
	if err != nil {
		t.Fatalf("disk-size flag missing: %v", err)
	}
	if diskSize != "" {
		t.Errorf("expected default disk-size to be empty, got %q", diskSize)
	}
}

func TestApplyCloudInitFlags_DefaultsUser(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "id_test.pub")
	if err := os.WriteFile(keyFile, []byte("ssh-ed25519 AAAAB3Nz test@example"), 0644); err != nil {
		t.Fatalf("writing ssh key: %v", err)
	}

	vm := &config.VirtualMachine{}
	opts := &createOptions{
		sshKeyFiles:    []string{keyFile},
		sshKeyExplicit: true,
	}

	if err := applyCloudInitFlags(vm, opts); err != nil {
		t.Fatalf("applyCloudInitFlags: %v", err)
	}
	if vm.Spec.CloudInit == nil {
		t.Fatal("expected cloud-init to be created")
	}
	if vm.Spec.CloudInit.User != config.DefaultCloudInitUser {
		t.Fatalf("expected default cloud-init user %q, got %q", config.DefaultCloudInitUser, vm.Spec.CloudInit.User)
	}
	if vm.Spec.CloudInit.Password != config.DefaultCloudInitPassword {
		t.Fatalf("expected default cloud-init password %q, got %q", config.DefaultCloudInitPassword, vm.Spec.CloudInit.Password)
	}
}

func TestApplyCreateDiskSize_SetsFirstDisk(t *testing.T) {
	vm := &config.VirtualMachine{
		Spec: config.VMSpec{
			Disks: []config.DiskSpec{
				{Name: "root", Source: config.DiskSource{Image: "/tmp/root.qcow2"}},
				{Name: "data", Source: config.DiskSource{Image: "/tmp/data.qcow2"}},
			},
		},
	}

	if err := applyCreateDiskSize(vm, "20Gi"); err != nil {
		t.Fatalf("applyCreateDiskSize: %v", err)
	}
	if vm.Spec.Disks[0].Size != "20Gi" {
		t.Fatalf("expected first disk size to be set, got %q", vm.Spec.Disks[0].Size)
	}
	if vm.Spec.Disks[1].Size != "" {
		t.Fatalf("expected only first disk to change, got %q", vm.Spec.Disks[1].Size)
	}
}

func TestApplyCreateDiskSize_RequiresDisk(t *testing.T) {
	vm := &config.VirtualMachine{}
	err := applyCreateDiskSize(vm, "20Gi")
	if err == nil {
		t.Fatal("expected error when no disks are present")
	}
}

func TestParsePortForwards_Valid(t *testing.T) {
	cases := []struct {
		input    string
		hostPort int
		vmPort   int
		protocol string
	}{
		{"2222:22", 2222, 22, "tcp"},
		{"8080:80/tcp", 8080, 80, "tcp"},
		{"5000:5000/udp", 5000, 5000, "udp"},
	}
	for _, c := range cases {
		pf, err := parsePortForwards([]string{c.input})
		if err != nil {
			t.Errorf("parsePortForwards(%q): unexpected error: %v", c.input, err)
			continue
		}
		if len(pf) != 1 {
			t.Errorf("expected 1 result for %q, got %d", c.input, len(pf))
			continue
		}
		if pf[0].HostPort != c.hostPort {
			t.Errorf("%q: HostPort want %d, got %d", c.input, c.hostPort, pf[0].HostPort)
		}
		if pf[0].VMPort != c.vmPort {
			t.Errorf("%q: VMPort want %d, got %d", c.input, c.vmPort, pf[0].VMPort)
		}
		if pf[0].Protocol != c.protocol {
			t.Errorf("%q: Protocol want %q, got %q", c.input, c.protocol, pf[0].Protocol)
		}
	}
}

func TestParsePortForwards_Invalid(t *testing.T) {
	invalid := []string{"notaport", "abc:22", "2222"}
	for _, s := range invalid {
		_, err := parsePortForwards([]string{s})
		if err == nil {
			t.Errorf("parsePortForwards(%q): expected error, got nil", s)
		}
	}
}

func TestParsePortForwards_Empty(t *testing.T) {
	pf, err := parsePortForwards(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pf) != 0 {
		t.Errorf("expected empty result, got %v", pf)
	}
}

func TestFindExtractedDisk_Missing(t *testing.T) {
	got, err := findExtractedDisk(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected no disk, got %q", got)
	}
}

func TestFindExtractedDisk_Qcow2Rename(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "disk.img")
	if err := os.WriteFile(imgPath, []byte{'Q', 'F', 'I', 0xfb, 0x00}, 0644); err != nil {
		t.Fatalf("writing disk.img: %v", err)
	}

	got, err := findExtractedDisk(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(dir, "disk.qcow2")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected renamed qcow2 file: %v", err)
	}
}

func TestFindExtractedDisk_Qcow2RenameFailure(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "disk.img")
	if err := os.WriteFile(imgPath, []byte{'Q', 'F', 'I', 0xfb, 0x00}, 0644); err != nil {
		t.Fatalf("writing disk.img: %v", err)
	}
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	defer os.Chmod(dir, 0755)

	_, err := findExtractedDisk(dir)
	if err == nil {
		t.Fatal("expected rename error, got nil")
	}
	if !strings.Contains(err.Error(), "renaming extracted qcow2 image") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyPortForwards_AddsToMasquerade(t *testing.T) {
	vm := &config.VirtualMachine{
		Spec: config.VMSpec{
			Networks: []config.NetworkSpec{
				{Name: "default", Type: "masquerade"},
			},
		},
	}
	pf := []config.PortForward{{HostPort: 2222, VMPort: 22}}
	applyPortForwards(vm, pf)
	if len(vm.Spec.Networks[0].PortForwards) != 1 {
		t.Errorf("expected 1 port forward, got %d", len(vm.Spec.Networks[0].PortForwards))
	}
	if vm.Spec.Networks[0].PortForwards[0].HostPort != 2222 {
		t.Errorf("unexpected HostPort: %d", vm.Spec.Networks[0].PortForwards[0].HostPort)
	}
}

func TestApplyPortForwards_RewritesFirstNetworkToMasquerade(t *testing.T) {
	vm := &config.VirtualMachine{
		Spec: config.VMSpec{
			Networks: []config.NetworkSpec{
				{Name: "default"},
			},
		},
	}
	pf := []config.PortForward{{HostPort: 8080, VMPort: 80}}
	applyPortForwards(vm, pf)
	if vm.Spec.Networks[0].Type != "masquerade" {
		t.Errorf("expected network upgraded to masquerade, got %q", vm.Spec.Networks[0].Type)
	}
	if len(vm.Spec.Networks[0].PortForwards) != 1 {
		t.Errorf("expected 1 port forward after upgrade, got %d", len(vm.Spec.Networks[0].PortForwards))
	}
}

func TestApplyPortForwards_CreatesNetworkIfNone(t *testing.T) {
	vm := &config.VirtualMachine{}
	pf := []config.PortForward{{HostPort: 2222, VMPort: 22}}
	applyPortForwards(vm, pf)
	if len(vm.Spec.Networks) != 1 {
		t.Fatalf("expected 1 network created, got %d", len(vm.Spec.Networks))
	}
	if vm.Spec.Networks[0].Type != "masquerade" {
		t.Errorf("expected masquerade, got %q", vm.Spec.Networks[0].Type)
	}
}

func TestApplyPortForwards_NoOp(t *testing.T) {
	vm := &config.VirtualMachine{
		Spec: config.VMSpec{
			Networks: []config.NetworkSpec{
				{Name: "default", Type: "masquerade"},
			},
		},
	}
	applyPortForwards(vm, nil)
	if len(vm.Spec.Networks[0].PortForwards) != 0 {
		t.Errorf("expected no port forwards, got %d", len(vm.Spec.Networks[0].PortForwards))
	}
}

func TestCleanCache_NothingToClean(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := doCleanCache(dir, true, strings.NewReader(""), &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Nothing to clean") {
		t.Errorf("expected 'Nothing to clean', got %q", buf.String())
	}
}

func TestCleanCache_RemovesPresentDirs(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"qemu-caps", "containerdisks", "resized-disks"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	if err := doCleanCache(dir, true, strings.NewReader(""), &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Cache cleaned") {
		t.Errorf("expected 'Cache cleaned', got %q", out)
	}
	for _, sub := range []string{"qemu-caps", "containerdisks", "resized-disks"} {
		if _, err := os.Stat(filepath.Join(dir, sub)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", sub)
		}
	}
}

func TestCleanCache_ListsDirsBeforeRemoving(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"qemu-caps", "qemu-support", "libvirt-logs", "containerdisks", "resized-disks"} {
		os.MkdirAll(filepath.Join(dir, sub), 0755)
	}

	var buf bytes.Buffer
	doCleanCache(dir, true, strings.NewReader(""), &buf)
	out := buf.String()

	for _, sub := range []string{"qemu-caps", "qemu-support", "libvirt-logs", "containerdisks", "resized-disks"} {
		if !strings.Contains(out, sub) {
			t.Errorf("expected %q listed in output, got:\n%s", sub, out)
		}
	}
}

func TestCleanCache_FlagDefaults(t *testing.T) {
	force, err := cleanCacheCmd.Flags().GetBool("force")
	if err != nil {
		t.Fatalf("force flag missing: %v", err)
	}
	if force {
		t.Error("expected force default to be false")
	}
}

func TestConfirmAction(t *testing.T) {
	var out bytes.Buffer
	ok, err := confirmAction(strings.NewReader("y\n"), &out, "Proceed? [y/N] ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected confirmation to succeed")
	}
	if got := out.String(); got != "Proceed? [y/N] " {
		t.Fatalf("unexpected prompt output: %q", got)
	}
}

func TestConfirmActionEOFDefaultsToNo(t *testing.T) {
	ok, err := confirmAction(strings.NewReader(""), io.Discard, "Proceed? [y/N] ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected EOF confirmation to default to no")
	}
}
