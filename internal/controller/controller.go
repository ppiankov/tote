package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/ppiankov/tote/internal/config"
	"github.com/ppiankov/tote/internal/detector"
	"github.com/ppiankov/tote/internal/events"
	"github.com/ppiankov/tote/internal/inventory"
	"github.com/ppiankov/tote/internal/metrics"
	"github.com/ppiankov/tote/internal/resolver"
	"github.com/ppiankov/tote/internal/transfer"
)

// PodReconciler watches Pods for image pull failures.
type PodReconciler struct {
	Client        client.Client
	Config        config.Config
	Finder        *inventory.Finder
	Emitter       *events.Emitter
	Metrics       *metrics.Counters
	Orchestrator  *transfer.Orchestrator
	AgentResolver *transfer.Resolver
}

// Reconcile handles a single Pod reconciliation.
func (r *PodReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	if !r.Config.Enabled {
		return reconcile.Result{}, nil
	}

	if r.Config.IsDenied(req.Namespace) {
		return reconcile.Result{}, nil
	}

	var pod corev1.Pod
	if err := r.Client.Get(ctx, req.NamespacedName, &pod); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if !namespaceOptedIn(ctx, r.Client, pod.Namespace) {
		return reconcile.Result{}, nil
	}

	if pod.Annotations[config.AnnotationPodAutoSalvage] != "true" {
		return reconcile.Result{}, nil
	}

	failures := detector.Detect(&pod)
	if len(failures) == 0 {
		return reconcile.Result{}, nil
	}

	for _, f := range failures {
		r.Metrics.RecordDetected()

		res := resolver.Resolve(f.Image)

		var digest string
		var nodes []string

		if res.Actionable {
			digest = res.Digest
			var err error
			nodes, err = r.Finder.FindNodes(ctx, digest)
			if err != nil {
				logger.Error(err, "failed to find nodes with image", "digest", digest)
				return reconcile.Result{}, err
			}
		} else {
			// Tag-only: try to resolve via Node.Status.Images first.
			var err error
			digest, nodes, err = r.Finder.FindNodesByTag(ctx, f.Image)
			if err != nil {
				logger.Error(err, "failed to resolve tag", "image", f.Image)
				return reconcile.Result{}, err
			}

			// Fallback: query agents directly via containerd (bypasses kubelet 50-image limit).
			if digest == "" && r.AgentResolver != nil {
				logger.Info("querying agents for tag resolution", "container", f.ContainerName, "image", f.Image)
				var sourceNode string
				digest, sourceNode, err = r.AgentResolver.ResolveTagViaAgents(ctx, f.Image)
				if err != nil {
					logger.Error(err, "agent tag resolution failed", "image", f.Image)
				}
				if digest != "" && sourceNode != "" {
					nodes = []string{sourceNode}
					logger.Info("resolved tag via agent", "container", f.ContainerName, "image", f.Image, "digest", digest, "node", sourceNode)
				} else if err == nil {
					logger.Info("agents returned no digest", "container", f.ContainerName, "image", f.Image)
				}
			}

			if digest == "" {
				logger.Info("image not actionable (tag-only, no cached digest found)", "container", f.ContainerName, "image", f.Image)
				r.Metrics.RecordNotActionable()
				r.Emitter.EmitNotActionable(&pod, f.Image)
				continue
			}
			if len(nodes) == 0 {
				logger.Info("resolved tag to digest via node cache", "container", f.ContainerName, "image", f.Image, "digest", digest)
			}
		}

		if len(nodes) > 0 {
			logger.Info("image salvageable", "container", f.ContainerName, "digest", digest, "nodes", nodes)
			r.Metrics.RecordSalvageable()
			r.Emitter.EmitSalvageable(&pod, f.Image, nodes)

			if r.Orchestrator != nil && pod.Spec.NodeName != "" {
				if pod.Annotations[config.AnnotationSalvagedDigest] == digest {
					continue
				}
				// Pick a source node that isn't the target node.
				sourceNode := ""
				for _, n := range nodes {
					if n != pod.Spec.NodeName {
						sourceNode = n
						break
					}
				}
				if sourceNode == "" {
					logger.Info("image already on target node, skipping salvage", "digest", digest, "node", pod.Spec.NodeName)
					continue
				}
				if err := r.Orchestrator.Salvage(ctx, &pod, digest, sourceNode); err != nil {
					logger.Error(err, "salvage failed", "digest", digest)
				}
			}
		}
	}

	return reconcile.Result{}, nil
}

// SetupWithManager registers the reconciler with the controller manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}

func namespaceOptedIn(ctx context.Context, c client.Reader, namespace string) bool {
	var ns corev1.Namespace
	if err := c.Get(ctx, types.NamespacedName{Name: namespace}, &ns); err != nil {
		return false
	}
	return ns.Annotations[config.AnnotationNamespaceAllow] == "true"
}
