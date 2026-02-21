package transfer

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/ppiankov/tote/api/v1"
)

// Resolver finds agent pod endpoints by node name.
type Resolver struct {
	Client         client.Reader
	Namespace      string
	Port           int
	TransportCreds credentials.TransportCredentials // nil = insecure
}

// NewResolver creates a Resolver that looks up agent pods in the given namespace.
func NewResolver(c client.Reader, namespace string, port int) *Resolver {
	return &Resolver{Client: c, Namespace: namespace, Port: port}
}

// EndpointForNode returns the gRPC endpoint (ip:port) for the agent running on
// the given node. It finds pods matching the tote agent labels whose
// Spec.NodeName matches.
func (r *Resolver) EndpointForNode(ctx context.Context, nodeName string) (string, error) {
	var pods corev1.PodList
	if err := r.Client.List(ctx, &pods,
		client.InNamespace(r.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":      "tote",
			"app.kubernetes.io/component": "agent",
		},
	); err != nil {
		return "", fmt.Errorf("listing agent pods: %w", err)
	}

	for _, pod := range pods.Items {
		if pod.Spec.NodeName == nodeName && pod.Status.PodIP != "" {
			return fmt.Sprintf("%s:%d", pod.Status.PodIP, r.Port), nil
		}
	}

	return "", fmt.Errorf("no agent pod found on node %s", nodeName)
}

// ResolveTagViaAgents queries all agent pods to resolve an image tag to a
// digest. Returns the digest and the node name where it was found.
// Returns empty strings if no agent has the image.
func (r *Resolver) ResolveTagViaAgents(ctx context.Context, imageRef string) (string, string, error) {
	var pods corev1.PodList
	if err := r.Client.List(ctx, &pods,
		client.InNamespace(r.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":      "tote",
			"app.kubernetes.io/component": "agent",
		},
	); err != nil {
		return "", "", fmt.Errorf("listing agent pods: %w", err)
	}

	logger := log.FromContext(ctx)
	logger.V(1).Info("querying agents for tag", "image", imageRef, "agentCount", len(pods.Items))

	for _, pod := range pods.Items {
		if pod.Status.PodIP == "" {
			continue
		}
		endpoint := fmt.Sprintf("%s:%d", pod.Status.PodIP, r.Port)
		digest, err := r.resolveTagFromAgent(ctx, endpoint, imageRef)
		if err != nil {
			logger.V(1).Info("agent ResolveTag failed", "endpoint", endpoint, "node", pod.Spec.NodeName, "error", err)
			continue
		}
		if digest == "" {
			continue
		}
		return digest, pod.Spec.NodeName, nil
	}
	return "", "", nil
}

func (r *Resolver) dialOption() grpc.DialOption {
	if r.TransportCreds != nil {
		return grpc.WithTransportCredentials(r.TransportCreds)
	}
	return grpc.WithTransportCredentials(insecure.NewCredentials())
}

// RemoveImageOnNode calls the agent on the given node to remove an image record.
func (r *Resolver) RemoveImageOnNode(ctx context.Context, nodeName, imageRef string) error {
	endpoint, err := r.EndpointForNode(ctx, nodeName)
	if err != nil {
		return err
	}
	conn, err := grpc.NewClient(endpoint, r.dialOption())
	if err != nil {
		return fmt.Errorf("connecting to agent: %w", err)
	}
	defer func() { _ = conn.Close() }()

	_, err = v1.NewToteAgentClient(conn).RemoveImage(ctx, &v1.RemoveImageRequest{ImageRef: imageRef})
	return err
}

func (r *Resolver) resolveTagFromAgent(ctx context.Context, endpoint, imageRef string) (string, error) {
	conn, err := grpc.NewClient(endpoint, r.dialOption())
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()

	resp, err := v1.NewToteAgentClient(conn).ResolveTag(ctx, &v1.ResolveTagRequest{ImageRef: imageRef})
	if err != nil {
		return "", err
	}
	return resp.Digest, nil
}
