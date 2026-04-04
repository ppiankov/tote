# tote — Emergency Image Salvage for Kubernetes

Kubernetes operator that detects ImagePullBackOff failures and salvages cached images from other cluster nodes via gRPC agents. Optionally pushes salvaged images to a backup registry and sends webhook notifications.

## Install

```bash
helm install tote ppiankov/tote -n tote-system --create-namespace
```

## Commands

### tote controller

Runs the controller (default when no subcommand given). Watches pods, detects failures, orchestrates salvage.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--enabled` | `true` | Global kill switch |
| `--metrics-addr` | `:8080` | Prometheus metrics endpoint |
| `--agent-namespace` | | Namespace where agents run (required for salvage) |
| `--agent-grpc-port` | `9090` | gRPC port for agent communication |
| `--max-concurrent-salvages` | `2` | Max parallel salvage operations |
| `--max-image-size` | `2147483648` | Max image size in bytes (0 = no limit) |
| `--session-ttl` | `5m` | Salvage session lifetime |
| `--backup-registry` | | Registry to push salvaged images (empty = disabled) |
| `--backup-registry-secret` | | dockerconfigjson Secret name for registry credentials |
| `--backup-registry-insecure` | `false` | Allow HTTP to backup registry |
| `--tls-cert` | | TLS certificate for mTLS |
| `--tls-key` | | TLS private key for mTLS |
| `--tls-ca` | | CA certificate for mTLS |
| `--json-log` | `false` | JSON log format |
| `--salvagerecord-ttl` | `168h` | TTL for completed SalvageRecords |
| `--webhook-url` | | URL for event notifications (empty = disabled) |
| `--webhook-events` | | Event types: detected, salvaged, salvage_failed, pushed, push_failed |

### tote agent

Runs the node agent DaemonSet. Serves images from local containerd via gRPC.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--containerd-socket` | `/run/containerd/containerd.sock` | Containerd socket path |
| `--grpc-port` | `9090` | gRPC listen port |
| `--metrics-addr` | `:8081` | Prometheus metrics endpoint |
| `--tls-cert` | | TLS certificate for mTLS |
| `--tls-key` | | TLS private key for mTLS |
| `--tls-ca` | | CA certificate for mTLS |
| `--json-log` | `false` | JSON log format |

### tote doctor

Checks runtime prerequisites and reports health status as structured JSON.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--namespace` | `tote-system` | Namespace where tote is deployed |

**JSON output:**

```json
{
  "checks": [
    {"name": "kubeconfig", "status": "ok", "message": "cluster reachable"},
    {"name": "crd", "status": "ok", "message": "salvagerecords.tote.dev installed"},
    {"name": "controller", "status": "ok", "message": "1/1 replicas ready"},
    {"name": "agents", "status": "ok", "message": "3/3 agents ready"},
    {"name": "namespaces", "status": "ok", "message": "2 namespaces opted in: default, myapp"}
  ],
  "ok": true
}
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| `0` | All checks passed |
| `1` | One or more checks failed |

### tote version

Print version information.

**JSON output (`--json`):**

