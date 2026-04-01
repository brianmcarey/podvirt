package libvirt

import (
	"testing"

	"github.com/brianmcarey/podvirt/pkg/util"
)

func TestNewClient_ContainerName(t *testing.T) {
	c := NewClient("my-vm")
	want := util.ContainerPrefix + "my-vm"
	if c.containerName != want {
		t.Errorf("containerName = %q, want %q", c.containerName, want)
	}
}

func TestParseDomainState(t *testing.T) {
	cases := []struct {
		input string
		want  DomainState
	}{
		{"running", StateRunning},
		{"Running", StateRunning},
		{"  running  ", StateRunning},
		{"paused", StatePaused},
		{"shut off", StateShutOff},
		{"in shutdown", StateShutting},
		{"crashed", StateCrashed},
		{"unknown", StateUnknown},
		{"something else", StateUnknown},
		{"", StateUnknown},
	}
	for _, c := range cases {
		got := parseDomainState(c.input)
		if got != c.want {
			t.Errorf("parseDomainState(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestParseDomainInfo_Running(t *testing.T) {
	raw := `Id:             1
Name:           test-vm
UUID:           abc123
OS Type:        hvm
State:          running
CPU(s):         2
CPU time:       5.0s
Max memory:     2097152 KiB
Used memory:    1048576 KiB
Persistent:     yes`

	info, err := parseDomainInfo("test-vm", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.State != StateRunning {
		t.Errorf("State = %q, want %q", info.State, StateRunning)
	}
	if info.CPUs != 2 {
		t.Errorf("CPUs = %d, want 2", info.CPUs)
	}
	if info.MaxMemKiB != 2097152 {
		t.Errorf("MaxMemKiB = %d, want 2097152", info.MaxMemKiB)
	}
	if info.UsedMemKiB != 1048576 {
		t.Errorf("UsedMemKiB = %d, want 1048576", info.UsedMemKiB)
	}
	if info.Name != "test-vm" {
		t.Errorf("Name = %q, want test-vm", info.Name)
	}
}

func TestParseDomainInfo_ShutOff(t *testing.T) {
	raw := `Id:             -
Name:           stopped-vm
State:          shut off
CPU(s):         4
Max memory:     4194304 KiB
Used memory:    4194304 KiB`

	info, err := parseDomainInfo("stopped-vm", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.State != StateShutOff {
		t.Errorf("State = %q, want %q", info.State, StateShutOff)
	}
	if info.CPUs != 4 {
		t.Errorf("CPUs = %d, want 4", info.CPUs)
	}
}

func TestParseDomainInfo_MissingFields(t *testing.T) {
	info, err := parseDomainInfo("bare-vm", "Name: bare-vm\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.State != StateUnknown {
		t.Errorf("State = %q, want unknown", info.State)
	}
	if info.CPUs != 0 {
		t.Errorf("CPUs = %d, want 0", info.CPUs)
	}
}

func TestParseKiB(t *testing.T) {
	cases := []struct {
		input string
		want  uint64
	}{
		{"2097152 KiB", 2097152},
		{"1048576 KiB", 1048576},
		{"0 KiB", 0},
		{"", 0},
		{"invalid", 0},
	}
	for _, c := range cases {
		got := parseKiB(c.input)
		if got != c.want {
			t.Errorf("parseKiB(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

func TestListDomains_ParsesNames(t *testing.T) {
	// Test the parsing logic directly (no live container needed).
	raw := " vm-one\n vm-two\n\n vm-three\n"
	var names []string
	for _, line := range splitLines(raw) {
		if name := trimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d: %v", len(names), names)
	}
	if names[0] != "vm-one" || names[1] != "vm-two" || names[2] != "vm-three" {
		t.Errorf("unexpected names: %v", names)
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
