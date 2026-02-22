//go:build e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func getClient(t *testing.T) *kubernetes.Clientset {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.Getenv("HOME") + "/.kube/config"
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("building kubeconfig: %v", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("creating clientset: %v", err)
	}
	return cs
}

func TestE2E_CRDInstalled(t *testing.T) {
	out, err := exec.Command("kubectl", "get", "crd", "salvagerecords.tote.dev").CombinedOutput()
	if err != nil {
		t.Fatalf("SalvageRecord CRD not installed: %s", out)
	}
}

func TestE2E_ControllerRunning(t *testing.T) {
	cs := getClient(t)
	pods, err := cs.CoreV1().Pods("tote").List(context.Background(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=tote,app.kubernetes.io/component=controller",
	})
	if err != nil {
		t.Fatalf("listing controller pods: %v", err)
	}
	if len(pods.Items) == 0 {
		t.Fatal("no controller pods found")
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			t.Errorf("controller pod %s is %s, expected Running", pod.Name, pod.Status.Phase)
		}
	}
}

func TestE2E_UnreachableImage_EmitsEvent(t *testing.T) {
	cs := getClient(t)
	ctx := context.Background()

	// Create opted-in namespace.
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tote-e2e-test",
			Annotations: map[string]string{
				"tote.dev/allow": "true",
			},
		},
	}
	_, err := cs.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("creating namespace: %v", err)
	}
	defer func() {
		_ = cs.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{})
	}()

	// Deploy a pod with an unreachable image.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-pull-fail",
			Namespace: ns.Name,
			Annotations: map[string]string{
				"tote.dev/auto-salvage": "true",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "app",
				Image: "registry.invalid/nonexistent@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			}},
		},
	}
	_, err = cs.CoreV1().Pods(ns.Name).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("creating pod: %v", err)
	}
	defer func() {
		_ = cs.CoreV1().Pods(ns.Name).Delete(ctx, pod.Name, metav1.DeleteOptions{})
	}()

	// Wait for tote to emit an event.
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		events, err := cs.CoreV1().Events(ns.Name).List(ctx, metav1.ListOptions{
			FieldSelector: "involvedObject.name=e2e-pull-fail",
		})
		if err != nil {
			t.Fatalf("listing events: %v", err)
		}
		for _, e := range events.Items {
			if strings.Contains(e.Reason, "ImageSalvageable") || strings.Contains(e.Reason, "ImageNotActionable") {
				t.Logf("received tote event: %s - %s", e.Reason, e.Message)
				return
			}
		}
		time.Sleep(5 * time.Second)
	}
	t.Error("timed out waiting for tote event")
}
