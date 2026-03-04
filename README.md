# tote

[![CI](https://github.com/ppiankov/tote/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/tote/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ppiankov/tote)](https://goreportcard.com/report/github.com/ppiankov/tote)

**tote** — Emergency Kubernetes operator for image pull failure recovery. Part of [SpectreHub](https://github.com/ppiankov/spectrehub).

## What it is

- Detects ImagePullBackOff and ErrImagePull failures in real time
- Finds cached copies of failed images on other cluster nodes
- Salvages images via node-to-node transfer using gRPC agents and containerd
- Supports backup registry push, mTLS, leader election, and CRD-based records
- Provides webhook notifications, Prometheus metrics, and structured JSON logging

## What it is NOT

- Not a container registry — salvages from node cache, not a storage layer
- Not a CI/CD tool — operates at runtime, not build time
- Not a monitoring dashboard — emits events and metrics for external systems
- Not a replacement for registry HA — emergency recovery, not primary distribution

## Quick start

### Helm

```sh
helm repo add ppiankov https://ppiankov.github.io/tote
helm install tote ppiankov/tote
```

### From source

```sh
git clone https://github.com/ppiankov/tote.git
cd tote
make build
```

### Usage

```sh
helm install tote ppiankov/tote --set agent.enabled=true
```

## Components

| Component | Description |
|-----------|-------------|
| Controller | Watches for ImagePullBackOff, coordinates salvage |
| Agent (DaemonSet) | Runs on each node, handles containerd image operations |
| CRD (SalvageRecord) | Tracks salvage history and outcomes |
| Webhooks | Notifies external systems on salvage events |

## SpectreHub integration

tote feeds image pull failure and salvage events into [SpectreHub](https://github.com/ppiankov/spectrehub) for unified visibility across your infrastructure.

```sh
spectrehub collect --tool tote
```

## Troubleshooting

See [docs/troubleshooting.md](docs/troubleshooting.md) for a step-by-step debugging guide.

## Safety

tote operates with **minimal cluster permissions**. It reads pod status and transfers cached images between nodes — never deletes images, pods, or other resources.

## License

MIT — see [LICENSE](LICENSE).

---

Built by [Obsta Labs](https://github.com/ppiankov)
