package podman

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/specgen"
	spec "github.com/opencontainers/runtime-spec/specs-go"
)

func (c *Client) RunOneShotContainer(image string, mounts []spec.Mount, entrypoint []string, command []string) (string, string, error) {
	terminal := false
	s := &specgen.SpecGenerator{
		ContainerBasicConfig: specgen.ContainerBasicConfig{
			Entrypoint: entrypoint,
			Command:    command,
			Terminal:   &terminal,
		},
		ContainerStorageConfig: specgen.ContainerStorageConfig{
			Image:  image,
			Mounts: mounts,
		},
		ContainerHealthCheckConfig: specgen.ContainerHealthCheckConfig{
			HealthLogDestination: "local",
		},
	}

	resp, err := containers.CreateWithSpec(c.ctx, s, nil)
	if err != nil {
		return "", "", fmt.Errorf("creating helper container: %w", err)
	}
	defer func() {
		opts := new(containers.RemoveOptions).WithForce(true)
		_, _ = containers.Remove(c.ctx, resp.ID, opts)
	}()

	if err := containers.Start(c.ctx, resp.ID, nil); err != nil {
		return "", "", fmt.Errorf("starting helper container: %w", err)
	}

	exitCode, waitErr := containers.Wait(c.ctx, resp.ID, nil)
	stdout, stderr, logErr := collectContainerLogs(c.ctx, resp.ID)
	if logErr != nil {
		return stdout, stderr, fmt.Errorf("reading helper container logs: %w", logErr)
	}
	if waitErr != nil {
		return stdout, stderr, fmt.Errorf("waiting for helper container: %w", waitErr)
	}
	if exitCode != 0 {
		return stdout, stderr, fmt.Errorf("helper container exited with status %d", exitCode)
	}
	return stdout, stderr, nil
}

func collectContainerLogs(ctx context.Context, containerID string) (string, string, error) {
	stdoutCh := make(chan string, 128)
	stderrCh := make(chan string, 128)
	done := make(chan error, 1)

	go func() {
		opts := new(containers.LogOptions).WithStdout(true).WithStderr(true)
		done <- containers.Logs(ctx, containerID, opts, stdoutCh, stderrCh)
		close(stdoutCh)
		close(stderrCh)
	}()

	var stdoutBuilder strings.Builder
	var stderrBuilder strings.Builder
	for stdoutCh != nil || stderrCh != nil {
		select {
		case line, ok := <-stdoutCh:
			if !ok {
				stdoutCh = nil
				continue
			}
			stdoutBuilder.WriteString(line)
		case line, ok := <-stderrCh:
			if !ok {
				stderrCh = nil
				continue
			}
			stderrBuilder.WriteString(line)
		}
	}

	if err := <-done; err != nil {
		return stdoutBuilder.String(), stderrBuilder.String(), err
	}
	return stdoutBuilder.String(), stderrBuilder.String(), nil
}
