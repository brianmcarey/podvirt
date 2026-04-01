package libvirt

import (
	"fmt"
	"strconv"
	"strings"
)

type DomainState string

const (
	StateRunning  DomainState = "running"
	StatePaused   DomainState = "paused"
	StateShutOff  DomainState = "shut off"
	StateShutting DomainState = "in shutdown"
	StateCrashed  DomainState = "crashed"
	StateUnknown  DomainState = "unknown"
)

type DomainInfo struct {
	Name       string
	State      DomainState
	CPUs       int
	MaxMemKiB  uint64
	UsedMemKiB uint64
}

func DomainName(vmName string) string {
	return "default_" + vmName
}

func (c *Client) GetState(vmName string) (DomainState, error) {
	out, err := c.virsh("domstate", DomainName(vmName))
	if err != nil {
		return StateUnknown, fmt.Errorf("getting state for VM %q: %w", vmName, err)
	}
	return parseDomainState(out), nil
}

func (c *Client) GetInfo(vmName string) (*DomainInfo, error) {
	out, err := c.virsh("dominfo", DomainName(vmName))
	if err != nil {
		return nil, fmt.Errorf("getting info for VM %q: %w", vmName, err)
	}
	return parseDomainInfo(vmName, out)
}

func (c *Client) Shutdown(vmName string) error {
	if _, err := c.virsh("shutdown", DomainName(vmName)); err != nil {
		return fmt.Errorf("shutting down VM %q: %w", vmName, err)
	}
	return nil
}

func (c *Client) Destroy(vmName string) error {
	if _, err := c.virsh("destroy", DomainName(vmName)); err != nil {
		return fmt.Errorf("destroying VM %q: %w", vmName, err)
	}
	return nil
}

func (c *Client) ListDomains() ([]string, error) {
	out, err := c.virsh("list", "--all", "--name")
	if err != nil {
		return nil, fmt.Errorf("listing domains: %w", err)
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func parseDomainState(raw string) DomainState {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "running":
		return StateRunning
	case "paused":
		return StatePaused
	case "shut off":
		return StateShutOff
	case "in shutdown":
		return StateShutting
	case "crashed":
		return StateCrashed
	default:
		return StateUnknown
	}
}

func parseDomainInfo(vmName, raw string) (*DomainInfo, error) {
	info := &DomainInfo{Name: vmName, State: StateUnknown}

	for _, line := range strings.Split(raw, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		switch key {
		case "State":
			info.State = parseDomainState(val)
		case "CPU(s)":
			if n, err := strconv.Atoi(val); err == nil {
				info.CPUs = n
			}
		case "Max memory":
			info.MaxMemKiB = parseKiB(val)
		case "Used memory":
			info.UsedMemKiB = parseKiB(val)
		}
	}
	return info, nil
}

func parseKiB(s string) uint64 {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0
	}
	n, _ := strconv.ParseUint(fields[0], 10, 64)
	return n
}
