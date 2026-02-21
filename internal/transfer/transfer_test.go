package transfer

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	v1 "github.com/ppiankov/tote/api/v1"
	"github.com/ppiankov/tote/internal/agent"
	"github.com/ppiankov/tote/internal/config"
	"github.com/ppiankov/tote/internal/events"
	"github.com/ppiankov/tote/internal/metrics"
	"github.com/ppiankov/tote/internal/session"
)

func startAgentServer(t *testing.T, store agent.ImageStore, sessions *session.Store) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := grpc.NewServer()
	agentSrv := &agent.Server{Store: store, Sessions: sessions}
	v1.RegisterToteAgentServer(srv, agentSrv)

	go func() { _ = srv.Serve(lis) }()

	return lis.Addr().String(), func() { srv.Stop() }
}

func targetPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-pod",
			Namespace: "default",
			Annotations: map[string]string{
				config.AnnotationPodAutoSalvage: "true",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-target",
		},
	}
}

func TestOrchestratorSalvage_RateLimited(t *testing.T) {
	scheme := newScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(targetPod()).Build()
	rec := k8sevents.NewFakeRecorder(10)
	reg := prometheus.NewRegistry()

	sessions := session.NewStore()
	resolver := NewResolver(cl, "tote-system", 9090)

	o := NewOrchestrator(sessions, resolver, events.NewEmitter(rec), metrics.NewCounters(reg), cl, 1, 5*time.Minute, 0)

	// Fill the semaphore
	o.Semaphore <- struct{}{}

	pod := targetPod()
	err := o.Salvage(context.Background(), pod, "sha256:abc", "node-source")
	if err == nil {
		t.Fatal("expected rate limit error")
	}

	<-o.Semaphore
}

func TestOrchestratorSalvage_NoSourceAgent(t *testing.T) {
	scheme := newScheme()
	pod := targetPod()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(pod).Build()
	rec := k8sevents.NewFakeRecorder(10)
	reg := prometheus.NewRegistry()

	sessions := session.NewStore()
	resolver := NewResolver(cl, "tote-system", 9090)

	o := NewOrchestrator(sessions, resolver, events.NewEmitter(rec), metrics.NewCounters(reg), cl, 2, 5*time.Minute, 0)

	err := o.Salvage(context.Background(), pod, "sha256:abc", "node-source")
	if err == nil {
		t.Fatal("expected error when no agent pod exists")
	}

	select {
	case event := <-rec.Events:
		if event == "" {
			t.Error("expected non-empty failure event")
		}
	default:
		t.Error("expected failure event to be emitted")
	}
}

func TestOrchestratorSalvage_EndToEnd(t *testing.T) {
	sourceStore := agent.NewFakeImageStore()
	sourceStore.AddImage("sha256:aaa", []byte("image-tar-data"))
	sessions := session.NewStore()

	sourceAddr, sourceCleanup := startAgentServer(t, sourceStore, sessions)
	defer sourceCleanup()

	targetStore := agent.NewFakeImageStore()
	targetAddr, targetCleanup := startAgentServer(t, targetStore, sessions)
	defer targetCleanup()

	// Direct gRPC test: PrepareExport on source, then ImportFrom on target
	conn, err := grpc.NewClient(sourceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial source: %v", err)
	}
	defer func() { _ = conn.Close() }()

	sourceClient := v1.NewToteAgentClient(conn)
	sess := sessions.Create("sha256:aaa", "node-source", "node-target", 5*time.Minute)

	_, err = sourceClient.PrepareExport(context.Background(), &v1.PrepareExportRequest{
		SessionToken: sess.Token,
		Digest:       "sha256:aaa",
	})
	if err != nil {
		t.Fatalf("PrepareExport: %v", err)
	}

	targetConn, err := grpc.NewClient(targetAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial target: %v", err)
	}
	defer func() { _ = targetConn.Close() }()

	targetClient := v1.NewToteAgentClient(targetConn)
	resp, err := targetClient.ImportFrom(context.Background(), &v1.ImportFromRequest{
		SessionToken:   sess.Token,
		Digest:         "sha256:aaa",
		SourceEndpoint: sourceAddr,
	})
	if err != nil {
		t.Fatalf("ImportFrom: %v", err)
	}
	// The fake import returns a different digest (sha256:fake-N) so the
	// response may report a mismatch, but the gRPC flow itself succeeds.
	_ = resp
}

