package inventory

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Finder locates nodes that have a specific image digest cached.
type Finder struct {
	Client client.Reader
}

// NewFinder creates a Finder with the given client.
func NewFinder(c client.Reader) *Finder {
	return &Finder{Client: c}
}

// FindNodes returns the names of nodes that have the given digest in their
// Status.Images. The digest should be in the format "sha256:<hex>".
func (f *Finder) FindNodes(ctx context.Context, digest string) ([]string, error) {
	var nodeList corev1.NodeList
	if err := f.Client.List(ctx, &nodeList); err != nil {
		return nil, err
	}

	var nodes []string
	for _, node := range nodeList.Items {
		if nodeHasDigest(node, digest) {
			nodes = append(nodes, node.Name)
		}
	}
	return nodes, nil
}

// FindNodesByTag searches Node.Status.Images for an image whose Names list
// contains the given tag reference (e.g. "registry/repo:tag"). When found, it
// extracts the digest from a sibling "@sha256:..." entry in the same Names
// list. Returns the resolved digest and all node names that have it.
func (f *Finder) FindNodesByTag(ctx context.Context, imageTag string) (string, []string, error) {
	var nodeList corev1.NodeList
	if err := f.Client.List(ctx, &nodeList); err != nil {
		return "", nil, err
	}

	var digest string
	var nodes []string
	for _, node := range nodeList.Items {
		d := nodeDigestForTag(node, imageTag)
		if d == "" {
			continue
		}
		if digest == "" {
			digest = d
		}
		nodes = append(nodes, node.Name)
	}
	return digest, nodes, nil
}

// nodeDigestForTag returns the digest from a Node.Status.Images entry that
// also contains the given tag. Returns empty if no match.
func nodeDigestForTag(node corev1.Node, imageTag string) string {
	for _, img := range node.Status.Images {
		hasTag := false
		var digest string
		for _, name := range img.Names {
			if name == imageTag {
				hasTag = true
			}
			if idx := strings.LastIndex(name, "@sha256:"); idx != -1 {
				d := name[idx+1:]
				if len(d) == 71 { // "sha256:" (7) + 64 hex
					digest = d
				}
			}
		}
		if hasTag && digest != "" {
			return digest
		}
	}
	return ""
}

// nodeHasDigest checks if any image name on the node contains the digest.
// Node.Status.Images[].Names contains entries like:
//   - "docker.io/library/nginx@sha256:abc123..."
//   - "docker.io/library/nginx:1.25"
func nodeHasDigest(node corev1.Node, digest string) bool {
	suffix := "@" + digest
	for _, img := range node.Status.Images {
		for _, name := range img.Names {
			if strings.HasSuffix(name, suffix) {
				return true
			}
		}
	}
	return false
}
