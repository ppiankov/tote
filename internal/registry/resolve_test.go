package registry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/registry"
)

func TestHTTPTagResolver_ResolveTag(t *testing.T) {
	// Start an in-memory registry.
	regHandler := registry.New()
	srv := httptest.NewServer(regHandler)
	defer srv.Close()

	// The in-memory registry is empty, so any tag query returns not-found.
	host := strings.TrimPrefix(srv.URL, "http://")

	r := NewHTTPTagResolver(5*time.Second, true)

	t.Run("not found returns empty string", func(t *testing.T) {
		digest, err := r.ResolveTag(context.Background(), host+"/library/nginx:latest")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if digest != "" {
			t.Fatalf("expected empty digest, got %q", digest)
		}
	})

	t.Run("invalid ref returns error", func(t *testing.T) {
		_, err := r.ResolveTag(context.Background(), "INVALID:::::ref")
		if err == nil {
			t.Fatal("expected error for invalid ref")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			time.Sleep(10 * time.Second)
		}))
		defer slowSrv.Close()

		slowHost := strings.TrimPrefix(slowSrv.URL, "http://")
		slow := NewHTTPTagResolver(100*time.Millisecond, true)
		_, err := slow.ResolveTag(context.Background(), slowHost+"/test/img:v1")
		if err == nil {
			t.Fatal("expected timeout error")
		}
	})

	t.Run("auth func called", func(t *testing.T) {
		called := false
		r2 := NewHTTPTagResolver(5*time.Second, true)
		r2.AuthFunc = func(ctx context.Context, h string) (string, string, error) {
			called = true
			return "user", "pass", nil
		}
		// Will get 404 but auth should be called.
		_, _ = r2.ResolveTag(context.Background(), host+"/test/img:v1")
		if !called {
			t.Fatal("auth func was not called")
		}
	})

	t.Run("auth func error propagated", func(t *testing.T) {
		r2 := NewHTTPTagResolver(5*time.Second, true)
		r2.AuthFunc = func(ctx context.Context, h string) (string, string, error) {
			return "", "", fmt.Errorf("auth broken")
		}
		_, err := r2.ResolveTag(context.Background(), host+"/test/img:v1")
		if err == nil || !strings.Contains(err.Error(), "auth broken") {
			t.Fatalf("expected auth error, got: %v", err)
		}
	})
}
