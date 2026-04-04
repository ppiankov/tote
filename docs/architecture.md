# Architecture

## Module layout

```
cmd/tote/main.go                  Cobra CLI: controller + agent subcommands
api/v1alpha1/                     SalvageRecord CRD types (tote.dev/v1alpha1)
config/crd/                       Generated CRD manifests
internal/
  version/version.go              Build-time version via LDFLAGS
  config/config.go                Kill switch, denied namespaces, annotation constants
  detector/detector.go            Extract ImagePullBackOff/ErrImagePull/CreateContainerError
  resolver/resolver.go            Parse image refs, classify digest vs tag-only
  registry/resolve.go             Resolve tag-only images via source registry v2 API (opt-in)
  inventory/inventory.go          Find nodes with a digest via Node.Status.Images
  events/events.go                Emit structured Kubernetes Warning events
  metrics/metrics.go              Prometheus counters + histograms
  controller/controller.go        PodReconciler wiring all packages together
  agent/                          containerd image store + gRPC agent server
  session/session.go              In-memory session store for transfer auth
  transfer/                       Orchestrator + agent endpoint resolver
  registry/                       Backup registry push via go-containerregistry
  tlsutil/                        mTLS credential loading for gRPC
  cleanup/                        SalvageRecord TTL reaper
  notify/                         Webhook notifications (JSON POST)
  webhook/                        Annotation validation webhook (fail-open)
```

## Reconciliation flow

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
      │   ├─ Tag-only → try Node.Status.Images → try agents → try registry v2 → emit NotActionable
      │   └─ Has digest → continue
      │
      ├─ inventory.FindNodes() → which nodes have the digest?
      │   └─ No nodes → skip
      │
      ├─ emit Salvageable event + metric
      │
      └─ Orchestrator configured?
          ├─ SalvageRecord exists for digest? → skip (idempotency)
          ├─ Source == target node? → skip
          ├─ Image too large? → emit failure event, skip
          │
          └─ Salvage:
              ├─ PrepareExport on source agent (verify + get size)
              ├─ ImportFrom on target agent (stream image)
              ├─ Create SalvageRecord CR (persistent history)
              ├─ PushImage to backup registry (optional, non-fatal)
              ├─ Delete pod (owned) for fast recovery
              └─ Pod recreated by owning controller → starts immediately
```

## Node inventory

tote uses two methods to find cached images:

1. **Node.Status.Images** (no agent required): The kubelet reports which images are cached on each node. Limited to 50 images by default (`--node-status-max-images`).

2. **Agent queries** (when deployed): The tote agent DaemonSet queries containerd directly, bypassing the 50-image limit. Also resolves tags to digests as a fallback.

3. **Registry v2 lookup** (opt-in): When both node status and agents fail to resolve a tag-only image, tote queries the source registry's v2 API to resolve the tag to a digest. Requires network access to the registry; skipped when disabled.
