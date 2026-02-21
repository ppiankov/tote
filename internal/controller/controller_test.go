package controller

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/ppiankov/tote/api/v1"
	"github.com/ppiankov/tote/internal/agent"
	"github.com/ppiankov/tote/internal/config"
	"github.com/ppiankov/tote/internal/events"
	"github.com/ppiankov/tote/internal/inventory"
	"github.com/ppiankov/tote/internal/metrics"
	"github.com/ppiankov/tote/internal/session"
	"github.com/ppiankov/tote/internal/transfer"
)

const testDigest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	return s
}

func optedInNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				config.AnnotationNamespaceAllow: "true",
			},
		},
	}
}

func failingPod(ns, name, image string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Annotations: map[string]string{
				config.AnnotationPodAutoSalvage: "true",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: image}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "app",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason:  "ImagePullBackOff",
						Message: "pull failed",
					},
				},
			}},
		},
	}
}

func nodeWithImage(name, imageName string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Images: []corev1.ContainerImage{
				{Names: []string{imageName}},
			},
		},
	}
}

type testFixture struct {
	reconciler *PodReconciler
	recorder   *k8sevents.FakeRecorder
}

func setupReconciler(objs ...runtime.Object) testFixture {
	scheme := newScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme)
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}
	cl := cb.Build()

	rec := k8sevents.NewFakeRecorder(10)
	reg := prometheus.NewRegistry()

	return testFixture{
		reconciler: &PodReconciler{
			Client:  cl,
			Config:  config.New(),
			Finder:  inventory.NewFinder(cl),
			Emitter: events.NewEmitter(rec),
			Metrics: metrics.NewCounters(reg),
		},
		recorder: rec,
	}
}

func reconcileRequest(ns, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: ns, Name: name},
	}
}

func TestReconcile_KillSwitch(t *testing.T) {
	f := setupReconciler(
		optedInNamespace("default"),
		failingPod("default", "app", "nginx@"+testDigest),
	)
	f.reconciler.Config.Enabled = false

	_, err := f.reconciler.Reconcile(context.Background(), reconcileRequest("default", "app"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case event := <-f.recorder.Events:
		t.Errorf("expected no events when disabled, got: %s", event)
	default:
	}
}

func TestReconcile_DeniedNamespace(t *testing.T) {
	f := setupReconciler(
		optedInNamespace("kube-system"),
		failingPod("kube-system", "coredns", "coredns@"+testDigest),
	)

	_, err := f.reconciler.Reconcile(context.Background(), reconcileRequest("kube-system", "coredns"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case event := <-f.recorder.Events:
		t.Errorf("expected no events for denied namespace, got: %s", event)
	default:
	}
}

func TestReconcile_MissingNamespaceAnnotation(t *testing.T) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	f := setupReconciler(ns, failingPod("default", "app", "nginx@"+testDigest))

	_, err := f.reconciler.Reconcile(context.Background(), reconcileRequest("default", "app"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case event := <-f.recorder.Events:
		t.Errorf("expected no events without namespace annotation, got: %s", event)
	default:
	}
}

func TestReconcile_MissingPodAnnotation(t *testing.T) {
	pod := failingPod("default", "app", "nginx@"+testDigest)
	delete(pod.Annotations, config.AnnotationPodAutoSalvage)

	f := setupReconciler(optedInNamespace("default"), pod)

	_, err := f.reconciler.Reconcile(context.Background(), reconcileRequest("default", "app"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case event := <-f.recorder.Events:
		t.Errorf("expected no events without pod annotation, got: %s", event)
	default:
	}
}

func TestReconcile_NoFailures(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "healthy",
			Namespace: "default",
			Annotations: map[string]string{
				config.AnnotationPodAutoSalvage: "true",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "nginx:latest"}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "app",
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}

	f := setupReconciler(optedInNamespace("default"), pod)

	_, err := f.reconciler.Reconcile(context.Background(), reconcileRequest("default", "healthy"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case event := <-f.recorder.Events:
		t.Errorf("expected no events for healthy pod, got: %s", event)
	default:
	}
}

func TestReconcile_Salvageable(t *testing.T) {
	image := "registry.example.com/app@" + testDigest
	f := setupReconciler(
		optedInNamespace("default"),
		failingPod("default", "app", image),
		nodeWithImage("node-1", "registry.example.com/app@"+testDigest),
	)

	_, err := f.reconciler.Reconcile(context.Background(), reconcileRequest("default", "app"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case event := <-f.recorder.Events:
		if event == "" {
			t.Error("expected salvageable event")
		}
	default:
		t.Error("expected a salvageable event to be emitted")
	}
}

func TestReconcile_NotActionable(t *testing.T) {
	f := setupReconciler(
		optedInNamespace("default"),
		failingPod("default", "app", "nginx:latest"),
	)

	_, err := f.reconciler.Reconcile(context.Background(), reconcileRequest("default", "app"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case event := <-f.recorder.Events:
		if event == "" {
			t.Error("expected not-actionable event")
		}
	default:
		t.Error("expected a not-actionable event to be emitted")
	}
}

func TestReconcile_TagOnlyResolvableViaNodeCache(t *testing.T) {
	tag := "registry.internal:5000/app:v1.0"
	// Node has both tag and digest in the same Names group.
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: corev1.NodeStatus{
			Images: []corev1.ContainerImage{
				{Names: []string{"registry.internal:5000/app@" + testDigest, tag}},
			},
		},
	}
	f := setupReconciler(
		optedInNamespace("default"),
		failingPod("default", "app", tag),
		node,
	)

	_, err := f.reconciler.Reconcile(context.Background(), reconcileRequest("default", "app"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case event := <-f.recorder.Events:
		if event == "" {
			t.Error("expected salvageable event")
		}
	default:
		t.Error("expected a salvageable event for tag resolved via node cache")
	}
}

func TestReconcile_PodNotFound(t *testing.T) {
	f := setupReconciler(optedInNamespace("default"))

	_, err := f.reconciler.Reconcile(context.Background(), reconcileRequest("default", "gone"))
	if err != nil {
		t.Fatalf("expected no error for missing pod, got: %v", err)
	}
}

func corruptImagePod(ns, name, image, nodeName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Annotations: map[string]string{
				config.AnnotationPodAutoSalvage: "true",
			},
		},
		Spec: corev1.PodSpec{
			NodeName:   nodeName,
			Containers: []corev1.Container{{Name: "app", Image: image}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "app",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason:  "CreateContainerError",
						Message: "failed to create containerd container: failed to resolve rootfs: content digest sha256:b50153e8abcd: not found",
					},
				},
			}},
		},
	}
}

