package agent

import (
	"context"
	"fmt"
	"io"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/transfer/archive"
	ctrdbuf "github.com/containerd/containerd/v2/core/transfer/image"
	"github.com/containerd/errdefs"
)

// ImageStore abstracts containerd image operations for testability.
type ImageStore interface {
	List(ctx context.Context) ([]string, error)
	Has(ctx context.Context, digest string) (bool, error)
	Export(ctx context.Context, digest string, w io.Writer) error
	Import(ctx context.Context, r io.Reader) (string, error)
}

// ContainerdStore implements ImageStore using the containerd client.
// Uses the "k8s.io" namespace where kubelet stores images.
type ContainerdStore struct {
	client *containerd.Client
}

// NewContainerdStore connects to containerd via the given socket path.
func NewContainerdStore(socketPath string) (*ContainerdStore, error) {
	c, err := containerd.New(socketPath, containerd.WithDefaultNamespace("k8s.io"))
	if err != nil {
		return nil, err
	}
	return &ContainerdStore{client: c}, nil
}

// Close releases the containerd client connection.
func (s *ContainerdStore) Close() error {
	return s.client.Close()
}

// List returns all image digests in the containerd store.
func (s *ContainerdStore) List(ctx context.Context) ([]string, error) {
	imgs, err := s.client.ImageService().List(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(imgs))
	var digests []string
	for _, img := range imgs {
		d := img.Target.Digest.String()
		if !seen[d] {
			seen[d] = true
			digests = append(digests, d)
		}
	}
	return digests, nil
}

// Has returns true if the given digest exists in containerd.
func (s *ContainerdStore) Has(ctx context.Context, digest string) (bool, error) {
	imgs, err := s.client.ImageService().List(ctx, "target.digest=="+digest)
	if err != nil {
		return false, err
	}
	return len(imgs) > 0, nil
}

// Export writes the image with the given digest as a tar archive to w.
func (s *ContainerdStore) Export(ctx context.Context, digest string, w io.Writer) error {
	imgs, err := s.client.ImageService().List(ctx, "target.digest=="+digest)
	if err != nil {
		return err
	}
	if len(imgs) == 0 {
		return fmt.Errorf("image %s: %w", digest, errdefs.ErrNotFound)
	}
	src := ctrdbuf.NewStore(imgs[0].Name)
	dst := archive.NewImageExportStream(nopCloser{w}, "")
	return s.client.Transfer(ctx, src, dst)
}

// Import reads a tar archive from r and imports it into containerd.
// Returns the digest of the imported image.
func (s *ContainerdStore) Import(ctx context.Context, r io.Reader) (string, error) {
	src := archive.NewImageImportStream(r, "")
	dst := ctrdbuf.NewStore("")
	if err := s.client.Transfer(ctx, src, dst); err != nil {
		return "", err
	}
	// List images to find the most recently imported one.
	// This is a simplification; in production we'd track the import result.
	imgs, err := s.client.ImageService().List(ctx)
	if err != nil {
		return "", err
	}
	if len(imgs) == 0 {
		return "", fmt.Errorf("no images after import: %w", errdefs.ErrNotFound)
	}
	return imgs[len(imgs)-1].Target.Digest.String(), nil
}

// nopCloser wraps an io.Writer to satisfy io.WriteCloser.
type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
