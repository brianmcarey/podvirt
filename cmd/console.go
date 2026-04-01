package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/brianmcarey/podvirt/pkg/podman"
	"github.com/spf13/cobra"
)

var consoleCmd = &cobra.Command{
	Use:   "console <vm-name>",
	Short: "Connect to a VM serial console",
	Args:  cobra.ExactArgs(1),
	RunE:  runConsole,
}

func init() {
	rootCmd.AddCommand(consoleCmd)
}

func runConsole(cmd *cobra.Command, args []string) error {
	vmName := args[0]

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

	return handleSerial(vmName)
}

// handleSerial attaches to the serial console inside the container by connecting
// directly to the Unix socket that virt-launcher exposes for the serial console.
func handleSerial(vmName string) error {
	containerName := podman.ContainerName(vmName)
	fmt.Printf("Attaching to serial console of VM %q (detach with Ctrl-C)\n\n", vmName)

	findSocket := `sh -c 'ls /var/run/kubevirt-private/*/virt-serial0 2>/dev/null | head -1'`
	c := exec.Command("podman", "exec", "-it", containerName,
		"sh", "-c", fmt.Sprintf("sock=$(%s); [ -z \"$sock\" ] && echo 'serial socket not found' >&2 && exit 1; nc -U \"$sock\"", findSocket))
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
