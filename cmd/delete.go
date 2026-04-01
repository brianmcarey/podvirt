package cmd

import (
	"fmt"
	"os"

	"github.com/brianmcarey/podvirt/pkg/libvirt"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:     "delete <vm-name>",
	Short:   "Delete a virtual machine",
	Aliases: []string{"rm"},
	Args:    cobra.ExactArgs(1),
	RunE:    runDelete,
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")
}

func runDelete(cmd *cobra.Command, args []string) error {
	vmName := args[0]
	force, _ := cmd.Flags().GetBool("force")

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

	// Confirm unless --force.
	if !force {
		confirmed, err := confirmAction(os.Stdin, cmd.OutOrStdout(), fmt.Sprintf("Delete VM %q? This cannot be undone. [y/N] ", vmName))
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
			return nil
		}
	}

	// Stop if running.
	vmInfo, err := client.InspectVM(vmName)
	if err == nil && vmInfo.State == "running" {
		fmt.Printf("Stopping VM %q before deletion...\n", vmName)
		lv := libvirt.NewClient(vmName)
		if err := lv.Destroy(vmName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: force-stopping libvirt domain for %q before deletion failed: %v\n", vmName, err)
		}
		if err := client.StopVM(vmName, 10); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: stopping container for %q before deletion failed: %v\n", vmName, err)
		}
	}

	fmt.Printf("Removing VM %q...\n", vmName)
	if err := client.RemoveVM(vmName, true); err != nil {
		return err
	}

	fmt.Printf("VM %q deleted.\n", vmName)
	return nil
}
