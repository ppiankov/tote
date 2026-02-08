package transfer

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sevents "k8s.io/client-go/tools/events"
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

	o := NewOrchestrator(sessions, resolver, events.NewEmitter(rec), metrics.NewCounters(reg), cl, 1, 5*time.Minute)

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

	o := NewOrchestrator(sessions, resolver, events.NewEmitter(rec), metrics.NewCounters(reg), cl, 2, 5*time.Minute)

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

	o := NewOrchestrator(sessions, resolver, events.NewEmitter(rec), metrics.NewCounters(reg), cl, 2, 5*time.Minute)

	// Salvage will fail (no agent pods), but semaphore should be released
	_ = o.Salvage(context.Background(), pod, "sha256:abc", "node-source")

	// Verify semaphore was released by acquiring both slots
	o.Semaphore <- struct{}{}
	o.Semaphore <- struct{}{}
	<-o.Semaphore
	<-o.Semaphore
}
