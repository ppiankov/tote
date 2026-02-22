# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.5.0] - 2026-02-22

### Added

- Inherit `tote.dev/auto-salvage` annotation from owner chain: Deployment, StatefulSet, DaemonSet, ReplicaSet, Job (WO-12)
- Webhook notifications via `--webhook-url` and `--webhook-events` (WO-13)
- SalvageRecord TTL cleanup via `--salvagerecord-ttl` (WO-15)
- Health probes: HTTP `/healthz` and `/readyz` on controller, gRPC health service on agent (WO-16)
- PodDisruptionBudget and NetworkPolicy Helm templates (WO-17)
- Salvage and push duration histograms: `tote_salvage_duration_seconds`, `tote_push_duration_seconds` (WO-18)
- JSON logging via `--json-log` flag (WO-19)
- Annotation validation webhook (fail-open, `webhook.enabled: false`) (WO-23)
- E2E test framework with kind (`make e2e`) (WO-24)
- Helm chart lint (`make helm-lint`) with 5 value combinations (WO-25)
- Agent pod-level seccomp profile `RuntimeDefault` (WO-22)
- New packages: `internal/cleanup`, `internal/notify`, `internal/webhook`

### Changed

- Reconcile requeues with 30s backoff on transient salvage failures (WO-20)
- SalvageRecord idempotency uses field index on `spec.digest` for O(1) lookup (WO-21)
- RBAC: added `apps/v1` and `batch/v1` `get` for owner inheritance, `delete` for salvagerecord cleanup

## [0.4.0] - 2026-02-22

### Added

- Push salvaged images to backup registry via `--backup-registry` (WO-5)
- Grafana dashboard with ConfigMap for sidecar auto-discovery (WO-6)
- mTLS for gRPC communication via `--tls-cert`, `--tls-key`, `--tls-ca` (WO-8)
- Leader election for multi-replica controller safety (WO-9)
- Detect and clean `CreateContainerError` from corrupt/incomplete images (WO-11)
- `SalvageRecord` CRD (`tote.dev/v1alpha1`) for persistent salvage tracking (WO-10)
- CRD manifest generation via controller-gen (`make generate`)
- Helm CRD auto-install (`charts/tote/crds/`)
- Prometheus metrics: `tote_corrupt_images_total`, `tote_push_{attempts,successes,failures}_total`
- Kubernetes events: `ImageCorrupt`, `ImagePushed`, `ImagePushFailed`
- CLI flags: `--backup-registry`, `--backup-registry-secret`, `--backup-registry-insecure`, `--tls-cert`, `--tls-key`, `--tls-ca`
- `internal/registry` package for backup registry push via go-containerregistry
- `internal/tlsutil` package for mTLS credential loading
- `PushImage` and `RemoveImage` gRPC RPCs
- Helm values: `tls.enabled`, `tls.secretName`, `dashboard.enabled`, `controller.backupRegistry*`
- RBAC: secrets `get` for registry credentials, leases for leader election, `tote.dev/salvagerecords`

### Changed

- CI split into 4 parallel jobs: test, lint-fast, lint-full, build (WO-7)
- Full linter set (errcheck, staticcheck, unused) runs in CI via `.golangci-full.yml`
- ClusterRole: added `coordination.k8s.io/leases`, `secrets`, and `salvagerecords` permissions
- Idempotency guard switched from pod annotations to SalvageRecord lookup

### Removed

- Pod annotations `tote.dev/salvaged-digest` and `tote.dev/imported-at` (replaced by SalvageRecord CRD)
- Pod `patch` RBAC verb (no longer needed)

## [0.3.0] - 2026-02-20

### Added

- Max image size guard via `--max-image-size` flag, default 2 GiB (WO-3)
- Pod restart after salvage for fast recovery (WO-1)
- Tag resolution via agent gRPC to bypass kubelet 50-image limit

### Changed

- Demoted per-reconcile agent resolution logs to V(1) (WO-2)
- Updated README for v0.2 architecture and salvage flow (WO-4)

### Fixed

- CRI label on imported images; skip same-node salvage
- Content-store API for containerd v1.x compatibility
- Session registration on agent during PrepareExport
- Agent runs as root to access containerd socket
- golangci-lint usable locally with fast config

## [0.2.0] - 2026-02-08

### Added

- Node-local image salvage via gRPC streaming between agents
- Agent DaemonSet (`tote agent`) with containerd integration
- gRPC service: `PrepareExport`, `ExportImage`, `ImportFrom`, `ListImages`
- Session-based authorization for inter-agent image transfers
- Salvage orchestration with configurable concurrency (`--max-concurrent-salvages`)
- Pod annotation patching: `tote.dev/salvaged-digest`, `tote.dev/imported-at`
- Kubernetes events: `ImageSalvaged`, `ImageSalvageFailed`
- Prometheus metrics: `tote_salvage_attempts_total`, `tote_salvage_successes_total`, `tote_salvage_failures_total`
- CLI subcommands: `tote controller`, `tote agent`
- Helm chart: agent DaemonSet, agent ServiceAccount, controller salvage flags
- One-shot salvage: skips pods already salvaged for the same digest

### Changed

- Controller now orchestrates salvage after detection when `--agent-namespace` is set
- ClusterRole: added pod `patch` verb for annotation updates
- Bare `tote` command runs controller for backward compatibility

## [0.1.0] - 2026-02-07

### Added

- Pod watcher for `ImagePullBackOff` and `ErrImagePull` container states
- Double opt-in via `tote.dev/allow` (Namespace) and `tote.dev/auto-salvage` (Pod) annotations
- Digest-only enforcement — tag-only image references marked as "not actionable"
- Node image inventory via `Node.Status.Images` (no DaemonSet required)
- Kubernetes Warning events: `ImageSalvageable` and `ImageNotActionable`
- Prometheus metrics: `tote_detected_failures_total`, `tote_salvageable_images_total`, `tote_not_actionable_total`
- Global kill switch (`--enabled=false`)
- Default-deny for critical namespaces: `kube-system`, `kube-public`, `kube-node-lease`
- CLI flags: `--enabled`, `--metrics-addr`, `--version`

[Unreleased]: https://github.com/ppiankov/tote/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/ppiankov/tote/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/ppiankov/tote/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/ppiankov/tote/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/ppiankov/tote/releases/tag/v0.2.0
[0.1.0]: https://github.com/ppiankov/tote/releases/tag/v0.1.0
