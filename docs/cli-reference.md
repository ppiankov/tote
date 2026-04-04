# CLI Reference

## Controller flags

`tote` or `tote controller`:

| Flag | Default | Description |
|------|---------|-------------|
| `--enabled` | `true` | Global kill switch |
| `--metrics-addr` | `:8080` | Prometheus metrics endpoint |
| `--agent-namespace` | | Namespace where tote agents run (required for salvage) |
| `--agent-grpc-port` | `9090` | gRPC port for agent communication |
| `--max-concurrent-salvages` | `2` | Max parallel salvage operations |
| `--max-image-size` | `2147483648` | Max image size in bytes (0 = no limit) |
| `--session-ttl` | `5m` | Session lifetime for salvage operations |
| `--backup-registry` | | Registry host to push salvaged images |
| `--backup-registry-secret` | | dockerconfigjson Secret name |
| `--backup-registry-insecure` | `false` | Allow HTTP to backup registry |
| `--tls-cert` | | TLS certificate (enables mTLS) |
| `--tls-key` | | TLS private key |
| `--tls-ca` | | CA certificate for peer verification |
| `--json-log` | `false` | JSON log format |
| `--webhook-url` | | Webhook notification URL |
| `--webhook-events` | | Event types: detected, salvaged, salvage_failed, pushed, push_failed |
| `--salvagerecord-ttl` | `168h` | TTL for completed SalvageRecords |
| `--registry-resolve` | `false` | Enable registry-assisted tag resolution for tag-only images |
| `--registry-resolve-timeout` | `5s` | Timeout for registry tag resolution requests |
| `--registry-resolve-ca` | | Path to CA certificate for source registry TLS |
| `--registry-insecure` | `false` | Allow HTTP connections to source registries |

## Agent flags

`tote agent`:

| Flag | Default | Description |
|------|---------|-------------|
| `--containerd-socket` | `/run/containerd/containerd.sock` | Path to containerd socket |
| `--grpc-port` | `9090` | gRPC listen port |
| `--metrics-addr` | `:8081` | Prometheus metrics endpoint |
| `--tls-cert` | | TLS certificate |
| `--tls-key` | | TLS private key |
| `--tls-ca` | | CA certificate |

## Annotations

| Annotation | Target | Required | Description |
|------------|--------|----------|-------------|
| `tote.dev/allow` | Namespace | Yes | Enables tote for opted-in pods |
| `tote.dev/auto-salvage` | Pod/owner | Yes | Marks workload for detection |

Both must be `"true"`. `tote.dev/auto-salvage` is inherited via ownerReferences (up to 2 levels).

## Denied namespaces

Always excluded: `kube-system`, `kube-public`, `kube-node-lease`.

## Kubernetes events

| Reason | Type | Description |
|--------|------|-------------|
| `ImageSalvageable` | Warning | Digest found cached on other nodes |
| `ImageNotActionable` | Warning | Image uses tag, not digest |
| `ImageSalvaged` | Normal | Image transferred successfully |
| `ImageSalvageFailed` | Warning | Transfer failed |
| `ImageCorrupt` | Warning | Corrupt image record in containerd |
| `ImagePushed` | Normal | Pushed to backup registry |
| `ImagePushFailed` | Warning | Backup push failed (non-fatal) |

## Prometheus metrics

| Metric | Type | Description |
|--------|------|-------------|
| `tote_detected_failures_total` | Counter | Image pull failures detected |
| `tote_salvageable_images_total` | Counter | Failures with cached digest on other nodes |
| `tote_not_actionable_total` | Counter | Tag-only image failures (with `--registry-resolve`, only incremented when both agent and registry resolution fail) |
| `tote_salvage_attempts_total` | Counter | Salvage transfer attempts |
| `tote_salvage_successes_total` | Counter | Successful salvages |
| `tote_salvage_failures_total` | Counter | Failed salvages |
| `tote_push_attempts_total` | Counter | Backup push attempts |
| `tote_push_successes_total` | Counter | Successful pushes |
| `tote_push_failures_total` | Counter | Failed pushes |
| `tote_corrupt_images_total` | Counter | Corrupt images cleaned |
| `tote_salvage_duration_seconds` | Histogram | Salvage transfer time |
| `tote_push_duration_seconds` | Histogram | Backup push time |
| `tote_registry_resolve_total` | Counter | Registry tag resolution attempts (labels: `result=success\|failure\|not_found`) |
| `tote_registry_resolve_duration_seconds` | Histogram | Duration of registry tag resolution operations |
