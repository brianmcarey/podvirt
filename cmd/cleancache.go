package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var cleanCacheCmd = &cobra.Command{
	Use:   "clean-cache",
	Short: "Remove cached podvirt data",
	Long: `Remove cached data from ~/.cache/podvirt/.

Cleans:
  qemu-caps/       QEMU capability cache (rebuilt automatically on next start)
  qemu-support/    QEMU wrapper staging dir (rebuilt automatically on next start)
  libvirt-logs/    libvirt/QEMU logs (only present when PODVIRT_DEBUG=1)
  containerdisks/  Extracted containerdisk images
  resized-disks/   Per-VM resized disk copies created by --disk-size / disk.size

Disk images supplied via --disk are not touched.`,
	RunE: runCleanCache,
}

func init() {
	rootCmd.AddCommand(cleanCacheCmd)
	cleanCacheCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")
}

func runCleanCache(cmd *cobra.Command, _ []string) error {
	force, _ := cmd.Flags().GetBool("force")

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("locating cache dir: %w", err)
	}
	return doCleanCache(filepath.Join(cacheDir, "podvirt"), force, os.Stdin, os.Stdout)
}

func doCleanCache(podvirtCache string, force bool, in io.Reader, out io.Writer) error {
	targets := []string{
		filepath.Join(podvirtCache, "qemu-caps"),
		filepath.Join(podvirtCache, "qemu-support"),
		filepath.Join(podvirtCache, "libvirt-logs"),
		filepath.Join(podvirtCache, "containerdisks"),
		filepath.Join(podvirtCache, "resized-disks"),
	}

	var present []string
	for _, t := range targets {
		if _, err := os.Stat(t); err == nil {
			present = append(present, t)
		}
	}

	if len(present) == 0 {
		fmt.Fprintln(out, "Nothing to clean.")
		return nil
	}

	fmt.Fprintln(out, "The following cache directories will be removed:")
	for _, p := range present {
		fmt.Fprintf(out, "  %s\n", p)
	}

	if !force {
		confirmed, err := confirmAction(in, out, "Proceed? [y/N] ")
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(out, "Aborted.")
			return nil
		}
	}

	for _, p := range present {
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("removing %s: %w", p, err)
		}
		fmt.Fprintf(out, "Removed %s\n", p)
	}
	fmt.Fprintln(out, "Cache cleaned.")
	return nil
}
