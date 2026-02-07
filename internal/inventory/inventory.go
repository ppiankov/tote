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