```json
{
  "version": "0.8.0"
}
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| `0` | Clean shutdown |
| `1` | Startup failure (bad flags, no kubeconfig, containerd unreachable) |

## Components

| Component | Kind | What it does |
|-----------|------|-------------|
| Controller | Deployment | Watches pods, detects failures, orchestrates salvage, pushes to backup registry |
| Agent | DaemonSet | Serves images from local containerd via gRPC |

**Note:** The Helm deployment is named `tote` (not `tote-controller`). Use `kubectl logs deploy/tote` for controller logs.

## CRDs

### SalvageRecord (tote.dev/v1alpha1)

Tracks salvage operations. Created after successful image transfer.

**JSON schema:**

```json
{
  "apiVersion": "tote.dev/v1alpha1",
  "kind": "SalvageRecord",
  "metadata": {"name": "...", "namespace": "..."},
  "spec": {
    "podName": "web-abc123",
    "digest": "sha256:abc123...",
    "imageRef": "nginx:1.25@sha256:abc123...",
    "sourceNode": "node-1",
    "targetNode": "node-2"
  },
  "status": {
    "phase": "Completed",
    "completedAt": "2026-01-15T10:30:00Z",
    "error": ""
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `spec.podName` | string | Pod that triggered the salvage |
| `spec.digest` | string | Image content digest (sha256:...) |
| `spec.imageRef` | string | Original image reference from pod spec |
| `spec.sourceNode` | string | Node the image was exported from |
| `spec.targetNode` | string | Node the image was imported to |
| `status.phase` | string | `Completed` or `Failed` |
| `status.completedAt` | string | RFC3339 timestamp |
| `status.error` | string | Failure reason (empty on success) |

```bash
kubectl get salvagerecords -A -o json | jq '.items[] | {digest: .spec.digest, source: .spec.sourceNode, target: .spec.targetNode, phase: .status.phase}'
```

## Kubernetes events

| Reason | Type | Action | When |
|--------|------|--------|------|
| `ImageSalvageable` | Warning | Detected | Image digest found cached on other nodes |
| `ImageNotActionable` | Warning | Detected | Image uses tag instead of digest |
| `ImageSalvaged` | Warning | Salvaged | Image successfully transferred to target node |
| `ImageSalvageFailed` | Warning | Salvaging | Salvage transfer failed |
| `ImageCorrupt` | Warning | Cleaning | Corrupt image record detected in containerd |
| `ImagePushed` | Normal | Pushing | Image pushed to backup registry |
| `ImagePushFailed` | Warning | Pushing | Backup registry push failed |

**Event JSON schema:**

```json
{
  "reason": "ImageSalvageable",
  "type": "Warning",
  "action": "Detected",
  "regarding": {"kind": "Pod", "name": "web-abc123", "namespace": "myapp"},
  "note": "Registry pull failed for nginx:1.25@sha256:abc...; image digest exists on nodes: [node-1, node-2]."
}
```

```bash
kubectl get events -n myapp --field-selector reason=ImageSalvageable -o json | jq '.items[] | {reason, note: .note, pod: .regarding.name}'
```

## Prometheus metrics

All metrics are exposed on the controller's `--metrics-addr` (default `:8080`).

| Metric | Type | Description |
|--------|------|-------------|
| `tote_detected_failures_total` | counter | Image pull failures detected |
| `tote_salvageable_images_total` | counter | Failures where image digest found on cluster nodes |
| `tote_not_actionable_total` | counter | Failures where image uses tag instead of digest |
| `tote_corrupt_images_total` | counter | Corrupt image records detected and cleaned |
| `tote_salvage_attempts_total` | counter | Salvage attempts |
| `tote_salvage_successes_total` | counter | Successful salvages |
| `tote_salvage_failures_total` | counter | Failed salvage attempts |
| `tote_push_attempts_total` | counter | Backup registry push attempts |
| `tote_push_successes_total` | counter | Successful pushes |
| `tote_push_failures_total` | counter | Failed push attempts |
| `tote_salvage_duration_seconds` | histogram | Salvage operation duration (buckets: 0.5, 1, 2, 5, 10, 30, 60, 120, 300) |
| `tote_push_duration_seconds` | histogram | Push operation duration (buckets: 0.5, 1, 2, 5, 10, 30, 60, 120, 300) |

**Prometheus exposition format:**

```
# HELP tote_detected_failures_total Total number of image pull failures detected.
# TYPE tote_detected_failures_total counter
tote_detected_failures_total 42
```

```bash
kubectl port-forward -n tote-system deploy/tote 8080:8080
curl -s localhost:8080/metrics | grep tote_
```

## JSON log format

When `--json-log=true`, logs are structured JSON (one object per line):

```json
{"level": "info", "ts": "2026-01-15T10:30:00Z", "logger": "controller.pod", "msg": "image salvageable", "container": "web", "digest": "sha256:abc...", "nodes": ["node-1"]}
```

| Field | Type | Description |
|-------|------|-------------|
| `level` | string | `info`, `error`, or `debug` |
| `ts` | string | RFC3339 timestamp |
| `logger` | string | Logger name (e.g. `controller.pod`, `agent`) |
| `msg` | string | Log message |
| `controller` | string | Controller name (present in reconciler logs) |
| `reconcileID` | string | Unique ID per reconciliation (present in reconciler logs) |

## Helm values

| Value | Default | Description |
|-------|---------|-------------|
| `image.repository` | `ghcr.io/ppiankov/tote` | Container image |
| `image.tag` | `0.7.0` | Image tag |
| `image.pullPolicy` | `IfNotPresent` | Pull policy |
| `resources.requests.memory` | `64Mi` | Controller memory request |
| `resources.limits.memory` | `256Mi` | Controller memory limit |
| `config.enabled` | `true` | Global kill switch |
| `config.metricsAddr` | `:8080` | Controller metrics bind address |
| `config.jsonLog` | `false` | JSON log format |
| `controller.maxConcurrentSalvages` | `2` | Max parallel salvage operations |
| `controller.sessionTTL` | `5m0s` | Salvage session lifetime |
| `controller.agentGRPCPort` | `9090` | Agent gRPC port |
| `controller.backupRegistry` | `""` | Registry for salvaged images (empty = disabled) |
| `controller.backupRegistrySecret` | `""` | dockerconfigjson Secret name |
| `controller.backupRegistryInsecure` | `false` | Allow HTTP to backup registry |
| `controller.salvageRecordTTL` | `168h` | TTL for completed SalvageRecords |
| `notifications.webhookUrl` | `""` | Webhook URL (empty = disabled) |
| `notifications.events` | `""` | Event types to notify |
| `tls.enabled` | `false` | Enable mTLS for gRPC |
| `tls.secretName` | `""` | TLS Secret name |
| `serviceMonitor.enabled` | `false` | Prometheus Operator ServiceMonitor |
| `serviceMonitor.labels` | `{}` | Additional ServiceMonitor labels |
| `prometheusRule.enabled` | `false` | PrometheusRule alerts |
| `prometheusRule.labels` | `{}` | Additional PrometheusRule labels |
| `dashboard.enabled` | `true` | Grafana dashboard ConfigMap |
| `pdb.enabled` | `false` | PodDisruptionBudget |
| `networkPolicy.enabled` | `false` | NetworkPolicy for controller and agent |
| `webhook.enabled` | `false` | Annotation validation webhook |
| `agent.enabled` | `true` | Deploy agent DaemonSet |
| `agent.containerdSocket` | `/run/containerd/containerd.sock` | Containerd socket path |
| `agent.grpcPort` | `9090` | Agent gRPC port |
| `agent.metricsAddr` | `:8081` | Agent metrics bind address |

## What this does NOT do

- Does not replace CI/CD or proper image management
- Does not use ML — deterministic detection via pod status
- Does not modify workloads — only transfers cached images between nodes
- Does not operate on kube-system or control plane namespaces
- Does not validate image signatures or provenance
- Does not pull images from registries — only moves images already cached in cluster

## Parsing examples

```bash
# Check for salvage events
kubectl get events -n myapp --field-selector reason=ImageSalvageable -o json | jq '.items[].note'

# Check for successful salvages
kubectl get events -n myapp --field-selector reason=ImageSalvaged -o json | jq '.items[].note'

# List salvage records
kubectl get salvagerecords -A -o json | jq '.items[] | {digest: .spec.digest, phase: .status.phase}'

# Check controller health
kubectl logs -n tote-system deploy/tote --tail=50

# Check metrics
kubectl port-forward -n tote-system deploy/tote 8080:8080
curl -s localhost:8080/metrics | grep tote_

# Run health check
tote doctor --namespace tote-system
```
