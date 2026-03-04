# Troubleshooting

Debug guide for tote. Follows the controller reconciliation flow — work through each section in order.

## 1. Prerequisites

Before anything else, verify tote is running and configured.

```sh
# Controller running?
kubectl get deploy -A | grep tote

# Agents running? (required for salvage)
kubectl get ds -A | grep tote

# Controller logs (recent)
kubectl logs -n tote-system deploy/tote-controller --tail=100

# Agent logs (pick a node)
kubectl logs -n tote-system ds/tote-agent --tail=100
```

**Common issues:**

| Symptom | Cause | Fix |
|---------|-------|-----|
| Controller pod not running | Helm install incomplete | `helm install tote ppiankov/tote` |
| Agent pods missing | Agent not enabled | `helm upgrade tote ppiankov/tote --set agent.enabled=true` |
| Controller CrashLoopBackOff | Missing CRD | `kubectl apply -f charts/tote/crds/` |
| RBAC 403 errors in logs | Insufficient permissions | Check ClusterRole has `list`, `watch`, `get` for pods, nodes, deployments, replicasets, statefulsets, daemonsets, jobs, salvagerecords |

## 2. Namespace opt-in

tote requires **double opt-in**: namespace annotation AND pod/owner annotation.

```sh
# Check namespace annotation
kubectl get ns my-namespace -o jsonpath='{.metadata.annotations.tote\.dev/allow}'
# Must print: true
```

If missing:

```sh
kubectl annotate namespace my-namespace tote.dev/allow=true
```

**Hard-denied namespaces** (cannot be overridden): `kube-system`, `kube-public`, `kube-node-lease`.

## 3. Pod opt-in

```sh
# Check pod annotation
kubectl get pod myapp-abc123 -n my-namespace -o jsonpath='{.metadata.annotations.tote\.dev/auto-salvage}'
# Must print: true
```

The annotation can live on the pod template or any owner in the chain (Deployment, StatefulSet, DaemonSet, Job, ReplicaSet). tote walks ownerReferences up to 2 levels.

```sh
# Check on the Deployment instead
kubectl get deploy myapp -n my-namespace -o jsonpath='{.metadata.annotations.tote\.dev/auto-salvage}'
```

## 4. Detection

tote recognizes three failure reasons:

| Container waiting reason | Action |
|--------------------------|--------|
| `ImagePullBackOff` | Salvage attempt |
| `ErrImagePull` | Salvage attempt |
| `CreateContainerError` (with rootfs resolution failure) | Corrupt image cleanup + pod restart |

```sh
# Verify the pod is in a recognized failure state
kubectl get pod myapp-abc123 -n my-namespace -o jsonpath='{.status.containerStatuses[*].state.waiting.reason}'
```

If the reason is something else (e.g., `CrashLoopBackOff`, `ContainerCreating`), tote will not act.

## 5. Image resolution

This is the most common point of failure. tote needs to find the image **cached on another node**.

### Digest vs tag

```sh
# Check what image reference the pod uses
kubectl get pod myapp-abc123 -n my-namespace -o jsonpath='{.spec.containers[0].image}'
```

- **Digest reference** (`registry/repo@sha256:abc...`): tote searches `Node.Status.Images` directly. Most reliable.
- **Tag reference** (`registry/repo:tag`): tote must resolve the tag to a digest. Goes through two fallback steps.

### Tag resolution flow

**Step 1 — Node.Status.Images lookup:**

kubelet reports only the **top 50 images by size** per node. If your image isn't in the top 50, it won't appear here.

```sh
# Check if the image appears in any node's image list
kubectl get nodes -o json | \
  jq -r '.items[] | .metadata.name as $n | .status.images[].names[] | select(contains("myapp")) | "\($n): \(.)"'
```

**Step 2 — Agent fallback (containerd query):**

If step 1 finds nothing, tote queries agents on every node via gRPC to resolve the tag directly against containerd. This bypasses the 50-image limit.

Requires:
- `--agent-namespace` flag set on the controller
- Agent DaemonSet running and healthy
- Network connectivity between controller and agents (port 9090 by default)

```sh
# Verify agents are ready
kubectl get pods -n tote-system -l app=tote-agent -o wide
```

**If both steps return nothing**, the controller logs:

```
image not actionable (tag-only, no cached digest found)
```

