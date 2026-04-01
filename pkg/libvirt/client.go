// Package libvirt provides VM domain operations by running virsh commands
// inside the virt-launcher container via the Podman exec API.
package libvirt

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/brianmcarey/podvirt/pkg/util"
	"github.com/containers/podman/v5/pkg/api/handlers"
	"github.com/containers/podman/v5/pkg/bindings"
	"github.com/containers/podman/v5/pkg/bindings/containers"
)

const virshTimeout = 30 * time.Second

type Client struct {
	containerName string
}

func NewClient(vmName string) *Client {
	return &Client{
		containerName: util.ContainerPrefix + vmName,
	}
}

func (c *Client) virsh(args ...string) (string, error) {
	timeoutCtx, cancel := context.WithTimeout(context.Background(), virshTimeout)
	defer cancel()

	ctx, err := bindings.NewConnection(timeoutCtx, "unix://"+util.PodmanSocketPath())
	if err != nil {
		return "", fmt.Errorf("connecting to Podman: %w", err)
	}
	cmdName := strings.Join(args, " ")

	cfg := &handlers.ExecCreateConfig{}
	cfg.AttachStdout = true
	cfg.AttachStderr = true
	cfg.Cmd = append([]string{"virsh"}, args...)

	sessionID, err := containers.ExecCreate(ctx, c.containerName, cfg)
	if err != nil {
		return "", fmt.Errorf("virsh %s: creating exec: %w", cmdName, err)
	}

	var stdout, stderr bytes.Buffer
	opts := new(containers.ExecStartAndAttachOptions).
		WithAttachOutput(true).
		WithAttachError(true).
		WithOutputStream(io.Writer(&stdout)).
		WithErrorStream(io.Writer(&stderr)).
		WithInputStream(*bufio.NewReader(strings.NewReader("")))

	if err := containers.ExecStartAndAttach(ctx, sessionID, opts); err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("virsh %s timed out after %s", cmdName, virshTimeout)
		}
		return "", fmt.Errorf("virsh %s: %w\n%s", cmdName, err, stderr.String())
	}
	if stderr.Len() > 0 && stdout.Len() == 0 {
		return "", fmt.Errorf("virsh %s: %s", cmdName, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
