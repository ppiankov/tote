# tote

[![CI](https://github.com/ppiankov/tote/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/tote/actions/workflows/ci.yml)
[![Release](https://github.com/ppiankov/tote/actions/workflows/release.yml/badge.svg)](https://github.com/ppiankov/tote/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/ppiankov/tote)](https://goreportcard.com/report/github.com/ppiankov/tote)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Emergency Kubernetes operator that detects image pull failures, finds cached copies on other nodes, and salvages images via node-to-node transfer.

---

## Why this exists

Kubernetes clusters with long-lived workloads lose images. Registries get cleaned. Artifacts get deleted. Harbors go down. Nobody has the Dockerfile anymore. Pods crash-loop with `ImagePullBackOff` while the exact image sits cached on another node, quietly working.

tote detects this situation, finds which nodes still have the image, and transfers it to where it's needed — automatically, if you opt in.

**If this tool ever feels comfortable, you've used it wrong.**

## What tote is

- A Kubernetes operator that watches for `ImagePullBackOff`, `ErrImagePull`, and `CreateContainerError`
- A detector that finds which cluster nodes still have the exact image digest cached
- A salvage engine that transfers images node-to-node via gRPC agents and containerd
- A cleanup tool that removes corrupt image records (content blobs missing) from containerd
- A loud alarm that emits Kubernetes Warning events with node names
- An emergency tool — a fire extinguisher, not plumbing

## What tote is NOT

- **NOT a replacement for proper CI/CD** — fix your build pipeline, tote just buys you time
- **NOT a security tool** — it does not validate signatures, provenance, or supply chain integrity
- **NOT invisible** — every detection emits a Warning event that says "This is technical debt"
- **NOT enabled by default** — requires explicit opt-in annotations on both Namespace and Pod

## Philosophy

> Principiis obsta — resist the beginnings.

tote exists to keep production alive long enough for humans to fix what they broke. It presents evidence and lets operators decide. It does not hide the problem. Every event it emits is a reminder that something upstream is broken.

This is duct tape for container images. Use it. Then fix the real problem.

## Quick start

### Prerequisites

- Kubernetes cluster (1.28+)
- `kubectl` configured with cluster access
- RBAC permissions: `get`/`list`/`watch` on Pods, Nodes, Namespaces; `create` on Events

### Install with Helm

```bash
helm install tote ./charts/tote -n tote --create-namespace
```

Or with custom values:

```bash
helm install tote ./charts/tote -n tote --create-namespace \
  --set config.metricsAddr=:9090 \
  --set resources.limits.memory=256Mi
```

This deploys tote as a Deployment with a ServiceAccount, ClusterRole, and ClusterRoleBinding. RBAC is created automatically.

### Install from binary

Download from [Releases](https://github.com/ppiankov/tote/releases):

```bash
# Linux amd64
curl -Lo tote https://github.com/ppiankov/tote/releases/latest/download/tote-linux-amd64
chmod +x tote

# macOS arm64
curl -Lo tote https://github.com/ppiankov/tote/releases/latest/download/tote-darwin-arm64
chmod +x tote

# Run with in-cluster config (inside a Pod)
./tote

# Run with local kubeconfig (for development)
./tote --metrics-addr=:9090
```

### Opt in a namespace and workload

tote does nothing unless you explicitly ask it to.

**Step 1: Annotate the namespace**

```bash
kubectl annotate namespace my-app tote.dev/allow=true
```

**Step 2: Annotate the pod (or pod template in your Deployment)**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    metadata:
      annotations:
        tote.dev/auto-salvage: "true"
    spec:
      containers:
        - name: app
          image: registry.example.com/my-app@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

**Step 3: When the registry is gone**

If the image pull fails, tote will emit events like:

```
Warning  ImageSalvageable  Registry pull failed for registry.example.com/my-app@sha256:e3b0c44...; image digest exists on nodes: [node-1, node-3]. This is technical debt — rebuild and push the image properly.
```

Or if the image uses a tag instead of a digest:

```
Warning  ImageNotActionable  Not actionable: image my-app:latest uses tag, not digest. Pin images by digest for tote to help.
```

## Usage

### CLI flags

**Controller** (`tote` or `tote controller`):

| Flag | Default | Description |
|------|---------|-------------|
| `--enabled` | `true` | Global kill switch. Set to `false` to disable all detection. |
| `--metrics-addr` | `:8080` | Bind address for the Prometheus metrics endpoint. |
| `--agent-namespace` | | Namespace where tote agents run (required for salvage). |
| `--agent-grpc-port` | `9090` | gRPC port for agent communication. |
| `--max-concurrent-salvages` | `2` | Max parallel salvage operations. |
| `--max-image-size` | `2147483648` | Max image size in bytes for salvage (0 = no limit). |
| `--session-ttl` | `5m` | Session lifetime for salvage operations. |
| `--backup-registry` | | Registry host to push salvaged images (empty = disabled). |
| `--backup-registry-secret` | | Name of dockerconfigjson Secret for backup registry credentials. |
| `--backup-registry-insecure` | `false` | Allow HTTP connections to backup registry. |
| `--tls-cert` | | Path to TLS certificate file (enables mTLS when all three TLS flags are set). |
| `--tls-key` | | Path to TLS private key file. |
| `--tls-ca` | | Path to CA certificate file for verifying peers. |

**Agent** (`tote agent`):

| Flag | Default | Description |
|------|---------|-------------|
| `--containerd-socket` | `/run/containerd/containerd.sock` | Path to containerd socket. |
| `--grpc-port` | `9090` | gRPC listen port. |
| `--metrics-addr` | `:8081` | Bind address for the Prometheus metrics endpoint. |
| `--tls-cert` | | Path to TLS certificate file (enables mTLS when all three TLS flags are set). |
| `--tls-key` | | Path to TLS private key file. |
| `--tls-ca` | | Path to CA certificate file for verifying peers. |

### Annotations

| Annotation | Target | Required | Description |
|------------|--------|----------|-------------|
| `tote.dev/allow` | Namespace | Yes | Enables tote detection for all opted-in pods in this namespace. |
| `tote.dev/auto-salvage` | Pod | Yes | Marks this pod for tote detection. Must be set directly on the pod (or pod template). |

Both annotations must be set to `"true"` for tote to act. If either is missing, tote silently skips the pod.

### Denied namespaces

The following namespaces are **always excluded**, regardless of annotations:

- `kube-system`
- `kube-public`
- `kube-node-lease`

### Prometheus metrics

| Metric | Type | Description |
|--------|------|-------------|
| `tote_detected_failures_total` | Counter | Total image pull failures detected on opted-in pods. |
| `tote_salvageable_images_total` | Counter | Failures where the image digest was found cached on cluster nodes. |
| `tote_not_actionable_total` | Counter | Failures where the image uses a tag instead of a digest. |
| `tote_corrupt_images_total` | Counter | Corrupt image records detected and cleaned (content blobs missing). |
| `tote_salvage_attempts_total` | Counter | Total salvage transfer attempts. |
| `tote_salvage_successes_total` | Counter | Successful image salvages. |
| `tote_salvage_failures_total` | Counter | Failed salvage attempts. |
| `tote_push_attempts_total` | Counter | Backup registry push attempts. |
| `tote_push_successes_total` | Counter | Successful backup registry pushes. |
| `tote_push_failures_total` | Counter | Failed backup registry push attempts. |

If `tote_salvageable_images_total` is non-zero, you have technical debt. If it stays non-zero, you have a process problem.

### RBAC requirements

tote needs the following cluster-level permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: tote
rules:
  - apiGroups: [""]
    resources: [pods]
    verbs: [get, list, watch, patch, delete]
  - apiGroups: [""]
    resources: [nodes, namespaces]
    verbs: [get, list, watch]
  - apiGroups: [""]
    resources: [events]
    verbs: [create, patch]
  - apiGroups: [events.k8s.io]
    resources: [events]
    verbs: [create, patch]
  - apiGroups: [""]
    resources: [secrets]
    verbs: [get]
```

Pod `patch` is for salvage annotations. Pod `delete` is for fast recovery after salvage (only pods with owner references). Secrets `get` is for reading backup registry credentials.

## Architecture

```
cmd/tote/main.go                  Cobra CLI: controller + agent subcommands
internal/
  version/version.go              Build-time version via LDFLAGS
  config/config.go                Kill switch, denied namespaces, annotation constants
  detector/detector.go            Extract ImagePullBackOff/ErrImagePull from Pod status
  resolver/resolver.go            Parse image refs, classify digest vs tag-only
  inventory/inventory.go          Find nodes with a digest via Node.Status.Images
  events/events.go                Emit structured Kubernetes Warning events
  metrics/metrics.go              Prometheus counters
  controller/controller.go        PodReconciler wiring all packages together
  agent/                          containerd image store + gRPC agent server
  session/session.go              In-memory session store for transfer auth
  transfer/                       Orchestrator + agent endpoint resolver
  registry/                       Backup registry push via go-containerregistry
  tlsutil/                        mTLS credential loading for gRPC
```

### Reconciliation flow

```
Pod event received
  │
  ├─ Kill switch disabled? → skip
  ├─ Denied namespace? → skip
  ├─ Namespace missing tote.dev/allow? → skip
  ├─ Pod missing tote.dev/auto-salvage? → skip
  │
  ├─ detector.Detect() → any ImagePullBackOff/ErrImagePull/CreateContainerError?
  │   └─ No failures → skip
  │
  └─ For each failing container:
      ├─ Corrupt image (CreateContainerError)?
      │   ├─ Agent available → RemoveImage (delete stale record)
      │   ├─ Owned pod → delete for fresh pull
      │   └─ kubelet retries: fresh pull or tote salvages on next cycle
      │
      ├─ resolver.Resolve() → has digest?
      │   ├─ Tag-only → try Node.Status.Images → try agents → emit NotActionable
      │   └─ Has digest → continue
      │
      ├─ inventory.FindNodes() → which nodes have the digest?
      │   └─ No nodes → skip
      │
      ├─ emit Salvageable event + metric
      │
      └─ Orchestrator configured?
          ├─ Already salvaged (annotation)? → skip
          ├─ Source == target node? → skip
          ├─ Image too large? → emit failure event, skip
          │
          └─ Salvage:
              ├─ PrepareExport on source agent (verify + get size)
              ├─ ImportFrom on target agent (stream image)
              ├─ PushImage to backup registry (optional, non-fatal)
              ├─ Delete pod (owned) or annotate (standalone)
              └─ Pod recreated by owning controller → starts immediately
```

### How node inventory works

tote uses two methods to find cached images:

1. **Node.Status.Images** (no agent required): The kubelet reports which images are cached on each node. Limited to 50 images by default (`--node-status-max-images`).

2. **Agent queries** (when deployed): The tote agent DaemonSet queries containerd directly, bypassing the 50-image limit. Also resolves tags to digests as a fallback.

## Known limitations

1. **kubelet image limit**: The kubelet reports at most 50 images per node by default (`--node-status-max-images=50`). Nodes with many images may not report all cached images. tote agents bypass this limit by querying containerd directly.

2. **Tag-only images are not actionable**: If your image reference uses a tag (`:latest`, `:v1.2.3`) instead of a digest (`@sha256:...`), tote cannot determine whether two nodes have the same image content. These are reported as "not actionable." When agents are deployed, tote can resolve tags via containerd as a fallback.

3. **`imagePullPolicy: Always` blocks salvage**: If a pod sets `imagePullPolicy: Always`, kubelet will always contact the registry to verify the image — even if the image exists locally. When the registry is unreachable, the pod will stay in `ImagePullBackOff` regardless of whether tote has salvaged the image to the local node. Salvage only works with `imagePullPolicy: IfNotPresent` (the default for tagged images). Pods using `:latest` (which defaults to `Always`) cannot be salvaged.

4. **Agent requires root access**: The tote agent DaemonSet runs as root (`runAsUser: 0`) to access the containerd socket. This is required for image export/import operations. Environments with strict security policies (e.g., financial institutions) should evaluate this requirement. The controller does not require root.

5. **No owner workload inheritance**: The `tote.dev/auto-salvage` annotation must be set directly on the Pod (or pod template). tote does not check the owning Deployment or StatefulSet annotations.

## Roadmap

### v0.1.0 — Detection

- [x] Watch pods for `ImagePullBackOff` / `ErrImagePull`
- [x] Double opt-in via namespace + pod annotations
- [x] Digest-only enforcement (tag references marked not actionable)
- [x] Node inventory via `Node.Status.Images`
- [x] Kubernetes Warning events with node names
- [x] Prometheus metrics
- [x] Global kill switch
- [x] Default-deny for critical namespaces

### v0.2.0 (current) — Node-local salvage

- [x] DaemonSet node agent for image export/import via containerd
- [x] Node-to-node image transfer via gRPC streaming
- [x] Tag resolution via agents (bypasses kubelet 50-image limit)
- [x] One-shot per digest (annotation guard)
- [x] Rate limiting (max concurrent salvages)
- [x] Max image size guard
- [x] Pod restart after salvage (fast recovery)
- [x] Helm chart

### v0.3 — Registry push and observability

- [x] Push salvaged images to backup registry
- [x] Grafana dashboard
- [x] Leader election
- [x] mTLS between agents
- [x] Detect `CreateContainerError` (corrupt/incomplete images)

### Future

- [ ] CRD for salvage tracking (`SalvageRecord`)
- [ ] Owner workload annotation inheritance
- [ ] Webhook/Slack notifications

## Development

### Build from source

```bash
git clone https://github.com/ppiankov/tote.git
cd tote
make deps
make build        # binary at bin/tote
make test         # tests with -race
make lint         # golangci-lint
make vet          # go vet
```

### Requirements

- Go 1.24+
- golangci-lint (for linting)

## License

[MIT](LICENSE)

---

*tote exists to keep prod alive long enough for humans to fix what they broke.*
