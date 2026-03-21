package doctor

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
)

// Status represents the outcome of a single check.
type Status string

const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

// Check is a single health check result.
type Check struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Message string `json:"message"`
}

// Result is the full doctor output.
type Result struct {
	Checks []Check `json:"checks"`
	OK     bool    `json:"ok"`
}

// Run executes all health checks against the cluster.
func Run(ctx context.Context, clientset kubernetes.Interface, namespace string) Result {
	var checks []Check
	allOK := true

	checks = append(checks, checkKubeconfig(ctx, clientset))
	checks = append(checks, checkCRD(ctx, clientset))
	checks = append(checks, checkController(ctx, clientset, namespace))
	checks = append(checks, checkAgents(ctx, clientset, namespace))
	checks = append(checks, checkNamespaces(ctx, clientset))

	for _, c := range checks {
		if c.Status == StatusFail {
			allOK = false
		}
	}

	return Result{Checks: checks, OK: allOK}
}

func checkKubeconfig(ctx context.Context, clientset kubernetes.Interface) Check {
	_, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return Check{Name: "kubeconfig", Status: StatusFail, Message: fmt.Sprintf("cluster unreachable: %v", err)}
	}
	return Check{Name: "kubeconfig", Status: StatusOK, Message: "cluster reachable"}
}

func checkCRD(ctx context.Context, clientset kubernetes.Interface) Check {
	dc, ok := clientset.Discovery().(*discovery.DiscoveryClient)
	if !ok {
		// Fake clients don't have a real discovery client; check API resources instead.
		return checkCRDViaResources(ctx, clientset)
	}
	resources, err := dc.ServerResourcesForGroupVersion("tote.dev/v1alpha1")
	if err != nil {
		return Check{Name: "crd", Status: StatusFail, Message: "salvagerecords.tote.dev CRD not installed"}
	}
	for _, r := range resources.APIResources {
		if r.Kind == "SalvageRecord" {
			return Check{Name: "crd", Status: StatusOK, Message: "salvagerecords.tote.dev installed"}
		}
	}
	return Check{Name: "crd", Status: StatusFail, Message: "SalvageRecord kind not found in tote.dev/v1alpha1"}
}

func checkCRDViaResources(ctx context.Context, clientset kubernetes.Interface) Check {
	_, resources, err := clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		return Check{Name: "crd", Status: StatusFail, Message: fmt.Sprintf("cannot discover resources: %v", err)}
	}
	gv := schema.GroupVersion{Group: "tote.dev", Version: "v1alpha1"}.String()
	for _, rl := range resources {
		if rl.GroupVersion == gv {
			for _, r := range rl.APIResources {
				if r.Kind == "SalvageRecord" {
					return Check{Name: "crd", Status: StatusOK, Message: "salvagerecords.tote.dev installed"}
				}
			}
		}
	}
	return Check{Name: "crd", Status: StatusFail, Message: "salvagerecords.tote.dev CRD not installed"}
}

func checkController(ctx context.Context, clientset kubernetes.Interface, namespace string) Check {
	deploys, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=tote,app.kubernetes.io/component!=agent",
	})
	if err != nil {
		return Check{Name: "controller", Status: StatusFail, Message: fmt.Sprintf("cannot list deployments: %v", err)}
	}

	// Fallback: try without component filter.
	if len(deploys.Items) == 0 {
		deploys, err = clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=tote",
		})
		if err != nil || len(deploys.Items) == 0 {
			return Check{Name: "controller", Status: StatusFail, Message: fmt.Sprintf("no tote deployment found in %s", namespace)}
		}
		// Filter out agent-related deployments.
		filtered := make([]appsv1.Deployment, 0)
		for _, d := range deploys.Items {
			if d.Labels["app.kubernetes.io/component"] != "agent" {
				filtered = append(filtered, d)
			}
		}
		if len(filtered) == 0 {
			return Check{Name: "controller", Status: StatusFail, Message: fmt.Sprintf("no tote controller deployment found in %s", namespace)}
		}
		deploys.Items = filtered
	}

	d := deploys.Items[0]
	ready := d.Status.ReadyReplicas
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	if ready < desired {
		return Check{Name: "controller", Status: StatusFail, Message: fmt.Sprintf("%d/%d replicas ready", ready, desired)}
	}
	return Check{Name: "controller", Status: StatusOK, Message: fmt.Sprintf("%d/%d replicas ready", ready, desired)}
}

func checkAgents(ctx context.Context, clientset kubernetes.Interface, namespace string) Check {
	dsList, err := clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=tote,app.kubernetes.io/component=agent",
	})
	if err != nil {
		return Check{Name: "agents", Status: StatusFail, Message: fmt.Sprintf("cannot list daemonsets: %v", err)}
	}
	if len(dsList.Items) == 0 {
		return Check{Name: "agents", Status: StatusWarn, Message: fmt.Sprintf("no agent DaemonSet found in %s (salvage disabled)", namespace)}
	}
	ds := dsList.Items[0]
	ready := ds.Status.NumberReady
	desired := ds.Status.DesiredNumberScheduled
	if ready < desired {
		return Check{Name: "agents", Status: StatusFail, Message: fmt.Sprintf("%d/%d agents ready", ready, desired)}
	}
	return Check{Name: "agents", Status: StatusOK, Message: fmt.Sprintf("%d/%d agents ready", ready, desired)}
}

func checkNamespaces(ctx context.Context, clientset kubernetes.Interface) Check {
	nsList, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsForbidden(err) {
			return Check{Name: "namespaces", Status: StatusWarn, Message: "cannot list namespaces (RBAC)"}
		}
		return Check{Name: "namespaces", Status: StatusFail, Message: fmt.Sprintf("cannot list namespaces: %v", err)}
	}
	var opted []string
	for _, ns := range nsList.Items {
		if ns.Annotations["tote.dev/allow"] == "true" {
			opted = append(opted, ns.Name)
		}
	}
	if len(opted) == 0 {
		return Check{Name: "namespaces", Status: StatusWarn, Message: "no namespaces opted in (annotate with tote.dev/allow=true)"}
	}
	return Check{Name: "namespaces", Status: StatusOK, Message: fmt.Sprintf("%d namespaces opted in: %s", len(opted), joinMax(opted, 5))}
}

func joinMax(items []string, max int) string {
	if len(items) <= max {
		s := ""
		for i, item := range items {
			if i > 0 {
				s += ", "
			}
			s += item
		}
		return s
	}
	s := ""
	for i := 0; i < max; i++ {
		if i > 0 {
			s += ", "
		}
		s += items[i]
	}
	return fmt.Sprintf("%s (and %d more)", s, len(items)-max)
}
