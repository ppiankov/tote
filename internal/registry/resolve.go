package registry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// TagResolver resolves an image tag to a content digest via registry v2 API.
type TagResolver interface {
	ResolveTag(ctx context.Context, imageRef string) (string, error)
}

// HTTPTagResolver resolves tags by querying the source registry.
type HTTPTagResolver struct {
	// AuthFunc returns credentials for a registry host. May be nil (anonymous).
	AuthFunc func(ctx context.Context, host string) (username, password string, err error)

	// Transport is the HTTP transport (for custom TLS). Nil uses http.DefaultTransport.
	Transport http.RoundTripper

	// Timeout per resolve request. Zero means 5s.
	Timeout time.Duration

	// Insecure allows HTTP connections to registries.
	Insecure bool
}

// NewHTTPTagResolver creates a resolver with the given timeout.
func NewHTTPTagResolver(timeout time.Duration, insecure bool) *HTTPTagResolver {
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &HTTPTagResolver{Timeout: timeout, Insecure: insecure}
}

// WithCA configures TLS with a custom CA certificate file.
func (r *HTTPTagResolver) WithCA(caPath string) error {
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return fmt.Errorf("reading CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("no valid certificates in CA file %s", caPath)
	}
	r.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		},
	}
	return nil
}

// ResolveTag queries the source registry for the given image reference and
// returns the content digest (sha256:...). Returns empty string if the image
// or tag is not found (404).
func (r *HTTPTagResolver) ResolveTag(ctx context.Context, imageRef string) (string, error) {
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := r.nameOpts()
	ref, err := name.ParseReference(imageRef, opts...)
	if err != nil {
		return "", fmt.Errorf("parsing image ref %q: %w", imageRef, err)
	}

	remoteOpts := []remote.Option{remote.WithContext(ctx)}
	if r.Transport != nil {
		remoteOpts = append(remoteOpts, remote.WithTransport(r.Transport))
	}
	if r.AuthFunc != nil {
		host := ref.Context().RegistryStr()
		username, password, authErr := r.AuthFunc(ctx, host)
		if authErr != nil {
			return "", fmt.Errorf("getting auth for %s: %w", host, authErr)
		}
		if username != "" {
			remoteOpts = append(remoteOpts, remote.WithAuth(&authn.Basic{
				Username: username,
				Password: password,
			}))
		}
	}

	desc, err := remote.Head(ref, remoteOpts...)
	if err != nil {
		// Check for 404-like errors — tag not found in registry.
		if strings.Contains(err.Error(), "MANIFEST_UNKNOWN") ||
			strings.Contains(err.Error(), "NOT_FOUND") ||
			strings.Contains(err.Error(), "404") {
			return "", nil
		}
		return "", fmt.Errorf("querying registry for %s: %w", imageRef, err)
	}

	digest := desc.Digest.String()
	if digest == "" {
		return "", nil
	}
	return digest, nil
}

func (r *HTTPTagResolver) nameOpts() []name.Option {
	if r.Insecure {
		return []name.Option{name.Insecure}
	}
	return nil
}
