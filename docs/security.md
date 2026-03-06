# Security & Safety

tote operates with cluster-level RBAC, requires explicit double opt-in (namespace + pod annotations), and secures all inter-node communication with mTLS.

## Defense in depth

| Layer | Mechanism | What it prevents |
|-------|-----------|------------------|
| Opt-in | Namespace + Pod annotations both required | Accidental salvage |
| Denied namespaces | `kube-system`, `kube-public`, `kube-node-lease` hardcoded | Control plane interference |
| RBAC | Least-privilege ClusterRole, no write to workloads | Unauthorized API access |
| mTLS | TLS 1.3 minimum, mutual cert verification on all gRPC | Eavesdropping, MITM |
| Session tokens | UUID per transfer, bound to digest + nodes + TTL | Replay attacks |
| NetworkPolicy | Controller-to-agent and agent-to-agent traffic only | Lateral network movement |
| Container hardening | `readOnlyRootFilesystem`, `drop: ALL` caps, seccomp | Container escape |
| Validation webhook | Rejects unknown `tote.dev/*` annotations, fail-open | Typo-driven misconfigs |
| Image size limit | `--max-image-size` (default 2 GiB) | Resource exhaustion |
| Concurrency limit | `--max-concurrent-salvages` (default 2) | Cluster resource pressure |

## Agent privilege

The agent DaemonSet runs as root (`runAsUser: 0`) because containerd's Unix socket requires it. All other hardening is applied: capabilities dropped, filesystem read-only, seccomp enforced. The controller runs as non-root.

## What tote does NOT protect against

- **Image provenance** — does not verify signatures, SBOMs, or supply chain integrity
- **Secrets in images** — faithfully transfers whatever containerd has cached
- **Root compromise** — containerd socket is already accessible to root
- **etcd encryption** — SalvageRecords follow cluster-level encryption settings
- **Registry credential rotation** — reads from Kubernetes Secret at transfer time

## Recommended cluster hardening

- Enable mTLS: `--set tls.enabled=true`
- Enable NetworkPolicy: `--set networkPolicy.enabled=true`
- Enable validation webhook: `--set webhook.enabled=true`
- Restrict agent DaemonSet to worker nodes via `nodeSelector`
- Use Pod Security Standards (`restricted` for controller, `baseline` for agent)
- Enable etcd encryption for SalvageRecord CRDs
