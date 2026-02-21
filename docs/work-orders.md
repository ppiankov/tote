# Work Orders

## v0.2.x — Quick wins

### WO-1: Pod restart after salvage
After successful salvage, delete the pod so the owning controller (ReplicaSet/Deployment) recreates it immediately. Currently pods sit in ImagePullBackOff for up to 5 minutes waiting for kubelet's backoff timer. Only delete pods with owner references (never standalone pods).

### WO-2: Demote debug logging
Reduce Info-level agent resolution logs (`querying agents for tag resolution`, per-agent errors) to V(1). Production controllers should not emit per-reconcile info logs for normal operation.

### WO-3: Image size guard
Add `--max-image-size` flag (default: 2GB). Skip salvage for images exceeding the limit. Emit an event explaining why. Prevents large image transfers from starving node bandwidth.

### WO-4: Update README
README still describes v0.1 (detection only). Update: What tote is/is NOT, architecture section (add agents, transfer, session packages), CLI flags table (add controller and agent subcommands), reconciliation flow diagram (add salvage path), RBAC requirements (add pod delete/patch).

## v0.3 — Registry push and observability

### WO-5: Registry push ✅
After successful salvage, optionally push the image to a configurable backup registry via `--backup-registry`. Controller reads credentials from a dockerconfigjson Secret (`--backup-registry-secret`), passes them to the source agent via gRPC. Agent uses `go-containerregistry` to export from containerd and push. Push is non-fatal — failure logged + event + metric. New package: `internal/registry` (push, ref rewriting, credential extraction). Three new Prometheus counters: `tote_push_{attempts,successes,failures}_total`.

### WO-6: Grafana dashboard ✅
Ship `charts/tote/dashboards/tote.json` with panels: salvage rate, failure rate, detected vs salvageable ratio, not-actionable count. Add a ConfigMap in the Helm chart for Grafana sidecar auto-discovery.

**Implemented**: Dashboard JSON with 3 rows (Detection, Salvage, Backup registry push), stat panels for all 10 counters, time-series rate panels, pie chart for detected/salvageable/not-actionable ratio, gauge for salvage failure rate. ConfigMap with `grafana_dashboard: "1"` label for sidecar auto-discovery. Toggled via `dashboard.enabled` in values.yaml.

### WO-7: CI optimization ✅
**Root cause found**: golangci-lint's package loader (`golang.org/x/tools/go/packages.Load`) type-checks all 297 transitive dependencies (containerd, gRPC, k8s) for ANY linter needing type info — even `govet`. This takes 50+ min locally. `go vet` runs instantly because it uses a more efficient loader.

**Fixed locally**: `.golangci.yml` now only enables `ineffassign` (no type info) + `gofmt` (formatter). `make lint` runs `go vet` first, then golangci-lint. Total: ~2 seconds.

**Implemented**: CI split into 4 parallel jobs: test (Go matrix), lint-fast (go vet + golangci-lint with fast config), lint-full (golangci-lint with `.golangci-full.yml` — errcheck, staticcheck, unused + caching), build. Removed dead code flagged by staticcheck.

## v1.0 — Production hardening

### WO-8: mTLS between agents ✅
Add mutual TLS for gRPC agent-to-agent and controller-to-agent communication. Generate certs via cert-manager or mount from Secrets. Required for regulated environments.

**Implemented**: New `internal/tlsutil` package with `ServerCredentials` and `ClientCredentials` functions. TLS 1.3 minimum, `RequireAndVerifyClientCert` on server, fixed `ServerName: "tote"` for hostname verification. Optional via `--tls-cert`, `--tls-key`, `--tls-ca` flags on both controller and agent. All 6 insecure gRPC client connections and the agent server now conditionally use mTLS. Helm chart supports `tls.enabled` and `tls.secretName` with volume mounts on both Deployment and DaemonSet. Compatible with cert-manager Certificate resources.

### WO-9: Leader election
Enable controller-runtime leader election (`ctrl.Options{LeaderElection: true}`). Required for running multiple controller replicas safely.

### WO-10: CRDs for salvage tracking
Replace pod annotations (`tote.dev/salvaged-digest`, `tote.dev/imported-at`) with a `SalvageRecord` CRD. Provides proper status tracking, history, and kubectl integration.

### WO-11: Detect CreateContainerError (corrupt/incomplete images) ✅
Kubelet reports image "already present on machine" but containerd fails to unpack because content blobs (layers) are missing. Pod enters `CreateContainerError` with message `failed to resolve rootfs: content digest sha256:...: not found`.

**Implemented**: Detector extended to catch `CreateContainerError` with rootfs resolution failures. Agent `RemoveImage` RPC deletes stale image records from containerd. Controller removes corrupt record via agent, then deletes the pod for a fresh pull. Prometheus counter `tote_corrupt_images_total` tracks occurrences.
