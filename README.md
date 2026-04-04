# tote

[![CI](https://github.com/ppiankov/tote/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/tote/actions/workflows/ci.yml)
[![Release](https://github.com/ppiankov/tote/actions/workflows/release.yml/badge.svg)](https://github.com/ppiankov/tote/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/ppiankov/tote)](https://goreportcard.com/report/github.com/ppiankov/tote)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![ANCC](https://img.shields.io/badge/ANCC-compliant-brightgreen)](docs/SKILL.md)

Emergency Kubernetes operator that detects image pull failures, finds cached copies on other nodes, and salvages images via node-to-node transfer.

**If this tool ever feels comfortable, you've used it wrong.**

## Why this exists

Kubernetes clusters with long-lived workloads lose images. Registries get cleaned. Artifacts get deleted. Harbors go down. Nobody has the Dockerfile anymore. Pods crash-loop with `ImagePullBackOff` while the exact image sits cached on another node, quietly working.

tote detects this situation, finds which nodes still have the image, and transfers it to where it's needed — automatically, if you opt in.

## What tote is

- A Kubernetes operator that watches for `ImagePullBackOff`, `ErrImagePull`, and `CreateContainerError`
- A detector that finds which cluster nodes still have the exact image digest cached
- A salvage engine that transfers images node-to-node via gRPC agents and containerd
- A cleanup tool that removes corrupt image records from containerd
- A backup pipeline that pushes salvaged images to a registry before the last cached copy disappears
- A webhook emitter that notifies external systems on detection and salvage events
- An emergency tool — a fire extinguisher, not plumbing

## What tote is NOT

- **NOT a replacement for proper CI/CD** — fix your build pipeline, tote just buys you time
- **NOT a security tool** — does not validate signatures, provenance, or supply chain integrity
- **NOT invisible** — every detection emits a Warning event that says "This is technical debt"
- **NOT enabled by default** — requires explicit opt-in annotations on both Namespace and Pod

## Philosophy

> Principiis obsta — resist the beginnings.

tote exists to keep production alive long enough for humans to fix what they broke. It presents evidence and lets operators decide. Every event it emits is a reminder that something upstream is broken.

This is duct tape for container images. Use it. Then fix the real problem.

## Quick start

```bash
# Helm install
helm install tote ./charts/tote -n tote --create-namespace

# Opt in a namespace
kubectl annotate namespace my-app tote.dev/allow=true

# Opt in a workload (in pod template)
# annotations:
#   tote.dev/auto-salvage: "true"
```

When an image pull fails, tote emits events like:

```
Warning  ImageSalvageable  Registry pull failed for registry.example.com/my-app@sha256:e3b0c44...;
                           image digest exists on nodes: [node-1, node-3].
                           This is technical debt — rebuild and push the image properly.
```

## Agent integration

Single binary, Helm-based deployment, structured Kubernetes events, Prometheus metrics.

Agents: read [`docs/SKILL.md`](docs/SKILL.md) for commands, Helm values, event parsing patterns, and metrics queries.

Key pattern: `kubectl get events --field-selector reason=ImageSalvageable -o json`

## SpectreHub integration

```sh
spectrehub collect --tool tote
```

## Documentation

| Document | Contents |
|----------|----------|
| [Architecture](docs/architecture.md) | Module layout, reconciliation flow, node inventory |
| [Operations Guide](docs/operations-guide.md) | Installation, configuration, alerts, metrics, logs, troubleshooting |
| [CLI Reference](docs/cli-reference.md) | Controller/agent flags, annotations, events, metrics |
| [Security & Safety](docs/security.md) | Defense in depth, mTLS, RBAC, hardening |
| [Known Limitations](docs/known-limitations.md) | kubelet limits, tag-only images, imagePullPolicy |

## License

[MIT](LICENSE)

---

*tote exists to keep prod alive long enough for humans to fix what they broke.*
