package registry

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// validOCITar creates a minimal valid OCI tar archive for testing.
func validOCITar() ([]byte, error) {
	img, err := random.Image(256, 1)
	if err != nil {
		return nil, fmt.Errorf("creating random image: %w", err)
	}
	var buf strings.Builder
	if err := tarball.Write(nil, img, &buf); err != nil {
		return nil, fmt.Errorf("writing tar: %w", err)
	}
	return []byte(buf.String()), nil
}

func TestPush_Success(t *testing.T) {
	// Start in-memory OCI registry.
	reg := registry.New()
	srv := httptest.NewServer(reg)
	defer srv.Close()

	// Create valid OCI tar data.
	tarData, err := validOCITar()
	if err != nil {
		t.Fatal(err)
	}

	// Export function returns the tar data.
	export := func(_ context.Context, _ string, w io.Writer) error {
		_, err := w.Write(tarData)
		return err
	}

	host := strings.TrimPrefix(srv.URL, "http://")
	targetRef := host + "/test/app:v1"

	err = Push(context.Background(), export, "sha256:test", targetRef, "", "", true)
	if err != nil {
		t.Fatalf("push failed: %v", err)
	}
}

func TestPush_ExportError(t *testing.T) {
	export := func(_ context.Context, _ string, _ io.Writer) error {
		return fmt.Errorf("export failed")
	}

	err := Push(context.Background(), export, "sha256:test", "registry.example.com/test:v1", "", "", false)
	if err == nil {
		t.Fatal("expected error when export fails")
	}
	if !strings.Contains(err.Error(), "export failed") {
		t.Errorf("expected export error, got: %v", err)
	}
}

func TestPush_InvalidTargetRef(t *testing.T) {
	export := func(_ context.Context, _ string, w io.Writer) error {
		_, err := w.Write([]byte("data"))
		return err
	}

	err := Push(context.Background(), export, "sha256:test", ":::invalid", "", "", false)
	if err == nil {
		t.Fatal("expected error for invalid target ref")
	}
}
