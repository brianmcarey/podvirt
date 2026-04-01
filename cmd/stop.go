package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/brianmcarey/podvirt/pkg/libvirt"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop <vm-name>",
	Short: "Stop a virtual machine",
	Args:  cobra.ExactArgs(1),
	RunE:  runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
	stopCmd.Flags().BoolP("force", "f", false, "Force stop immediately")
	stopCmd.Flags().Bool("graceful", false, "Attempt graceful ACPI shutdown before forcing")
	stopCmd.Flags().Int("timeout", 5, "Seconds to wait for graceful shutdown before forcing (used with --graceful)")
}

func runStop(cmd *cobra.Command, args []string) error {
	vmName := args[0]
	force, _ := cmd.Flags().GetBool("force")
	graceful, _ := cmd.Flags().GetBool("graceful")
	timeout, _ := cmd.Flags().GetInt("timeout")

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

	lv := libvirt.NewClient(vmName)
	stopTimeout := uint(timeout)

	if graceful && !force {
		fmt.Printf("Sending shutdown signal to VM %q...\n", vmName)
		if err := lv.Shutdown(vmName); err != nil {
			fmt.Printf("Graceful shutdown unavailable (%v), falling back to force stop.\n", err)
			force = true
		} else {
			deadline := time.Now().Add(time.Duration(timeout) * time.Second)
			for time.Now().Before(deadline) {
				state, err := lv.GetState(vmName)
				if err != nil || state == libvirt.StateShutOff {
					break
				}
				time.Sleep(2 * time.Second)
			}
			state, _ := lv.GetState(vmName)
			if state != libvirt.StateShutOff {
				fmt.Printf("VM did not shut down within %ds, forcing stop.\n", timeout)
				force = true
			}
		}
	} else {
		force = true
	}

	if force {
		fmt.Printf("Force-stopping VM %q...\n", vmName)
		if err := lv.Destroy(vmName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: force-stopping libvirt domain for %q failed: %v\n", vmName, err)
		} else {
			// The domain is already hard-stopped, so skip Podman's extra grace period.
			stopTimeout = 0
		}
	}

	if err := client.StopVM(vmName, stopTimeout); err != nil {
		return err
	}

	fmt.Printf("VM %q stopped.\n", vmName)
	return nil
}
