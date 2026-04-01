package util

import (
	"fmt"
	"os"
	"os/exec"
)

func CheckPodmanSocket() error {
	socketPath := PodmanSocketPath()
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return fmt.Errorf(`Podman socket not found at %q

podvirt uses the Podman REST API, which requires the Podman socket to be running.

To enable it for your user (rootless, recommended):
    systemctl --user enable --now podman.socket

To verify it is running:
    systemctl --user status podman.socket

The socket will then be available at:
    %s`, socketPath, socketPath)
	}
	return nil
}

func CheckKVM() error {
	if _, err := os.Stat("/dev/kvm"); os.IsNotExist(err) {
		return fmt.Errorf(`/dev/kvm not found

KVM kernel module is required to run virtual machines.

To load it:
    sudo modprobe kvm_intel   # Intel CPUs
    sudo modprobe kvm_amd     # AMD CPUs

To make it persistent:
    echo 'kvm_intel' | sudo tee /etc/modules-load.d/kvm.conf`)
	}

	// Check read access (user must be in the kvm group for rootless).
	f, err := os.OpenFile("/dev/kvm", os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf(`/dev/kvm exists but is not accessible: %w

Add your user to the kvm group:
    sudo usermod -aG kvm %s
    # Log out and back in for the change to take effect`, err, os.Getenv("USER"))
	}
	f.Close()
	return nil
}

func PodmanBinaryPath() (string, error) {
	// Check PATH first — covers the normal case (running from host).
	if p, err := exec.LookPath("podman"); err == nil {
		return p, nil
	}
	// Toolbox / distrobox inject the host binaries under /var/run/host or /run/host.
	for _, candidate := range []string{
		"/var/run/host/usr/bin/podman",
		"/run/host/usr/bin/podman",
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("podman binary not found in PATH or at /var/run/host/usr/bin/podman")
}

func PodmanSocketPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		p := xdg + "/podman/podman.sock"
		if _, err := os.Stat(p); err == nil {
			return p
		}
		return p
	}
	return "/run/podman/podman.sock"
}
