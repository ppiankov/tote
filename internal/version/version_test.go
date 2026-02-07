package version

import "testing"

func TestDefaultVersion(t *testing.T) {
	if Version != "dev" {
		t.Errorf("expected default version %q, got %q", "dev", Version)
	}
}
