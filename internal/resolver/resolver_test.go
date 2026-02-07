package resolver

import "testing"

const validDigest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestResolve_DigestOnly(t *testing.T) {
	r := Resolve("nginx@" + validDigest)
	if !r.Actionable {
		t.Error("expected actionable for digest reference")
	}
	if r.Digest != validDigest {
		t.Errorf("expected digest %q, got %q", validDigest, r.Digest)
	}
}

func TestResolve_TagAndDigest(t *testing.T) {
	r := Resolve("nginx:1.25@" + validDigest)
	if !r.Actionable {
		t.Error("expected actionable for tag+digest reference")
	}
	if r.Digest != validDigest {
		t.Errorf("expected digest %q, got %q", validDigest, r.Digest)
	}
}

func TestResolve_TagOnly(t *testing.T) {
	r := Resolve("nginx:1.25")
	if r.Actionable {
		t.Error("expected not actionable for tag-only reference")
	}
	if r.Digest != "" {
		t.Errorf("expected empty digest, got %q", r.Digest)
	}
}

func TestResolve_NoTag(t *testing.T) {
	r := Resolve("nginx")
	if r.Actionable {
		t.Error("expected not actionable for bare image reference")
	}
}

func TestResolve_InvalidDigestLength(t *testing.T) {
	r := Resolve("nginx@sha256:short")
	if r.Actionable {
		t.Error("expected not actionable for short digest")
	}
}

func TestResolve_NonSHA256(t *testing.T) {
	r := Resolve("nginx@sha512:" + "a" + validDigest[7:])
	if r.Actionable {
		t.Error("expected not actionable for non-sha256 digest")
	}
}

func TestResolve_EmptyString(t *testing.T) {
	r := Resolve("")
	if r.Actionable {
		t.Error("expected not actionable for empty string")
	}
	if r.Original != "" {
		t.Errorf("expected empty original, got %q", r.Original)
	}
}

func TestResolve_PreservesOriginal(t *testing.T) {
	image := "registry.example.com/repo/image:v1.0@" + validDigest
	r := Resolve(image)
	if r.Original != image {
		t.Errorf("expected original %q, got %q", image, r.Original)
	}
}
