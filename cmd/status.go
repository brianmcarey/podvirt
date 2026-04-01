package cmd

import (
	"fmt"
	"time"

	"github.com/brianmcarey/podvirt/pkg/libvirt"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status <vm-name>",
	Short: "Show status of a virtual machine",
	Args:  cobra.ExactArgs(1),
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().Bool("watch", false, "Continuously monitor VM status (every 2s)")
}

func runStatus(cmd *cobra.Command, args []string) error {
	vmName := args[0]
	watch, _ := cmd.Flags().GetBool("watch")

	client, err := newPodmanClient()
	if err != nil {
		return err
	}

	exists, err := client.ExistsVM(vmName)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("VM %q not found", vmName)
	}

	printStatus := func() error {
		vmInfo, err := client.InspectVM(vmName)
		if err != nil {
			return err
		}

		lv := libvirt.NewClient(vmName)
		domInfo, domErr := lv.GetInfo(vmName)

		fmt.Printf("VM:           %s\n", vmName)
		fmt.Printf("Container ID: %s\n", vmInfo.ContainerID[:12])
		fmt.Printf("Container:    %s\n", vmInfo.State)
		fmt.Printf("Image:        %s\n", vmInfo.Image)

		if domErr == nil {
			fmt.Printf("Domain state: %s\n", domInfo.State)
			fmt.Printf("vCPUs:        %d\n", domInfo.CPUs)
			fmt.Printf("Memory (max): %d MiB\n", domInfo.MaxMemKiB/1024)
			fmt.Printf("Memory (used):%d MiB\n", domInfo.UsedMemKiB/1024)
		} else {
			fmt.Printf("Domain state: unavailable (%v)\n", domErr)
		}
		return nil
	}

	if !watch {
		return printStatus()
	}

	for {
		fmt.Print("\033[H\033[2J")
		fmt.Printf("=== %s  [Ctrl-C to quit] ===\n\n", time.Now().Format("15:04:05"))
		if err := printStatus(); err != nil {
			return err
		}
		time.Sleep(2 * time.Second)
	}
}
