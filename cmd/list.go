package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/brianmcarey/podvirt/pkg/libvirt"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List virtual machines",
	Aliases: []string{"ls"},
	RunE:    runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().String("state", "", "Filter by state (running, stopped, all)")
	listCmd.Flags().StringP("output", "o", "table", "Output format: table, json, yaml")
}

type vmListEntry struct {
	Name         string               `json:"name"         yaml:"name"`
	State        string               `json:"state"        yaml:"state"`
	ContainerID  string               `json:"containerID"  yaml:"containerID"`
	CPUs         int                  `json:"cpus"         yaml:"cpus"`
	MemoryMiB    uint64               `json:"memoryMiB"    yaml:"memoryMiB"`
	PortMappings []vmPortMappingEntry `json:"portMappings,omitempty" yaml:"portMappings,omitempty"`
}

type vmPortMappingEntry struct {
	HostPort      uint16 `json:"hostPort"      yaml:"hostPort"`
	ContainerPort uint16 `json:"containerPort" yaml:"containerPort"`
	Protocol      string `json:"protocol"      yaml:"protocol"`
}

func runList(cmd *cobra.Command, args []string) error {
	stateFilter, _ := cmd.Flags().GetString("state")
	outputFmt, _ := cmd.Flags().GetString("output")

	client, err := newPodmanClient()
	if err != nil {
		return err
	}

	vms, err := client.ListVMs()
	if err != nil {
		return err
	}

	var entries []vmListEntry
	for _, vm := range vms {
		lv := libvirt.NewClient(vm.Name)
		info, err := lv.GetInfo(vm.Name)

		entry := vmListEntry{
			Name:        vm.Name,
			State:       vm.State,
			ContainerID: vm.ContainerID[:12],
		}
		for _, pm := range vm.PortMappings {
			entry.PortMappings = append(entry.PortMappings, vmPortMappingEntry{
				HostPort:      pm.HostPort,
				ContainerPort: pm.ContainerPort,
				Protocol:      pm.Protocol,
			})
		}
		if err == nil {
			entry.State = string(info.State)
			entry.CPUs = info.CPUs
			entry.MemoryMiB = info.UsedMemKiB / 1024
		}

		if stateFilter != "" && stateFilter != "all" && entry.State != stateFilter {
			continue
		}
		entries = append(entries, entry)
	}

	switch outputFmt {
	case "json":
		return json.NewEncoder(os.Stdout).Encode(entries)
	case "yaml":
		return yaml.NewEncoder(os.Stdout).Encode(entries)
	default:
		printListTable(os.Stdout, entries)
	}
	return nil
}

func printListTable(w io.Writer, entries []vmListEntry) {
	if len(entries) == 0 {
		fmt.Fprintln(w, "No VMs found.")
		return
	}
	table := tablewriter.NewTable(w,
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{Borders: tw.BorderNone})),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithRowAlignment(tw.AlignLeft),
	)
	table.Header("NAME", "STATE", "CPUS", "MEMORY (MiB)", "CONTAINER ID")
	for _, e := range entries {
		mem := "-"
		if e.MemoryMiB > 0 {
			mem = fmt.Sprintf("%d", e.MemoryMiB)
		}
		cpus := "-"
		if e.CPUs > 0 {
			cpus = fmt.Sprintf("%d", e.CPUs)
		}
		table.Append([]string{e.Name, e.State, cpus, mem, e.ContainerID}) //nolint:errcheck
	}
	table.Render() //nolint:errcheck
}
