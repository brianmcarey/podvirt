package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <vm-name>",
	Short: "Open an SSH session to a VM",
	Long: `Connect to a running VM via SSH.

podvirt ssh connects to the VM through its host-mapped SSH port on
localhost. The VM must expose guest port 22 with --port during create,
for example --port 2222:22.

The VM must have been created with an SSH key injected via cloud-init
(use --ssh-key on podvirt create).

Example:
  podvirt create --name myvm --image quay.io/containerdisks/fedora:43 \
      --ssh-key ~/.ssh/id_ed25519.pub --port 2222:22
  podvirt start myvm
  podvirt ssh myvm`,
	Args: cobra.ArbitraryArgs,
	RunE: runSSH,
}

func init() {
	rootCmd.AddCommand(sshCmd)
	sshCmd.Flags().StringP("user", "u", "", "SSH username")
	sshCmd.Flags().IntP("port", "p", 0, "SSH host port (auto-detected from port mappings if unset)")
	sshCmd.Flags().String("identity", "", "Path to SSH private key")
}

func runSSH(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: podvirt ssh <vm-name> [-- <remote command>]")
	}
	vmName := args[0]
	remoteCmd := args[1:]
	userOverride, _ := cmd.Flags().GetString("user")
	portOverride, _ := cmd.Flags().GetInt("port")
	identity, _ := cmd.Flags().GetString("identity")

	client, err := newPodmanClient()
	if err != nil {
		return err
	}

	info, err := client.InspectVM(vmName)
	if err != nil {
		return err
	}

	if info.State != "running" {
		return fmt.Errorf("VM %q is not running (state: %s)", vmName, info.State)
	}

	hostPort := portOverride
	if hostPort == 0 {
		for _, pm := range info.PortMappings {
			if pm.ContainerPort == 22 {
				hostPort = int(pm.HostPort)
				break
			}
		}
	}
	if hostPort == 0 {
		return fmt.Errorf(`VM %q has no port 22 mapping.

Create the VM with a port forward, e.g.:
  podvirt create --name %s ... --port 2222:22`, vmName, vmName)
	}

	// Warn if SSH was not configured at create time.
	if !info.SSHKeysConfigured {
		fmt.Fprintf(os.Stderr, "Warning: no SSH keys were configured for VM %q.\n"+
			"  Authentication will likely fail. Recreate with --ssh-key, or use --identity.\n\n", vmName)
	}
	if userOverride == "" && info.SSHUser == "" {
		fmt.Fprintf(os.Stderr, "Warning: no SSH username configured for VM %q.\n"+
			"  Connecting as your local user. Recreate with --user (e.g. --user fedora), or use --user flag.\n\n", vmName)
	}

	sshArgs := []string{
		"-4",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-p", fmt.Sprintf("%d", hostPort),
	}
	if identity != "" {
		sshArgs = append(sshArgs, "-i", identity)
	}

	target := "localhost"
	if userOverride != "" {
		target = userOverride + "@localhost"
	} else if info.SSHUser != "" {
		target = info.SSHUser + "@localhost"
	}
	sshArgs = append(sshArgs, target)
	if len(remoteCmd) > 0 {
		sshArgs = append(sshArgs, remoteCmd...)
	}

	sshBin, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found in PATH: %w", err)
	}

	fmt.Printf("Connecting to VM %q via SSH (localhost:%d)...\n\n", vmName, hostPort)
	c := exec.Command(sshBin, sshArgs...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
