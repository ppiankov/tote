package transfer

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	v1 "github.com/ppiankov/tote/api/v1"
	v1alpha1 "github.com/ppiankov/tote/api/v1alpha1"
	"github.com/ppiankov/tote/internal/events"
	"github.com/ppiankov/tote/internal/metrics"
	"github.com/ppiankov/tote/internal/notify"
	"github.com/ppiankov/tote/internal/registry"
	"github.com/ppiankov/tote/internal/session"
)

// Orchestrator coordinates image salvage between agent nodes.
type Orchestrator struct {
	Sessions     *session.Store
	Resolver     *Resolver
	Emitter      *events.Emitter
	Metrics      *metrics.Counters
	Client       client.Client
	Semaphore    chan struct{}
	SessionTTL   time.Duration
	MaxImageSize int64

	// Backup registry push (optional).
	BackupRegistry         string
	BackupRegistrySecret   string
	BackupRegistryInsecure bool
	SecretNamespace        string

	TransportCreds credentials.TransportCredentials // nil = insecure
	Notifier       *notify.Notifier
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
	maxImageSize int64,
) *Orchestrator {
	return &Orchestrator{
		Sessions:     sessions,
		Resolver:     resolver,
		Emitter:      emitter,
		Metrics:      m,
		Client:       c,
		Semaphore:    make(chan struct{}, maxConcurrent),
		SessionTTL:   sessionTTL,
		MaxImageSize: maxImageSize,
	}
}

// SetBackupRegistry configures optional registry push after salvage.
func (o *Orchestrator) SetBackupRegistry(reg, secret, namespace string, insec bool) {
	o.BackupRegistry = reg
	o.BackupRegistrySecret = secret
	o.BackupRegistryInsecure = insec
	o.SecretNamespace = namespace
}

// Salvage attempts to transfer an image from sourceNode to the pod's node.
// It is one-shot: on failure it emits an event but does not retry.
func (o *Orchestrator) Salvage(ctx context.Context, pod *corev1.Pod, digest, imageRef, sourceNode string) error {
	logger := log.FromContext(ctx)
	o.Metrics.RecordSalvageAttempt()
	start := time.Now()

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
	sizeBytes, err := o.prepareExport(ctx, sourceEndpoint, sess.Token, digest)
	if err != nil {
		o.fail(pod, digest, fmt.Sprintf("prepare export: %v", err))
		return err
	}

	// Check image size limit
	if o.MaxImageSize > 0 && sizeBytes > o.MaxImageSize {
		reason := fmt.Sprintf("image %s is %d bytes, exceeds limit %d bytes", digest, sizeBytes, o.MaxImageSize)
		o.fail(pod, digest, reason)
		return fmt.Errorf("image size exceeded: %s", reason)
	}

	// ImportFrom on target agent
	if err := o.importFrom(ctx, targetEndpoint, sess.Token, digest, sourceEndpoint); err != nil {
		o.fail(pod, digest, fmt.Sprintf("import: %v", err))
		return err
	}

	o.Metrics.RecordSalvageSuccess()
	o.Metrics.RecordSalvageDuration(time.Since(start))
	o.Emitter.EmitSalvaged(pod, digest, sourceNode, targetNode)
	if o.Notifier != nil {
		_ = o.Notifier.Notify(ctx, notify.Event{
			Type:       "salvaged",
			PodName:    pod.Name,
			Namespace:  pod.Namespace,
			Digest:     digest,
			SourceNode: sourceNode,
			TargetNode: targetNode,
		})
	}
	logger.Info("salvage complete", "digest", digest, "source", sourceNode, "target", targetNode)

	// Record the salvage as a CRD for persistent history.
	if err := o.createSalvageRecord(ctx, pod, digest, imageRef, sourceNode, targetNode, "Completed", ""); err != nil {
		logger.Error(err, "failed to create SalvageRecord")
	}

	// Optional: push to backup registry (non-fatal).
	if o.BackupRegistry != "" {
		o.pushToBackupRegistry(ctx, pod, digest, imageRef, sourceEndpoint, sourceNode)
	}

	// Delete the pod so the owning controller recreates it with the cached image.
	// Skip standalone pods (no owner) — they cannot be recreated automatically.
	if len(pod.OwnerReferences) > 0 {
		if err := o.Client.Delete(ctx, pod); err != nil {
			logger.Error(err, "failed to delete pod after salvage", "pod", pod.Name)
		} else {
			logger.Info("deleted pod for fast recovery", "pod", pod.Name, "namespace", pod.Namespace)
		}
	}

	return nil
}

func (o *Orchestrator) dialOption() grpc.DialOption {
	if o.TransportCreds != nil {
		return grpc.WithTransportCredentials(o.TransportCreds)
	}
	return grpc.WithTransportCredentials(insecure.NewCredentials())
}

func (o *Orchestrator) fail(pod *corev1.Pod, digest, reason string) {
	o.Metrics.RecordSalvageFailure()
	o.Emitter.EmitSalvageFailed(pod, digest, reason)
	if o.Notifier != nil {
		_ = o.Notifier.Notify(context.Background(), notify.Event{
			Type:      "salvage_failed",
			PodName:   pod.Name,
			Namespace: pod.Namespace,
			Digest:    digest,
			Error:     reason,
		})
	}
}

