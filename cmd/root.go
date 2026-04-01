package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var appVersion = "main"

var rootCmd = &cobra.Command{
	Use:   "podvirt",
	Short: "Run KubeVirt VMs on a workstation using Podman",
	Long: `podvirt leverages KubeVirt's virt-launcher container with Podman
to run virtual machines on a standard Linux workstation,
without requiring a full Kubernetes cluster.`,
	SilenceUsage: true,
}

func Execute(version string) {
	appVersion = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