func TestReconcile_CorruptImage_NoAgent(t *testing.T) {
	pod := corruptImagePod("default", "app", "registry.example.com/app:v1", "node-1")
	f := setupReconciler(optedInNamespace("default"), pod)

	_, err := f.reconciler.Reconcile(context.Background(), reconcileRequest("default", "app"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Without AgentResolver, no events should be emitted for corrupt image.
	select {
	case event := <-f.recorder.Events:
		t.Errorf("expected no events without agent resolver, got: %s", event)
	default:
	}
}

func TestReconcile_CorruptImage_WithAgent(t *testing.T) {
	store := agent.NewFakeImageStore()
	store.AddTag("registry.example.com/app:v1", "sha256:abc")

	sessions := session.NewStore()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := lis.Addr().(*net.TCPAddr).Port

	srv := grpc.NewServer()
	v1.RegisterToteAgentServer(srv, agent.NewServer(store, sessions, port))
	go func() { _ = srv.Serve(lis) }()
	defer srv.GracefulStop()

	pod := corruptImagePod("default", "app", "registry.example.com/app:v1", "node-1")
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "app-abc",
		UID:        "test-uid",
	}}

	agentPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tote-agent-node1",
			Namespace: "tote",
			Labels: map[string]string{
				"app.kubernetes.io/name":      "tote",
				"app.kubernetes.io/component": "agent",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			PodIP: "127.0.0.1",
		},
	}

	fixture := setupReconciler(optedInNamespace("default"), pod, agentPod)
	fixture.reconciler.AgentResolver = transfer.NewResolver(fixture.reconciler.Client, "tote", port)

	_, err = fixture.reconciler.Reconcile(context.Background(), reconcileRequest("default", "app"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify corrupt image event was emitted.
	select {
	case event := <-fixture.recorder.Events:
		if !strings.Contains(event, "Corrupt") {
			t.Errorf("expected corrupt image event, got: %s", event)
		}
	default:
		t.Error("expected a corrupt image event")
	}

	// Verify image tag was removed from store.
	digest, _ := store.ResolveTag(context.Background(), "registry.example.com/app:v1")
	if digest != "" {
		t.Errorf("expected tag to be removed, but ResolveTag returned %q", digest)
	}

	// Verify pod was deleted (owned pod).
	var deletedPod corev1.Pod
	getErr := fixture.reconciler.Client.Get(context.Background(), types.NamespacedName{Name: "app", Namespace: "default"}, &deletedPod)
	if getErr == nil {
		t.Error("expected pod to be deleted after corrupt image cleanup")
	}
}
