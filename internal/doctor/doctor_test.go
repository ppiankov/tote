package doctor

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func TestRun_AllHealthy(t *testing.T) {
	clientset := fake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tote-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:        "myapp",
			Annotations: map[string]string{"tote.dev/allow": "true"},
		}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tote",
				Namespace: "tote-system",
				Labels:    map[string]string{"app.kubernetes.io/name": "tote"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](1),
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 1,
			},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tote-agent",
				Namespace: "tote-system",
				Labels: map[string]string{
					"app.kubernetes.io/name":      "tote",
					"app.kubernetes.io/component": "agent",
				},
			},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 3,
				NumberReady:            3,
			},
		},
	)

	result := Run(context.Background(), clientset, "tote-system")

	// kubeconfig and namespaces should pass; CRD will fail (fake client has no CRD).
	for _, c := range result.Checks {
		switch c.Name {
		case "kubeconfig":
			if c.Status != StatusOK {
				t.Errorf("kubeconfig: got %s (%s)", c.Status, c.Message)
			}
		case "controller":
			if c.Status != StatusOK {
				t.Errorf("controller: got %s (%s)", c.Status, c.Message)
			}
		case "agents":
			if c.Status != StatusOK {
				t.Errorf("agents: got %s (%s)", c.Status, c.Message)
			}
		case "namespaces":
			if c.Status != StatusOK {
				t.Errorf("namespaces: got %s (%s)", c.Status, c.Message)
			}
		}
	}
}

func TestRun_NoAgents(t *testing.T) {
	clientset := fake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tote-system"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tote",
				Namespace: "tote-system",
				Labels:    map[string]string{"app.kubernetes.io/name": "tote"},
			},
			Spec:   appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
		},
	)

	result := Run(context.Background(), clientset, "tote-system")

	for _, c := range result.Checks {
		if c.Name == "agents" && c.Status != StatusWarn {
			t.Errorf("agents: expected warn, got %s (%s)", c.Status, c.Message)
		}
		if c.Name == "namespaces" && c.Status != StatusWarn {
			t.Errorf("namespaces: expected warn, got %s (%s)", c.Status, c.Message)
		}
	}
}

func TestRun_ControllerNotReady(t *testing.T) {
	clientset := fake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tote-system"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tote",
				Namespace: "tote-system",
				Labels:    map[string]string{"app.kubernetes.io/name": "tote"},
			},
			Spec:   appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 0},
		},
	)

	result := Run(context.Background(), clientset, "tote-system")

	if result.OK {
		t.Error("expected result.OK=false when controller not ready")
	}
	for _, c := range result.Checks {
		if c.Name == "controller" && c.Status != StatusFail {
			t.Errorf("controller: expected fail, got %s (%s)", c.Status, c.Message)
		}
	}
}

func TestJoinMax(t *testing.T) {
	tests := []struct {
		items []string
		max   int
		want  string
	}{
		{[]string{"a", "b"}, 5, "a, b"},
		{[]string{"a", "b", "c", "d", "e", "f"}, 3, "a, b, c (and 3 more)"},
		{[]string{"x"}, 5, "x"},
	}
	for _, tt := range tests {
		got := joinMax(tt.items, tt.max)
		if got != tt.want {
			t.Errorf("joinMax(%v, %d) = %q, want %q", tt.items, tt.max, got, tt.want)
		}
	}
}
