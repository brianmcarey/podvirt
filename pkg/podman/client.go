package podman

import (
	"context"
	"fmt"

	"github.com/brianmcarey/podvirt/pkg/util"
	"github.com/containers/podman/v5/pkg/bindings"
)

type Client struct {
	ctx context.Context
}

func New(ctx context.Context) (*Client, error) {
	if err := util.CheckPodmanSocket(); err != nil {
		return nil, err
	}
	socketPath := util.PodmanSocketPath()
	connCtx, err := bindings.NewConnection(ctx, "unix://"+socketPath)
	if err != nil {
		return nil, fmt.Errorf("connecting to Podman socket %q: %w", socketPath, err)
	}
	return &Client{ctx: connCtx}, nil
}

func (c *Client) Context() context.Context {
	return c.ctx
}
