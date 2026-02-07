# tote

[![CI](https://github.com/ppiankov/tote/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/tote/actions/workflows/ci.yml)
[![Release](https://github.com/ppiankov/tote/actions/workflows/release.yml/badge.svg)](https://github.com/ppiankov/tote/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/ppiankov/tote)](https://goreportcard.com/report/github.com/ppiankov/tote)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Emergency Kubernetes operator that detects image pull failures and finds which nodes still have the image cached.

---

## Why this exists

Kubernetes clusters with long-lived workloads lose images. Registries get cleaned. Artifacts get deleted. Harbors go down. Nobody has the Dockerfile anymore. Pods crash-loop with `ImagePullBackOff` while the exact image sits cached on another node, quietly working.

tote detects this situation and tells you about it — loudly, with shame, and with evidence.

**If this tool ever feels comfortable, you've used it wrong.**

## What tote is

- A Kubernetes operator that watches for `ImagePullBackOff` and `ErrImagePull`
- A detector that finds which cluster nodes still have the exact image digest cached
- A loud alarm that emits Kubernetes Warning events with node names and remediation hints
- An emergency tool — a fire extinguisher, not plumbing

## What tote is NOT

- **NOT a remediation tool** — v0.1 detects and reports, it does not move images or modify workloads
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

### Install

Download the binary from [Releases](https://github.com/ppiankov/tote/releases):

```bash
# Linux amd64
curl -Lo tote https://github.com/ppiankov/tote/releases/latest/download/tote-linux-amd64
chmod +x tote

# macOS arm64
curl -Lo tote https://github.com/ppiankov/tote/releases/latest/download/tote-darwin-arm64
chmod +x tote
```

### Run

```bash
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

| Flag | Default | Description |
|------|---------|-------------|
| `--enabled` | `true` | Global kill switch. Set to `false` to disable all detection. |
| `--metrics-addr` | `:8080` | Bind address for the Prometheus metrics endpoint. |
| `--version` | | Print version and exit. |

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
    resources: [pods, namespaces, nodes]
    verbs: [get, list, watch]
  - apiGroups: [""]
    resources: [events]
    verbs: [create, patch]
  - apiGroups: [events.k8s.io]
    resources: [events]
    verbs: [create, patch]
```

## Architecture

```
cmd/tote/main.go                  Cobra CLI → controller-runtime manager
internal/
  version/version.go              Build-time version via LDFLAGS
  config/config.go                Kill switch, denied namespaces, annotation constants
  detector/detector.go            Extract ImagePullBackOff/ErrImagePull from Pod status
  resolver/resolver.go            Parse image refs, classify digest vs tag-only
  inventory/inventory.go          Find nodes with a digest via Node.Status.Images
  events/events.go                Emit structured Kubernetes Warning events
  metrics/metrics.go              Prometheus counters
  controller/controller.go        PodReconciler wiring all packages together
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
  ├─ detector.Detect() → any ImagePullBackOff/ErrImagePull?
  │   └─ No failures → skip
  │
  └─ For each failing container:
      ├─ resolver.Resolve() → has digest?
      │   └─ Tag-only → emit NotActionable event + metric
      │
      └─ inventory.FindNodes() → which nodes have the digest?
          └─ Nodes found → emit Salvageable event + metric
```

### How node inventory works

tote reads `Node.Status.Images` from the Kubernetes API — the kubelet already reports which container images are cached on each node. No DaemonSet required, no node-level agent, no containerd access.

This means tote needs only **read access** to work. It never touches your nodes, your containers, or your workloads.

## Known limitations

1. **kubelet image limit**: The kubelet reports at most 50 images per node by default (`--node-status-max-images=50`). Nodes with many images may not report all cached images. tote cannot find images beyond this limit.

2. **Tag-only images are not actionable**: If your image reference uses a tag (`:latest`, `:v1.2.3`) instead of a digest (`@sha256:...`), tote cannot determine whether two nodes have the same image content. These are reported as "not actionable."

3. **No owner workload inheritance**: The `tote.dev/auto-salvage` annotation must be set directly on the Pod (or pod template). tote does not check the owning Deployment or StatefulSet annotations.

4. **No leader election**: Running multiple replicas will emit duplicate events. Safe but redundant. Leader election will be added when remediation actions are introduced.

5. **Detection only**: v0.1 detects and reports. It does not export images, push to registries, or modify workloads. That is the v0.2 roadmap.

## Roadmap

### v0.1.0 (current) — Detection

- [x] Watch pods for `ImagePullBackOff` / `ErrImagePull`
- [x] Double opt-in via namespace + pod annotations
- [x] Digest-only enforcement (tag references marked not actionable)
- [x] Node inventory via `Node.Status.Images`
- [x] Kubernetes Warning events with node names
- [x] Prometheus metrics
- [x] Global kill switch
- [x] Default-deny for critical namespaces

### v0.2.0 — Salvage

- [ ] DaemonSet node agent for image export via containerd/CRI
- [ ] Admin-configured destination registry
- [ ] Image export → push → workload rewrite flow
- [ ] One-shot per digest (no infinite loops)
- [ ] TTL on salvaged images
- [ ] Audit receipts (before/after/evidence bundle)
- [ ] Leader election
- [ ] Rate limiting (max concurrent salvages, per-namespace caps)
- [ ] Max image size guard

### Future

- [ ] Owner workload annotation inheritance
- [ ] Namespace-level rate limiting
- [ ] Webhook/Slack notifications
- [ ] Helm chart
- [ ] Predicate filtering (only enqueue pods with waiting containers)

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
