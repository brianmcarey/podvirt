package podman

import (
	"fmt"
	"io"
	"os"

	"github.com/containers/podman/v5/pkg/bindings/images"
)

func (c *Client) EnsureImage(image string) error {
	exists, err := c.imageExists(image)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	fmt.Printf("Pulling image %s ...\n", image)
	return c.PullImage(image)
}

func (c *Client) PullImage(image string) error {
	var w io.Writer = os.Stdout
	_, err := images.Pull(c.ctx, image, &images.PullOptions{
		ProgressWriter: &w,
	})
	if err != nil {
		return fmt.Errorf("pulling image %q: %w", image, err)
	}
	return nil
}

func (c *Client) imageExists(image string) (bool, error) {
	exists, err := images.Exists(c.ctx, image, nil)
	if err != nil {
		return false, fmt.Errorf("checking image %q: %w", image, err)
	}
	return exists, nil
}
