package transfer

import (
	"context"
	"fmt"
	"strings"
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
	o.Emitter.EmitSalvaged(pod, digest, sourceNode, targetNode)
	logger.Info("salvage complete", "digest", digest, "source", sourceNode, "target", targetNode)

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

func (o *Orchestrator) prepareExport(ctx context.Context, endpoint, token, digest string) (int64, error) {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
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

func (o *Orchestrator) pushToBackupRegistry(ctx context.Context, pod *corev1.Pod, digest, imageRef, sourceEndpoint, sourceNode string) {
	logger := log.FromContext(ctx)

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
	o.Emitter.EmitPushed(pod, digest, targetRef, sourceNode)
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
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
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

// findImageRef looks up the original image reference from the pod spec.
func findImageRef(pod *corev1.Pod, digest string) string {
	for _, c := range pod.Spec.Containers {
		if strings.Contains(c.Image, digest) {
			return c.Image
		}
	}
	for _, c := range pod.Spec.InitContainers {
		if strings.Contains(c.Image, digest) {
			return c.Image
		}
	}
	if len(pod.Spec.Containers) == 1 {
		return pod.Spec.Containers[0].Image
	}
	return ""
}
