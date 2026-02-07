package resolver

import "strings"

// Resolution holds the parsed image reference.
type Resolution struct {
	// Original is the raw image string from the pod spec.
	Original string

	// Digest is the sha256 digest, empty if tag-only.
	Digest string

	// Actionable is true when the image reference includes a digest.
	Actionable bool
}

// Resolve parses an image reference and extracts the digest if present.
//
// Supported formats:
//   - "registry/repo@sha256:abc123..."  -> actionable
//   - "registry/repo:tag@sha256:abc..." -> actionable
//   - "registry/repo:tag"               -> not actionable
//   - "registry/repo"                   -> not actionable (implies :latest)
func Resolve(image string) Resolution {
	r := Resolution{Original: image}

	idx := strings.LastIndex(image, "@")
	if idx == -1 {
		return r
	}

	digest := image[idx+1:]
	// sha256: (7 chars) + 64 hex chars = 71
	if strings.HasPrefix(digest, "sha256:") && len(digest) == 71 {
		r.Digest = digest
		r.Actionable = true
	}

	return r
}