This means the image is **not cached on any node in the cluster**. tote cannot salvage what doesn't exist — the image must have been pulled successfully at least once on some node.

### Enable verbose logging

Tag resolution details are logged at verbosity level 1. To see them:

```sh
# Controller flag
--zap-log-level=1
```

Look for these log lines:
- `querying agents for tag resolution` — agent fallback triggered
- `resolved tag via agent` — agent found the image
- `agents returned no digest` — agent query succeeded but image not found

## 6. Salvage

Once a source node is found, the controller orchestrates the transfer.

**Common salvage failures:**

| Log message | Cause | Fix |
|-------------|-------|-----|
| `salvage rate limited` | Hit `--max-concurrent-salvages` (default: 2) | Wait, or increase the limit |
| `image ... exceeds limit ... bytes` | Image larger than `--max-image-size` (default: 2 GiB) | Increase limit or set to 0 (unlimited) |
| `image already on target node, skipping salvage` | Image exists on the pod's node already — pull failure is likely auth/network, not cache | Fix registry access |
| `connection refused` / `Unavailable` | Agent unreachable on source or target node | Check agent pods, network policies, mTLS config |
| `salvage failed` | Generic — check full error message | Enable verbose logging |

**Idempotency:** tote checks for an existing `SalvageRecord` before attempting salvage. If a completed record exists for the same digest in the namespace, salvage is skipped.

```sh
# Check existing SalvageRecords
kubectl get salvagerecords -n my-namespace
```

## 7. Post-salvage

After successful salvage:

1. **Pod restart**: tote deletes the pod (only if it has ownerReferences) so the owning controller recreates it with the now-cached image
2. **SalvageRecord**: a CRD record is created with the salvage details
3. **Backup registry push** (if `--backup-registry` is configured): pushes to the backup registry for durability

```sh
# Verify SalvageRecord was created
kubectl get salvagerecords -n my-namespace -o yaml

# Check pod events for tote activity
kubectl describe pod myapp-abc123 -n my-namespace | grep -A2 tote
```

## 8. Quick reference

### Annotations

| Annotation | Target | Required |
|------------|--------|----------|
| `tote.dev/allow: "true"` | Namespace | Yes |
| `tote.dev/auto-salvage: "true"` | Pod or owner (Deployment, StatefulSet, DaemonSet, Job) | Yes |

### Kubernetes events on pods

| Event reason | Meaning |
|-------------|---------|
| `ImageSalvageable` | Image digest found on other nodes, salvage will be attempted |
| `ImageNotActionable` | Tag-only image, no cached digest found anywhere |
| `ImageSalvaged` | Transfer completed successfully |
| `ImageSalvageFailed` | Transfer attempted but failed |
| `ImageCorrupt` | Stale image record with missing blobs, cleaning up |
| `ImagePushed` | Pushed to backup registry |
| `ImagePushFailed` | Backup registry push failed (non-fatal) |

### Prometheus metrics

| Metric | What it tells you |
|--------|-------------------|
| `tote_detected_failures_total` | Total failures seen — if 0, check prerequisites |
| `tote_salvageable_images_total` | Failures where a cached copy was found |
| `tote_not_actionable_total` | Tag-only images with no cached digest |
| `tote_salvage_attempts_total` | Salvage operations started |
| `tote_salvage_successes_total` | Successful salvages |
| `tote_salvage_failures_total` | Failed salvages |
| `tote_salvage_duration_seconds` | Transfer time histogram |
| `tote_corrupt_images_total` | Corrupt image cleanups |
| `tote_push_attempts_total` | Backup registry push attempts |
| `tote_push_successes_total` | Successful pushes |
| `tote_push_failures_total` | Failed pushes |
| `tote_push_duration_seconds` | Push time histogram |

### Decision tree

```
Pod in ImagePullBackOff/ErrImagePull
  → Namespace has tote.dev/allow=true?
    NO → annotate namespace
    YES ↓
  → Pod/owner has tote.dev/auto-salvage=true?
    NO → annotate pod or deployment
    YES ↓
  → tote_detected_failures_total incrementing?
    NO → controller not running or not watching this namespace
    YES ↓
  → tote_salvageable_images_total incrementing?
    NO → image not cached on any node (check tote_not_actionable_total)
    YES ↓
  → tote_salvage_successes_total incrementing?
    NO → check salvage failure logs (agent connectivity, rate limits, image size)
    YES → salvage worked, pod should restart
```
