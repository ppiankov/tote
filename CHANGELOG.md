# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.2.0]: https://github.com/ppiankov/tote/releases/tag/v0.2.0
[0.1.0]: https://github.com/ppiankov/tote/releases/tag/v0.1.0