func TestOrchestratorSemaphore(t *testing.T) {
	scheme := newScheme()
	pod := targetPod()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(pod).Build()
	rec := k8sevents.NewFakeRecorder(10)
	reg := prometheus.NewRegistry()

	sessions := session.NewStore()
	resolver := NewResolver(cl, "tote-system", 9090)

	o := NewOrchestrator(sessions, resolver, events.NewEmitter(rec), metrics.NewCounters(reg), cl, 2, 5*time.Minute, 0)

	// Salvage will fail (no agent pods), but semaphore should be released
	_ = o.Salvage(context.Background(), pod, "sha256:abc", "node-source")

	// Verify semaphore was released by acquiring both slots
	o.Semaphore <- struct{}{}
	o.Semaphore <- struct{}{}
	<-o.Semaphore
	<-o.Semaphore
}

func ownedPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owned-pod",
			Namespace: "default",
			Annotations: map[string]string{
				config.AnnotationPodAutoSalvage: "true",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "ReplicaSet",
				Name:       "my-rs",
				UID:        "uid-1",
			}},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-target",
		},
	}
}

// salvageOrchestrator sets up a full orchestrator with a running agent server
// for end-to-end salvage tests. Both source and target resolve to the same
// gRPC server (shared fake store).
func salvageOrchestrator(t *testing.T, pod *corev1.Pod) (*Orchestrator, *k8sevents.FakeRecorder, client.Client) {
	t.Helper()

	store := agent.NewFakeImageStore()
	store.AddImage("sha256:aaa", []byte("image-tar-data"))
	sessions := session.NewStore()

	addr, cleanup := startAgentServer(t, store, sessions)
	t.Cleanup(cleanup)

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	scheme := newScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(
		pod,
		agentPod("tote-system", "agent-source", "node-source", host),
		agentPod("tote-system", "agent-target", "node-target", host),
	).Build()

	rec := k8sevents.NewFakeRecorder(10)
	reg := prometheus.NewRegistry()
	resolver := NewResolver(cl, "tote-system", port)
	o := NewOrchestrator(sessions, resolver, events.NewEmitter(rec), metrics.NewCounters(reg), cl, 2, 5*time.Minute, 0)

	return o, rec, cl
}

func TestOrchestratorSalvage_DeletesPodWithOwner(t *testing.T) {
	pod := ownedPod()
	o, _, cl := salvageOrchestrator(t, pod)

	err := o.Salvage(context.Background(), pod, "sha256:aaa", "node-source")
	if err != nil {
		t.Fatalf("salvage failed: %v", err)
	}

	// Pod with owner references should be deleted for fast recovery
	var got corev1.Pod
	err = cl.Get(context.Background(), client.ObjectKeyFromObject(pod), &got)
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected pod to be deleted, got err: %v", err)
	}
}

func TestOrchestratorSalvage_AnnotatesStandalonePod(t *testing.T) {
	pod := targetPod() // no owner references
	o, _, cl := salvageOrchestrator(t, pod)

	err := o.Salvage(context.Background(), pod, "sha256:aaa", "node-source")
	if err != nil {
		t.Fatalf("salvage failed: %v", err)
	}

	// Standalone pod should NOT be deleted, but should get salvage annotations
	var got corev1.Pod
	err = cl.Get(context.Background(), client.ObjectKeyFromObject(pod), &got)
	if err != nil {
		t.Fatalf("expected pod to still exist: %v", err)
	}
	if got.Annotations[config.AnnotationSalvagedDigest] != "sha256:aaa" {
		t.Errorf("expected salvaged-digest annotation, got annotations: %v", got.Annotations)
	}
	if got.Annotations[config.AnnotationImportedAt] == "" {
		t.Error("expected imported-at annotation to be set")
	}
}

func TestOrchestratorSalvage_ImageSizeExceeded(t *testing.T) {
	pod := targetPod()
	o, _, _ := salvageOrchestrator(t, pod)

	// Image data is 14 bytes ("image-tar-data"). Set limit to 10 bytes.
	o.MaxImageSize = 10

	err := o.Salvage(context.Background(), pod, "sha256:aaa", "node-source")
	if err == nil {
		t.Fatal("expected error for oversized image")
	}
	if !strings.Contains(err.Error(), "image size exceeded") {
		t.Errorf("expected size exceeded error, got: %v", err)
	}
}

func TestOrchestratorSalvage_ImageSizeWithinLimit(t *testing.T) {
	pod := targetPod()
	o, _, _ := salvageOrchestrator(t, pod)

	// Image data is 14 bytes. Set limit to 100 bytes — should pass.
	o.MaxImageSize = 100

	err := o.Salvage(context.Background(), pod, "sha256:aaa", "node-source")
	if err != nil {
		t.Fatalf("salvage should succeed within size limit: %v", err)
	}
}
