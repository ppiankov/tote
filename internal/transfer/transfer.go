package transfer

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	v1 "github.com/ppiankov/tote/api/v1"
	"github.com/ppiankov/tote/internal/config"
	"github.com/ppiankov/tote/internal/events"
	"github.com/ppiankov/tote/internal/metrics"
	"github.com/ppiankov/tote/internal/session"
)

// Orchestrator coordinates image salvage between agent nodes.
type Orchestrator struct {
	Sessions   *session.Store
	Resolver   *Resolver
	Emitter    *events.Emitter
	Metrics    *metrics.Counters
	Client     client.Client
	Semaphore  chan struct{}
	SessionTTL time.Duration
}

// NewOrchestrator creates an Orchestrator with the given dependencies.
func NewOrchestrator(
	sessions *session.Store,
	resolver *Resolver,
	emitter *events.Emitter,
	m *metrics.Counters,
	c client.Client,
	maxConcurrent int,
	sessionTTL time.Duration,
) *Orchestrator {
	return &Orchestrator{
		Sessions:   sessions,
		Resolver:   resolver,
		Emitter:    emitter,
		Metrics:    m,
		Client:     c,
		Semaphore:  make(chan struct{}, maxConcurrent),
		SessionTTL: sessionTTL,
	}
}

// Salvage attempts to transfer an image from sourceNode to the pod's node.
// It is one-shot: on failure it emits an event but does not retry.
func (o *Orchestrator) Salvage(ctx context.Context, pod *corev1.Pod, digest, sourceNode string) error {
	logger := log.FromContext(ctx)
	o.Metrics.RecordSalvageAttempt()

	targetNode := pod.Spec.NodeName

	// Acquire semaphore (non-blocking)
	select {
	case o.Semaphore <- struct{}{}:
		defer func() { <-o.Semaphore }()
	default:
		logger.Info("salvage rate limited", "digest", digest)
		return fmt.Errorf("rate limited: max concurrent salvages reached")
	}

	// Resolve agent endpoints
	sourceEndpoint, err := o.Resolver.EndpointForNode(ctx, sourceNode)
	if err != nil {
		o.fail(pod, digest, fmt.Sprintf("resolving source agent: %v", err))
		return err
	}
	targetEndpoint, err := o.Resolver.EndpointForNode(ctx, targetNode)
	if err != nil {
		o.fail(pod, digest, fmt.Sprintf("resolving target agent: %v", err))
		return err
	}

	// Create session
	sess := o.Sessions.Create(digest, sourceNode, targetNode, o.SessionTTL)
	defer o.Sessions.Delete(sess.Token)

	// PrepareExport on source agent
	if err := o.prepareExport(ctx, sourceEndpoint, sess.Token, digest); err != nil {
		o.fail(pod, digest, fmt.Sprintf("prepare export: %v", err))
		return err
	}

	// ImportFrom on target agent
	if err := o.importFrom(ctx, targetEndpoint, sess.Token, digest, sourceEndpoint); err != nil {
		o.fail(pod, digest, fmt.Sprintf("import: %v", err))
		return err
	}

	o.Metrics.RecordSalvageSuccess()
	o.Emitter.EmitSalvaged(pod, digest, sourceNode, targetNode)
	logger.Info("salvage complete", "digest", digest, "source", sourceNode, "target", targetNode)

	// Delete the pod so the owning controller recreates it with the cached image.
	// Skip standalone pods (no owner) — they cannot be recreated automatically.
	if len(pod.OwnerReferences) > 0 {
		if err := o.Client.Delete(ctx, pod); err != nil {
			logger.Error(err, "failed to delete pod after salvage", "pod", pod.Name)
		} else {
			logger.Info("deleted pod for fast recovery", "pod", pod.Name, "namespace", pod.Namespace)
		}
	} else {
		// Standalone pod: mark as salvaged so we don't retry.
		if err := o.patchPodAnnotation(ctx, pod, digest); err != nil {
			logger.Error(err, "failed to patch pod annotation after salvage")
		}
	}

	return nil
}

func (o *Orchestrator) fail(pod *corev1.Pod, digest, reason string) {
	o.Metrics.RecordSalvageFailure()
	o.Emitter.EmitSalvageFailed(pod, digest, reason)
}

func (o *Orchestrator) prepareExport(ctx context.Context, endpoint, token, digest string) error {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connecting to source: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client := v1.NewToteAgentClient(conn)
	_, err = client.PrepareExport(ctx, &v1.PrepareExportRequest{
		SessionToken: token,
		Digest:       digest,
	})
	return err
}

func (o *Orchestrator) importFrom(ctx context.Context, endpoint, token, digest, sourceEndpoint string) error {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connecting to target: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client := v1.NewToteAgentClient(conn)
	resp, err := client.ImportFrom(ctx, &v1.ImportFromRequest{
		SessionToken:   token,
		Digest:         digest,
		SourceEndpoint: sourceEndpoint,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

func (o *Orchestrator) patchPodAnnotation(ctx context.Context, pod *corev1.Pod, digest string) error {
	patch := client.MergeFrom(pod.DeepCopy())
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[config.AnnotationSalvagedDigest] = digest
	pod.Annotations[config.AnnotationImportedAt] = time.Now().UTC().Format(time.RFC3339)
	return o.Client.Patch(ctx, pod, patch)
}
