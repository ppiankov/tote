# Roadmap

## Completed

### v0.1.0 — Detection

- [x] Watch pods for `ImagePullBackOff` / `ErrImagePull`
- [x] Double opt-in via namespace + pod annotations
- [x] Digest-only enforcement (tag references marked not actionable)
- [x] Node inventory via `Node.Status.Images`
- [x] Kubernetes Warning events with node names
- [x] Prometheus metrics (detected, salvageable, not_actionable)
- [x] Global kill switch
- [x] Default-deny for critical namespaces

### v0.2.0 — Node-local salvage

- [x] DaemonSet node agent for image export/import via containerd
- [x] Node-to-node image transfer via gRPC streaming
- [x] Tag resolution via agents (bypasses kubelet 50-image limit)
- [x] One-shot per digest (SalvageRecord guard)
- [x] Rate limiting (max concurrent salvages)
- [x] Helm chart

### v0.3.0 — Quick wins

- [x] Max image size guard (`--max-image-size`, default 2 GiB)
- [x] Pod restart after salvage (fast recovery)
- [x] Demote per-reconcile agent logs to V(1)

### v0.4.0 — Registry push and observability

- [x] Push salvaged images to backup registry (`--backup-registry`)
- [x] Grafana dashboard with sidecar auto-discovery
- [x] Leader election for multi-replica safety
- [x] mTLS for gRPC communication (TLS 1.3, mutual cert verification)
- [x] Detect `CreateContainerError` (corrupt/incomplete images)
- [x] `SalvageRecord` CRD for persistent salvage tracking

### v0.5.0 — Usability and hardening

- [x] Owner workload annotation inheritance (Deployment, StatefulSet, DaemonSet, Job)
- [x] Webhook notifications (`--webhook-url`, `--webhook-events`)
- [x] SalvageRecord TTL cleanup (`--salvagerecord-ttl`)
- [x] Health and readiness probes (HTTP + gRPC)
- [x] PodDisruptionBudget and NetworkPolicy templates
- [x] Salvage and push duration histograms
- [x] JSON logging (`--json-log`)
- [x] Annotation validation webhook (fail-open)
- [x] Reconcile requeue on transient salvage failures
- [x] SalvageRecord field index for O(1) idempotency lookup
- [x] Agent seccomp profile (RuntimeDefault)
- [x] E2E tests with kind
- [x] Helm chart lint validation

### v0.5.1 — Fixes and CI hardening

- [x] Fix CRD generation (`+groupName=tote.dev` marker)
- [x] Fix RBAC: add `list`/`watch` for apps, batch, SalvageRecords
- [x] ServiceMonitor aligned with production Prometheus Operator patterns
- [x] Safety and Security section in README
- [x] CI: generated code check (controller-gen + git diff)
- [x] CI: Helm lint job
- [x] `make proto` Makefile target

### v0.6.0 — Observability and memory

- [x] PrometheusRule alerts in Helm chart
- [x] Troubleshooting guide
- [x] Cache transform to reduce controller memory ~10x (strip unused pod fields)
- [x] Increase default controller memory to 512Mi

### v0.7.0 — Diagnostics and compliance

- [x] `tote doctor` command (kubeconfig, CRD, controller, agents, namespaces)
- [x] `docs/SKILL.md` for ANCC compliance
- [x] PrometheusRule alerts for not-actionable image spikes

### v0.8.0 — Registry resolution

- [x] Registry-assisted tag resolution (`--registry-resolve`) — query source registries to resolve tag→digest for tag-only images
- [x] Replace deprecated `NewSimpleClientset` with `NewClientset`

## Current state

**v0.8.0** — tagged, released. CI: 6 parallel jobs, all green, under 2 minutes.

## Known gaps (not blocking)

| Gap | Severity | Notes |
|-----|----------|-------|
| Agent test coverage ~30% | Low | Tests require containerd socket; `FakeImageStore` covers cross-package use |
| Transfer test coverage ~40% | Low | Same containerd dependency; orchestrator logic tested via controller_test.go |
| E2E not in CI | Low | Requires Kind cluster; verified manually on real cluster |
| Local golangci-lint v1 vs v2 | Low | CI works; local `make lint` uses fast config with v1 |

## Future

These are genuinely future features — not planned for any specific release.

### Multi-cluster salvage

Extend the controller to discover and transfer images across cluster boundaries. Requires federation mechanism (e.g., multi-cluster services, submariner, or custom gRPC federation). Significant scope increase — only justified for organizations with many ephemeral clusters.

### Priority-based salvage queue

Currently all salvage requests are equal. A priority queue would let operators mark certain workloads as high-priority (e.g., `tote.dev/priority: critical`) so their salvage operations preempt others when `--max-concurrent-salvages` is the bottleneck.

### Image pre-warming

Track which images are deployed where and proactively distribute them to nodes before they're needed. This shifts tote from reactive (salvage after failure) to proactive (prevent failure). Significantly changes the project's scope and philosophy — would need careful consideration of whether this still fits "emergency tool, not plumbing."

## Out of scope

These are explicitly not planned:

- ML-based prediction of pull failures
- Registry mirroring or caching proxy
- Automatic image rebuilds
- Image signature verification or supply chain validation
