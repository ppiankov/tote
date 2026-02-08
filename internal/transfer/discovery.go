package transfer

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resolver finds agent pod endpoints by node name.
type Resolver struct {
	Client    client.Reader
	Namespace string
	Port      int
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
