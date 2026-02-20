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

// nodeWithImageGroup creates a node with a single ContainerImage entry
// whose Names list contains all provided names (simulates how kubelet
// groups tag and digest references for the same image).
func nodeWithImageGroup(name string, imageNames ...string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Images: []corev1.ContainerImage{
				{Names: imageNames},
			},
		},
	}
}

func TestFindNodesByTag_ResolvesDigest(t *testing.T) {
	tag := "registry.internal:5000/app/api:v1.0"
	finder := newFinder(
		nodeWithImageGroup("node-1", "registry.internal:5000/app/api@"+testDigest, tag),
		nodeWithImageGroup("node-2", "registry.internal:5000/app/api@"+testDigest, tag),
		nodeWithImages("node-3", "other-image:latest"),
	)

	digest, nodes, err := finder.FindNodesByTag(context.Background(), tag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digest != testDigest {
		t.Errorf("expected digest %s, got %s", testDigest, digest)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d: %v", len(nodes), nodes)
	}
}

func TestFindNodesByTag_NoMatch(t *testing.T) {
	finder := newFinder(
		nodeWithImageGroup("node-1", "registry/other@"+testDigest, "registry/other:v1"),
	)

	digest, nodes, err := finder.FindNodesByTag(context.Background(), "registry/missing:v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digest != "" {
		t.Errorf("expected empty digest, got %s", digest)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestFindNodesByTag_TagWithoutDigestSibling(t *testing.T) {
	// Node has the tag but no digest entry in the same Names group.
	finder := newFinder(
		nodeWithImages("node-1", "registry/app:v1"),
	)

	digest, nodes, err := finder.FindNodesByTag(context.Background(), "registry/app:v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digest != "" {
		t.Errorf("expected empty digest, got %s", digest)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}
