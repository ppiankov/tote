package transfer

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	return s
}

func agentPod(ns, name, nodeName, podIP string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "tote",
				"app.kubernetes.io/component": "agent",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
		},
		Status: corev1.PodStatus{
			PodIP: podIP,
		},
	}
}

func TestEndpointForNode_Found(t *testing.T) {
	scheme := newScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(
		agentPod("tote-system", "agent-abc", "node-1", "10.0.0.1"),
		agentPod("tote-system", "agent-def", "node-2", "10.0.0.2"),
	).Build()

	r := NewResolver(cl, "tote-system", 9090)

	ep, err := r.EndpointForNode(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep != "10.0.0.1:9090" {
		t.Errorf("expected 10.0.0.1:9090, got %s", ep)
	}
}

func TestEndpointForNode_NotFound(t *testing.T) {
	scheme := newScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(
		agentPod("tote-system", "agent-abc", "node-1", "10.0.0.1"),
	).Build()

	r := NewResolver(cl, "tote-system", 9090)

	_, err := r.EndpointForNode(context.Background(), "node-missing")
	if err == nil {
		t.Fatal("expected error for missing node")
	}
}

func TestEndpointForNode_NoPodIP(t *testing.T) {
	scheme := newScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(
		agentPod("tote-system", "agent-abc", "node-1", ""), // no IP yet
	).Build()

	r := NewResolver(cl, "tote-system", 9090)

	_, err := r.EndpointForNode(context.Background(), "node-1")
	if err == nil {
		t.Fatal("expected error when pod has no IP")
	}
}

func TestEndpointForNode_WrongNamespace(t *testing.T) {
	scheme := newScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(
		agentPod("other-ns", "agent-abc", "node-1", "10.0.0.1"),
	).Build()

	r := NewResolver(cl, "tote-system", 9090)

	_, err := r.EndpointForNode(context.Background(), "node-1")
	if err == nil {
		t.Fatal("expected error when agent is in wrong namespace")
	}
}