func (o *Orchestrator) prepareExport(ctx context.Context, endpoint, token, digest string) (int64, error) {
	conn, err := grpc.NewClient(endpoint, o.dialOption())
	if err != nil {
		return 0, fmt.Errorf("connecting to source: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client := v1.NewToteAgentClient(conn)
	resp, err := client.PrepareExport(ctx, &v1.PrepareExportRequest{
		SessionToken: token,
		Digest:       digest,
	})
	if err != nil {
		return 0, err
	}
	return resp.SizeBytes, nil
}

func (o *Orchestrator) importFrom(ctx context.Context, endpoint, token, digest, sourceEndpoint string) error {
	conn, err := grpc.NewClient(endpoint, o.dialOption())
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

// createSalvageRecord persists a SalvageRecord CR for tracking.
func (o *Orchestrator) createSalvageRecord(ctx context.Context, pod *corev1.Pod, digest, imageRef, sourceNode, targetNode, phase, errMsg string) error {
	// Extract short hex from digest (e.g. "sha256:abc123de..." -> "abc123de").
	shortDigest := digest
	if idx := strings.Index(digest, ":"); idx >= 0 {
		shortDigest = digest[idx+1:]
	}
	if len(shortDigest) > 8 {
		shortDigest = shortDigest[:8]
	}
	name := fmt.Sprintf("%s-%s", pod.Name, shortDigest)

	record := &v1alpha1.SalvageRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: pod.Namespace,
		},
		Spec: v1alpha1.SalvageRecordSpec{
			PodName:    pod.Name,
			Digest:     digest,
			ImageRef:   imageRef,
			SourceNode: sourceNode,
			TargetNode: targetNode,
		},
		Status: v1alpha1.SalvageRecordStatus{
			Phase:       phase,
			CompletedAt: time.Now().UTC().Format(time.RFC3339),
			Error:       errMsg,
		},
	}

	return o.Client.Create(ctx, record)
}

func (o *Orchestrator) pushToBackupRegistry(ctx context.Context, pod *corev1.Pod, digest, imageRef, sourceEndpoint, sourceNode string) {
	logger := log.FromContext(ctx)
	pushStart := time.Now()

	targetRef, err := registry.BackupRef(imageRef, o.BackupRegistry)
	if err != nil {
		logger.Error(err, "failed to construct backup ref", "image", imageRef)
		return
	}

	o.Metrics.RecordPushAttempt()

	username, password, err := o.loadRegistryCredentials(ctx)
	if err != nil {
		logger.Error(err, "failed to load registry credentials")
		o.Metrics.RecordPushFailure()
		o.Emitter.EmitPushFailed(pod, digest, targetRef, err.Error())
		return
	}

	if err := o.pushImage(ctx, sourceEndpoint, digest, targetRef, username, password); err != nil {
		logger.Error(err, "registry push failed (non-fatal)", "digest", digest, "target", targetRef)
		o.Metrics.RecordPushFailure()
		o.Emitter.EmitPushFailed(pod, digest, targetRef, err.Error())
		return
	}

	o.Metrics.RecordPushSuccess()
	o.Metrics.RecordPushDuration(time.Since(pushStart))
	o.Emitter.EmitPushed(pod, digest, targetRef, sourceNode)
	if o.Notifier != nil {
		_ = o.Notifier.Notify(ctx, notify.Event{
			Type:      "pushed",
			PodName:   pod.Name,
			Namespace: pod.Namespace,
			Digest:    digest,
		})
	}
	logger.Info("pushed to backup registry", "digest", digest, "target", targetRef, "source", sourceNode)
}

func (o *Orchestrator) loadRegistryCredentials(ctx context.Context) (string, string, error) {
	if o.BackupRegistrySecret == "" {
		return "", "", nil // anonymous push
	}
	var secret corev1.Secret
	key := client.ObjectKey{Namespace: o.SecretNamespace, Name: o.BackupRegistrySecret}
	if err := o.Client.Get(ctx, key, &secret); err != nil {
		return "", "", fmt.Errorf("reading secret %s/%s: %w", key.Namespace, key.Name, err)
	}
	data, ok := secret.Data[".dockerconfigjson"]
	if !ok {
		return "", "", fmt.Errorf("secret %s/%s missing .dockerconfigjson key", key.Namespace, key.Name)
	}
	return registry.ExtractCredentials(data, o.BackupRegistry)
}

func (o *Orchestrator) pushImage(ctx context.Context, endpoint, digest, targetRef, username, password string) error {
	conn, err := grpc.NewClient(endpoint, o.dialOption())
	if err != nil {
		return fmt.Errorf("connecting to source for push: %w", err)
	}
	defer func() { _ = conn.Close() }()

	resp, err := v1.NewToteAgentClient(conn).PushImage(ctx, &v1.PushImageRequest{
		Digest:           digest,
		TargetRef:        targetRef,
		RegistryUsername: username,
		RegistryPassword: password,
		Insecure:         o.BackupRegistryInsecure,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}
