package cmd

import (
	"fmt"
	"time"

	"github.com/brianmcarey/podvirt/pkg/libvirt"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start <vm-name>",
	Short: "Start a virtual machine",
	Args:  cobra.ExactArgs(1),
	RunE:  runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().Bool("wait", false, "Wait until VM reaches running state")
	startCmd.Flags().Int("wait-timeout", 120, "Seconds to wait for VM to become running (used with --wait)")
}

func runStart(cmd *cobra.Command, args []string) error {
	vmName := args[0]
	wait, _ := cmd.Flags().GetBool("wait")
	waitTimeout, _ := cmd.Flags().GetInt("wait-timeout")

	client, err := newPodmanClient()
	if err != nil {
		return err
	}

	exists, err := client.ExistsVM(vmName)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("VM %q not found (create it first with 'podvirt create')", vmName)
	}

	fmt.Printf("Starting VM %q...\n", vmName)
	if err := client.StartVM(vmName); err != nil {
		return err
	}

	if !wait {
		fmt.Printf("VM %q started. Check status with: podvirt status %s\n", vmName, vmName)
		return nil
	}

	fmt.Printf("Waiting for VM %q to reach running state (timeout: %ds)...\n", vmName, waitTimeout)
	lv := libvirt.NewClient(vmName)
	deadline := time.Now().Add(time.Duration(waitTimeout) * time.Second)
	for time.Now().Before(deadline) {
		state, err := lv.GetState(vmName)
		if err == nil && state == libvirt.StateRunning {
			fmt.Printf("VM %q is running.\n", vmName)
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("VM %q did not reach running state within %ds", vmName, waitTimeout)
}
