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

### WO-10: CRDs for salvage tracking ✅
Replace pod annotations (`tote.dev/salvaged-digest`, `tote.dev/imported-at`) with a `SalvageRecord` CRD. Provides proper status tracking, history, and kubectl integration.

**Implemented**: `SalvageRecord` CRD in `api/v1alpha1/` (group: `tote.dev`, version: `v1alpha1`). Spec tracks pod name, digest, image ref, source/target nodes. Status has phase (`Completed`/`Failed`), completion time, and error. Generated via controller-gen v0.20.1. Replaces pod annotation-based tracking — idempotency now uses SalvageRecord list query. Records persist independently of pods. RBAC updated. Makefile `generate` target added.

### WO-11: Detect CreateContainerError (corrupt/incomplete images) ✅
Kubelet reports image "already present on machine" but containerd fails to unpack because content blobs (layers) are missing. Pod enters `CreateContainerError` with message `failed to resolve rootfs: content digest sha256:...: not found`.

**Implemented**: Detector extended to catch `CreateContainerError` with rootfs resolution failures. Agent `RemoveImage` RPC deletes stale image records from containerd. Controller removes corrupt record via agent, then deletes the pod for a fresh pull. Prometheus counter `tote_corrupt_images_total` tracks occurrences.

## v0.5 — Usability and notifications

### WO-12: Owner workload annotation inheritance ✅
Currently `tote.dev/auto-salvage` must be set directly on the Pod (or pod template). This is fine for Deployments (you annotate the template), but makes bulk opt-in tedious for clusters with many workloads.

**Implemented**: `isAutoSalvageEnabled()` walks ownerReferences up to 2 levels (Pod → RS → Deployment). Supports ReplicaSet, Deployment, StatefulSet, DaemonSet, Job. RBAC added for `apps/v1` and `batch/v1` `get`.

### WO-13: Webhook/Slack notifications ✅
Send notifications to external systems when tote detects or salvages images.

**Implemented**: `internal/notify` package with `Notifier` (JSON POST, 5s timeout, fire-and-forget). Event types: detected, salvaged, salvage_failed, pushed, push_failed. Wired into controller and orchestrator. Flags: `--webhook-url`, `--webhook-events`. Helm values: `notifications.webhookUrl`, `notifications.events`.

## v0.6 — Operational hardening

### WO-15: SalvageRecord TTL and cleanup ✅
**Implemented**: `internal/cleanup` package with `Reaper` implementing `manager.Runnable`. Periodic sweep (10min interval), deletes records older than TTL. Leader-election-aware. Flag: `--salvagerecord-ttl` (default `168h`). RBAC: added `delete` verb for salvagerecords.

### WO-16: Health and readiness probes ✅
**Implemented**: Controller: `HealthProbeBindAddress: ":8081"`, `healthz.Ping` for both healthz and readyz. Agent: gRPC health service via `google.golang.org/grpc/health`. Helm: HTTP probes on controller (port 8081), gRPC probes on agent, startup probe on agent with `failureThreshold: 10`.

### WO-17: PodDisruptionBudget and NetworkPolicy ✅
**Implemented**: PDB template (`minAvailable: 1`, `pdb.enabled: false`). NetworkPolicy templates for controller (metrics+health ingress, apiserver+agent+DNS egress) and agent (gRPC ingress from tote pods, DNS+registry egress). `networkPolicy.enabled: false` by default.

### WO-18: Salvage duration and size histograms ✅
**Implemented**: `tote_salvage_duration_seconds` and `tote_push_duration_seconds` histograms with buckets `{0.5, 1, 2, 5, 10, 30, 60, 120, 300}`. Instrumented in `Orchestrator.Salvage()` and `pushToBackupRegistry()`.

### WO-19: Structured JSON logging ✅
**Implemented**: `--json-log` flag on both controller and agent. JSON mode uses production zap config; default uses development mode (console). Helm value: `config.jsonLog: false`.

### WO-20: Reconcile requeue on salvage failure ✅
**Implemented**: `isTransientError()` classifies rate-limit, connection, and gRPC errors as transient. Returns `Result{RequeueAfter: 30s}` for transient failures; permanent failures (image too large, no source) return without requeue.

### WO-21: SalvageRecord index for idempotency ✅
**Implemented**: Field indexer on `spec.digest` in `SetupWithManager`. `hasSalvageRecord()` uses `client.MatchingFields{"spec.digest": digest}` for O(1) lookup. Fake client tests use `WithIndex()`.

## Security hardening

### WO-22: Agent security context ✅
**Implemented**: Pod-level `seccompProfile: RuntimeDefault` on agent DaemonSet. Container already has `runAsUser: 0`, `readOnlyRootFilesystem: true`, `capabilities: { drop: [ALL] }`.

### WO-23: Annotation validation webhook ✅
**Implemented**: `internal/webhook` package with `AnnotationValidator`. Rejects unknown `tote.dev/*` annotations and non-boolean values. Fail-open on decode errors. Helm: `ValidatingWebhookConfiguration` + Service, `webhook.enabled: false` by default, `failurePolicy: Ignore`.

## Testing

### WO-24: End-to-end tests with kind ✅
**Implemented**: `test/e2e/` with build-tagged (`//go:build e2e`) tests. Tests: CRD installed, controller running, unreachable image emits event. `kind-config.yaml` for single-node cluster. Makefile: `e2e-setup`, `e2e`, `e2e-teardown` targets.

### WO-25: Helm chart validation ✅
**Implemented**: `make helm-lint` target: `helm lint` + `helm template` with 5 value combinations (default, TLS, dashboard, PDB+NetworkPolicy).

## Housekeeping

### WO-14: Tag v0.4.0 release
No git tag exists for v0.4.0. The release workflow (`release.yml`) triggers on `v*` tags and builds binaries, container images, and GitHub releases automatically.

**Steps:**
1. Verify CI passes on current main
2. `git tag v0.4.0 && git push origin v0.4.0`
3. Verify release workflow: binaries built, checksums generated, container image pushed to ghcr.io
4. Verify GitHub release page has correct changelog body
5. Clean up: add `tote` binary to `.gitignore` if not already there
