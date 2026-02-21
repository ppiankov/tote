package registry

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// ExportFunc exports an image as a tar archive to the writer.
type ExportFunc func(ctx context.Context, digest string, w io.Writer) error

// Push exports an image via exportFn and pushes it to a remote registry.
// The exportFn writes a containerd/OCI tar archive.
func Push(ctx context.Context, export ExportFunc, digest, targetRef, username, password string, insecure bool) error {
	var buf bytes.Buffer
	if err := export(ctx, digest, &buf); err != nil {
		return fmt.Errorf("exporting image %s: %w", digest, err)
	}

	tag, err := name.NewTag(targetRef, nameOpts(insecure)...)
	if err != nil {
		return fmt.Errorf("parsing target ref %q: %w", targetRef, err)
	}

	img, err := tarball.Image(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
	}, nil)
	if err != nil {
		return fmt.Errorf("reading image tar: %w", err)
	}

	opts := []remote.Option{remote.WithContext(ctx)}
	if username != "" {
		opts = append(opts, remote.WithAuth(&authn.Basic{
			Username: username,
			Password: password,
		}))
	}

	if err := remote.Write(tag, img, opts...); err != nil {
		return fmt.Errorf("pushing to %s: %w", targetRef, err)
	}
	return nil
}

func nameOpts(insecure bool) []name.Option {
	if insecure {
		return []name.Option{name.Insecure}
	}
	return nil
}
