package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	ctrimg "github.com/containerd/containerd/v2/core/images"
	ctrarchive "github.com/containerd/containerd/v2/core/images/archive"
	"github.com/containerd/errdefs"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// ImageStore abstracts containerd image operations for testability.
type ImageStore interface {
	List(ctx context.Context) ([]string, error)
	Has(ctx context.Context, digest string) (bool, error)
	Size(ctx context.Context, digest string) (int64, error)
	ResolveTag(ctx context.Context, imageRef string) (string, error)
	Export(ctx context.Context, digest string, w io.Writer) error
	Import(ctx context.Context, r io.Reader) (string, error)
}

// ContainerdStore implements ImageStore using the containerd client.
// Uses the "k8s.io" namespace where kubelet stores images.
//
// Export and Import use the content-store API (core/images/archive) instead
// of the Transfer API (core/transfer/archive) for compatibility with
// containerd v1.x runtimes that lack the streaming.v1.Streaming service.
// When containerd v2 is the minimum supported runtime, these methods can be
// switched back to the Transfer API for better streaming performance:
//
//	import (
//	    "github.com/containerd/containerd/v2/core/transfer/archive"
//	    ctrdbuf "github.com/containerd/containerd/v2/core/transfer/image"
//	)
//	Export:  client.Transfer(ctx, ctrdbuf.NewStore(name), archive.NewImageExportStream(w, ""))
//	Import:  client.Transfer(ctx, archive.NewImageImportStream(r, ""), ctrdbuf.NewStore(""))
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

// ResolveTag looks up an image reference (e.g. "registry/repo:tag") in
// containerd and returns the digest if found. Returns empty string if not found.
func (s *ContainerdStore) ResolveTag(ctx context.Context, imageRef string) (string, error) {
	imgs, err := s.client.ImageService().List(ctx, "name=="+imageRef)
	if err != nil {
		return "", err
	}
	if len(imgs) == 0 {
		return "", nil
	}
	return imgs[0].Target.Digest.String(), nil
}

// Has returns true if the given digest exists in containerd.
func (s *ContainerdStore) Has(ctx context.Context, digest string) (bool, error) {
	imgs, err := s.client.ImageService().List(ctx, "target.digest=="+digest)
	if err != nil {
		return false, err
	}
	return len(imgs) > 0, nil
}

// Size returns the total content size of the image in bytes.
func (s *ContainerdStore) Size(ctx context.Context, digest string) (int64, error) {
	imgs, err := s.client.ImageService().List(ctx, "target.digest=="+digest)
	if err != nil {
		return 0, err
	}
	if len(imgs) == 0 {
		return 0, fmt.Errorf("image %s: %w", digest, errdefs.ErrNotFound)
	}
	img := containerd.NewImage(s.client, imgs[0])
	return img.Size(ctx)
}

// Export writes the image with the given digest as a tar archive to w.
// Uses direct content-store access for containerd v1.x compatibility.
func (s *ContainerdStore) Export(ctx context.Context, digest string, w io.Writer) error {
	imgs, err := s.client.ImageService().List(ctx, "target.digest=="+digest)
	if err != nil {
		return err
	}
	if len(imgs) == 0 {
		return fmt.Errorf("image %s: %w", digest, errdefs.ErrNotFound)
	}
	return ctrarchive.Export(ctx, s.client.ContentStore(), w,
		ctrarchive.WithImage(s.client.ImageService(), imgs[0].Name))
}

// Import reads a tar archive from r and imports it into containerd.
// Returns the digest of the imported image.
// Uses direct content-store access for containerd v1.x compatibility.
func (s *ContainerdStore) Import(ctx context.Context, r io.Reader) (string, error) {
	desc, err := ctrarchive.ImportIndex(ctx, s.client.ContentStore(), r)
	if err != nil {
		return "", fmt.Errorf("importing archive: %w", err)
	}

	// ImportIndex only writes blobs to the content store. We must create
	// image records so the image appears in ImageService().List().
	indexData, err := content.ReadBlob(ctx, s.client.ContentStore(), desc)
	if err != nil {
		return "", fmt.Errorf("reading imported index: %w", err)
	}

	var index ocispec.Index
	if err := json.Unmarshal(indexData, &index); err != nil {
		return "", fmt.Errorf("parsing imported index: %w", err)
	}

	if len(index.Manifests) == 0 {
		return "", fmt.Errorf("no manifests in imported archive: %w", errdefs.ErrNotFound)
	}

	var lastDigest string
	for _, m := range index.Manifests {
		name := m.Annotations[ctrimg.AnnotationImageName]
		if name == "" {
			name = m.Annotations[ocispec.AnnotationRefName]
		}
		if name == "" {
			name = m.Digest.String()
		}
		if _, err := s.client.ImageService().Create(ctx, ctrimg.Image{
			Name:   name,
			Target: m,
			Labels: map[string]string{
				"io.cri-containerd.image": "managed",
			},
		}); err != nil && !errdefs.IsAlreadyExists(err) {
			return "", fmt.Errorf("creating image record: %w", err)
		}
		lastDigest = m.Digest.String()
	}

	return lastDigest, nil
}
