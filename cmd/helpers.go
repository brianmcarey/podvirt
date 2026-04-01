package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/brianmcarey/podvirt/pkg/podman"
	"github.com/brianmcarey/podvirt/pkg/util"
)

func newPodmanClient() (*podman.Client, error) {
	if err := util.CheckKVM(); err != nil {
		return nil, fmt.Errorf("pre-flight check failed:\n%w", err)
	}
	c, err := podman.New(context.Background())
	if err != nil {
		return nil, err
	}
	return c, nil
}

func confirmAction(in io.Reader, out io.Writer, prompt string) (bool, error) {
	fmt.Fprint(out, prompt)

	resp, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("reading confirmation: %w", err)
	}
	return strings.EqualFold(strings.TrimSpace(resp), "y"), nil
}
