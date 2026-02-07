package events

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sevents "k8s.io/client-go/tools/events"
)

func testPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}
}

func TestEmitSalvageable(t *testing.T) {
	rec := k8sevents.NewFakeRecorder(10)
	emitter := NewEmitter(rec)

	emitter.EmitSalvageable(testPod(), "nginx@sha256:abc123", []string{"node-1", "node-2"})

	event := <-rec.Events
	if !strings.Contains(event, ReasonSalvageable) {
		t.Errorf("expected event to contain reason %q, got %q", ReasonSalvageable, event)
	}
	if !strings.Contains(event, "node-1, node-2") {
		t.Errorf("expected event to contain node names, got %q", event)
	}
	if !strings.Contains(event, "nginx@sha256:abc123") {
		t.Errorf("expected event to contain image reference, got %q", event)
	}
}

func TestEmitNotActionable(t *testing.T) {
	rec := k8sevents.NewFakeRecorder(10)
	emitter := NewEmitter(rec)

	emitter.EmitNotActionable(testPod(), "nginx:latest")

	event := <-rec.Events
	if !strings.Contains(event, ReasonNotActionable) {
		t.Errorf("expected event to contain reason %q, got %q", ReasonNotActionable, event)
	}
	if !strings.Contains(event, "nginx:latest") {
		t.Errorf("expected event to contain image reference, got %q", event)
	}
}

func TestEmitSalvageable_SingleNode(t *testing.T) {
	rec := k8sevents.NewFakeRecorder(10)
	emitter := NewEmitter(rec)

	emitter.EmitSalvageable(testPod(), "app@sha256:def456", []string{"node-a"})

	event := <-rec.Events
	if !strings.Contains(event, "node-a") {
		t.Errorf("expected event to contain single node name, got %q", event)
	}
}
