package inventory

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testDigest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func nodeWithImages(name string, imageNames ...string) *corev1.Node {
	images := make([]corev1.ContainerImage, len(imageNames))
	for i, n := range imageNames {
		images[i] = corev1.ContainerImage{Names: []string{n}}
	}
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status:     corev1.NodeStatus{Images: images},
	}
}

func newFinder(objs ...runtime.Object) *Finder {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	cb := fake.NewClientBuilder().WithScheme(scheme)
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}
	return NewFinder(cb.Build())
}

func TestFindNodes_MatchOnSingleNode(t *testing.T) {
	finder := newFinder(
		nodeWithImages("node-1", "docker.io/nginx@"+testDigest),
		nodeWithImages("node-2", "docker.io/nginx:latest"),
	)

	nodes, err := finder.FindNodes(context.Background(), testDigest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 || nodes[0] != "node-1" {
		t.Errorf("expected [node-1], got %v", nodes)
	}
}

func TestFindNodes_MatchOnMultipleNodes(t *testing.T) {
	finder := newFinder(
		nodeWithImages("node-1", "docker.io/nginx@"+testDigest),
		nodeWithImages("node-2", "registry.io/nginx@"+testDigest),
	)

	nodes, err := finder.FindNodes(context.Background(), testDigest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestFindNodes_NoMatch(t *testing.T) {
	finder := newFinder(
		nodeWithImages("node-1", "docker.io/nginx:latest"),
	)

	nodes, err := finder.FindNodes(context.Background(), testDigest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestFindNodes_MatchByDigestNotTag(t *testing.T) {
	finder := newFinder(
		nodeWithImages("node-1", "docker.io/nginx:1.25"),
	)

	nodes, err := finder.FindNodes(context.Background(), testDigest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes for tag-only reference, got %d", len(nodes))
	}
}

func TestFindNodes_EmptyNodeList(t *testing.T) {
	finder := newFinder()

	nodes, err := finder.FindNodes(context.Background(), testDigest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestFindNodes_NodeWithNoImages(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "bare-node"},
		Status:     corev1.NodeStatus{},
	}
	finder := newFinder(node)

	nodes, err := finder.FindNodes(context.Background(), testDigest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}
