---
name: tote
description: Kubernetes operator that detects image pull failures and salvages cached images from other nodes
user-invocable: false
metadata: {"requires":{"bins":["kubectl","helm"]}}
---

# tote — Emergency Image Salvage for Kubernetes

You have access to `tote`, a Kubernetes operator that detects `ImagePullBackOff` failures and salvages cached images from other cluster nodes via gRPC agents. Two components: a controller (Deployment) watches for failures, agents (DaemonSet) serve images from local containerd.

## Install

### Helm (recommended)

```bash
helm install tote oci://ghcr.io/ppiankov/tote/charts/tote \
  --namespace tote-system --create-namespace
```

Or from local chart:

```bash
git clone https://github.com/ppiankov/tote.git
helm install tote charts/tote -n tote-system --create-namespace
```

### Private registry (air-gapped clusters)

```bash
helm install tote charts/tote -n tote-system --create-namespace \
  --set image.repository=<your-registry>/tote \
  --set image.tag=0.2.1 \
  --set imagePullSecrets[0].name=<your-pull-secret>
```

### From source

```bash
go install github.com/ppiankov/tote/cmd/tote@latest
```

## Setup

tote uses a double opt-in model. Both the namespace and each pod must be annotated:

```bash
# 1. Enable namespace
kubectl annotate namespace <ns> tote.dev/allow=true

# 2. Enable pod (via deployment)
kubectl patch deployment <name> -n <ns> -p \
  '{"spec":{"template":{"metadata":{"annotations":{"tote.dev/auto-salvage":"true"}}}}}'
```

### Hardcoded denials

`kube-system`, `kube-public`, `kube-node-lease` are always excluded.

## Components

| Component | Kind | What it does |
|-----------|------|-------------|
| Controller | Deployment | Watches pods, detects failures, resolves images, orchestrates salvage |
| Agent | DaemonSet | Serves images from local containerd via gRPC (port 9090) |

## Helm Values

| Value | Default | Description |
|-------|---------|-------------|
| `image.repository` | `ghcr.io/ppiankov/tote` | Container image |
| `image.tag` | `0.2.1` | Image tag |
| `imagePullSecrets` | `[]` | Pull secrets for private registries |
| `config.enabled` | `true` | Global kill switch |
| `config.metricsAddr` | `:8080` | Controller metrics bind address |
| `controller.maxConcurrentSalvages` | `2` | Max parallel salvage operations |
| `controller.sessionTTL` | `5m0s` | Salvage session lifetime |
| `agent.enabled` | `true` | Deploy agent DaemonSet |
| `agent.containerdSocket` | `/run/containerd/containerd.sock` | Containerd socket path |

## Annotations

| Annotation | Level | Value | Purpose |
|-----------|-------|-------|---------|
| `tote.dev/allow` | Namespace | `"true"` | Opt namespace into monitoring |
| `tote.dev/auto-salvage` | Pod | `"true"` | Opt pod into salvage |
| `tote.dev/salvaged-digest` | Pod | `sha256:...` | Set by controller after successful salvage |
| `tote.dev/imported-at` | Pod | RFC3339 | Timestamp of completed salvage |

## Agent Usage Pattern

For agents managing Kubernetes clusters, the typical workflow is:

```bash
# 1. Install tote
helm install tote charts/tote -n tote-system --create-namespace

# 2. Opt in a namespace
kubectl annotate namespace myapp tote.dev/allow=true

# 3. Opt in deployments
kubectl patch deployment myapp -n myapp -p \
  '{"spec":{"template":{"metadata":{"annotations":{"tote.dev/auto-salvage":"true"}}}}}'

# 4. Check for detections
kubectl get events -n myapp --field-selector reason=ImageSalvageable
kubectl get events -n myapp --field-selector reason=ImageNotActionable

# 5. Check controller logs
kubectl logs -n tote-system deploy/tote --tail=50

# 6. Check metrics
kubectl port-forward -n tote-system deploy/tote 8080:8080
curl -s localhost:8080/metrics | grep tote_
```

### Key metrics

| Metric | Type | Meaning |
|--------|------|---------|
| `tote_detected_total` | counter | Pods with image pull failures |
| `tote_salvageable_total` | counter | Images found cached on other nodes |
| `tote_not_actionable_total` | counter | Tag-only images with no cached digest |
| `tote_salvage_success_total` | counter | Successful image transfers |
| `tote_salvage_failure_total` | counter | Failed transfers |

### Kubernetes events

| Reason | Type | Meaning |
|--------|------|---------|
| `ImageSalvageable` | Warning | Image found on other nodes, salvage possible |
| `ImageNotActionable` | Warning | Tag-only reference, no cached digest available |
| `ImageSalvaged` | Normal | Image transferred successfully |

## Troubleshooting

```bash
# Controller not detecting pods?
# Check: namespace annotated?
kubectl get ns <ns> -o jsonpath='{.metadata.annotations.tote\.dev/allow}'

# Check: pod annotated?
kubectl get pod <pod> -n <ns> -o jsonpath='{.metadata.annotations.tote\.dev/auto-salvage}'

# Check: controller logs
kubectl logs deploy/tote -n tote-system | tail -20

# Agent not serving images?
kubectl logs daemonset/tote-agent -n tote-system | tail -20
```

## What tote Does NOT Do

- Does not replace CI/CD or proper image management
- Does not push images to registries (planned v0.3)
- Does not use ML — deterministic detection via pod status
- Does not modify workloads — only transfers cached images between nodes
- Does not operate on kube-system or other control plane namespaces

## Processing Flow

```
Pod event → kill switch? → denied namespace? → namespace annotation?
  → pod annotation? → detect ImagePullBackOff → resolve image reference
  → digest available? (yes: match by digest / no: search node cache by tag)
  → find source nodes → emit event → orchestrate transfer
```

## Exit Codes

- `0` — clean shutdown
- `1` — startup failure (bad flags, no kubeconfig, containerd unreachable)
