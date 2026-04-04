# Tote — Operations Guide

## What tote is

Tote is a Kubernetes operator for emergency image recovery. It detects image pull failures (`ImagePullBackOff`, `ErrImagePull`) and automatically transfers cached copies of those images from other nodes in the cluster.

**In plain terms:** when a pod cannot pull its image from a registry (registry down, image deleted, network unreachable), tote finds that image in the containerd cache on another node and transfers it to the node that needs it. The pod starts, production stays up.

> **Important:** Tote is a band-aid. It saves production while you fix the root cause. Every tote alert means something is broken upstream (CI/CD, registry, network). Do not ignore alerts.

---

## What tote is NOT

- **NOT a CI/CD replacement** — if your pipeline is broken, tote saves you once, but the problem will return
- **NOT an image puller** — it only transfers images already cached on cluster nodes, never downloads from registries
- **NOT automatic** — requires explicit opt-in annotations on both namespace and pod/deployment
- **NOT for system namespaces** — `kube-system`, `kube-public`, `kube-node-lease` are always blocked
- **NOT a workload modifier** — it only transfers images and restarts pods, never modifies your specs
- **NOT compatible with `imagePullPolicy: Always`** — kubelet always contacts the registry, salvage cannot help

---

## Architecture

Tote consists of two components:

| Component | Kind | Role |
|-----------|------|------|
| **Controller** | Deployment (1 replica) | Watches pods, detects image pull failures, orchestrates image transfers |
| **Agent** | DaemonSet (one per node) | Serves images from the local containerd store over gRPC |

```
Pod -> ImagePullBackOff
  |
  +-- Controller detects the failure
       |
       +-- Looks up image digest in Node.Status.Images
       +-- If not found -> queries Agents via gRPC (containerd directly)
       +-- If not found -> queries the registry (if --registry-resolve is enabled)
       |
       +-- Found digest + found node with image -> SALVAGE
       |   +-- Transfers image from source node to target node
       |   +-- Creates a SalvageRecord (CRD)
       |   +-- Deletes the pod (if it has ownerReferences)
       |   +-- The owning controller (Deployment) creates a new pod -> starts immediately
       |
       +-- Not found anywhere -> NotActionable (alert, metric, event)
```

---

## Installation

### Step 1: Add the Helm repo

```bash
helm repo add ppiankov https://ppiankov.github.io/helm-charts
helm repo update
```

Or install from the local chart:

```bash
git clone https://github.com/ppiankov/tote.git
cd tote
```

### Step 2: Install tote

**Minimal install (recommended to start):**

```bash
helm install tote charts/tote -n tote-system --create-namespace
```

**Full install with monitoring and alerts:**

```bash
helm install tote charts/tote -n tote-system --create-namespace \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.labels.release=prometheus-operator \
  --set prometheusRule.enabled=true \
  --set prometheusRule.labels.release=prometheus-operator \
  --set registryResolve.enabled=true
```

> **Critical:** The `release=prometheus-operator` value must match the `ruleSelector` and `serviceMonitorSelector` of your Prometheus instance. Verify with:
> ```bash
> kubectl get prometheus -A -o jsonpath='{.items[0].spec.ruleSelector}'
> ```

### Step 3: Verify the installation

```bash
# Is the controller running?
kubectl get deploy -n tote-system
# Expected: tote   1/1   1   1   Running

# Are agents running?
kubectl get ds -n tote-system
# Expected: tote-agent   3   3   3   3   3   Running (count = number of nodes)

# Full health check:
kubectl exec -n tote-system deploy/tote -- tote doctor
```

Example `tote doctor` output:

```json
{
  "checks": [
    {"name": "kubeconfig", "status": "ok", "message": "cluster reachable"},
    {"name": "crd", "status": "ok", "message": "salvagerecords.tote.dev installed"},
    {"name": "controller", "status": "ok", "message": "1/1 replicas ready"},
    {"name": "agents", "status": "ok", "message": "3/3 agents ready"},
    {"name": "namespaces", "status": "ok", "message": "2 namespaces opted in: default, myapp"}
  ],
  "ok": true
}
```

If `ok: false`, see the Troubleshooting section below.

---

## Enabling for your application

Tote **does not act automatically**. You must explicitly enable it for each namespace and deployment.

### Step 1: Enable the namespace

```bash
kubectl annotate namespace <your-namespace> tote.dev/allow=true
```

**Verify:**

```bash
kubectl get ns <your-namespace> -o jsonpath='{.metadata.annotations.tote\.dev/allow}'
# Should print: true
```

### Step 2: Enable the deployment (or pod)

```bash
kubectl annotate deployment <your-deployment> -n <your-namespace> tote.dev/auto-salvage=true
```

**Verify:**

```bash
kubectl get deployment <your-deployment> -n <your-namespace> -o jsonpath='{.metadata.annotations.tote\.dev/auto-salvage}'
# Should print: true
```

> **Tip:** The annotation can be placed on a Deployment, StatefulSet, DaemonSet, Job, or bare Pod. Tote walks ownerReferences up to 2 levels (Pod -> ReplicaSet -> Deployment).

---

## Alerts (PrometheusRule)

When `prometheusRule.enabled=true`, tote creates 7 alerts:

| Alert | Severity | Trigger condition | What to do |
|-------|----------|-------------------|------------|
| `ToteImagePullFailureDetected` | warning | Image pull failures detected in the last 5 minutes | Check which pods are in `ImagePullBackOff`: `kubectl get pods -A \| grep ImagePull` |
| `ToteSalvageFailing` | critical | Salvage operations are failing over the last 10 minutes | Check controller logs and agent connectivity (gRPC port 9090) |
| `ToteNoSalvageableImages` | warning | More non-salvageable images than salvageable ones | Switch to digest references instead of tags |
| `ToteNotActionableSpike` | warning | More than 3 non-salvageable images in 5 minutes | Deployments reference images that exist on no node. Check the registry |
| `ToteNotActionableSustained` | critical | More than 10 non-salvageable images in 30 minutes with zero successes | The cluster has lost access to image sources. **Check registry and network immediately** |
| `ToteSalvageOccurred` | warning | At least one salvage completed successfully in the last 15 minutes | **Tote saved you, but the problem is not fixed.** The image was only available from a node cache. Push it to the registry — if the node is drained or rebooted, the image will be lost |
| `ToteControllerDown` | critical | Prometheus cannot scrape controller metrics for 5 minutes | Controller is down. Check: `kubectl get pods -n tote-system` |

---

## Warning vs Critical

- **warning** — something needs attention, but production is running. Schedule a fix
- **critical** — something is seriously broken. Act **now**

---

## Kubernetes Events

Tote emits Kubernetes events on every pod it processes. Events are the **primary tool for understanding what happened** to a specific pod.

### How to view events

```bash
# All tote events in a namespace:
kubectl get events -n <namespace> --field-selector reason=ImageSalvageable

# All tote event types on a specific pod:
kubectl describe pod <pod-name> -n <namespace> | grep -A5 tote

# JSON for parsing:
kubectl get events -n <namespace> -o json | jq '.items[] | select(.reason | startswith("Image")) | {reason, note: .note, pod: .regarding.name}'
```

### Event reference

| Event (reason) | Type | Meaning |
|----------------|------|---------|
| `ImageSalvageable` | Warning | Image found in another node's cache. Salvage will proceed |
| `ImageNotActionable` | Warning | Image NOT found on any node. Tote cannot help |
| `ImageResolvedUncached` | Warning | Image tag was resolved via the registry, but no node has cached this digest. The image exists in the registry but was never pulled into the cluster |
| `ImageSalvaged` | Warning | Image successfully transferred to the target node. Pod will be restarted |
| `ImageSalvageFailed` | Warning | Transfer failed. Check controller logs |
| `ImageCorrupt` | Warning | containerd has a corrupt image record. It will be cleaned up |
| `ImagePushed` | Normal | Image pushed to the backup registry |
| `ImagePushFailed` | Warning | Push to the backup registry failed |

> **Note:** All tote events are `Warning` except `ImagePushed` (Normal). This is **by design**: every tote action means something went wrong upstream. The Warning type serves as a reminder: fix the root cause.

---

## Prometheus Metrics

Metrics are exposed on port `8080` of the controller.

### How to check metrics

```bash
# Port-forward:
kubectl port-forward -n tote-system deploy/tote 8080:8080

# View all tote metrics:
curl -s localhost:8080/metrics | grep tote_
```

### Metric reference

| Metric | Type | Description |
|--------|------|-------------|
| `tote_detected_failures_total` | counter | Image pull failures detected |
| `tote_salvageable_images_total` | counter | Of those: image found in another node's cache |
| `tote_not_actionable_total` | counter | Of those: image NOT found anywhere (cannot salvage) |
| `tote_corrupt_images_total` | counter | Corrupt image records found in containerd |
| `tote_salvage_attempts_total` | counter | Salvage transfer attempts |
| `tote_salvage_successes_total` | counter | Successful transfers |
| `tote_salvage_failures_total` | counter | Failed transfers |
| `tote_push_attempts_total` | counter | Push attempts to the backup registry |
| `tote_push_successes_total` | counter | Successful pushes |
| `tote_push_failures_total` | counter | Failed pushes |
| `tote_salvage_duration_seconds` | histogram | Image transfer duration (seconds) |
| `tote_push_duration_seconds` | histogram | Backup registry push duration (seconds) |
| `tote_registry_resolve_total` | counter | Tag resolution attempts via registry (labels: `result=success\|failure\|not_found`) |
| `tote_registry_resolve_duration_seconds` | histogram | Tag resolution duration via registry |

### Key ratios

- `tote_detected_failures_total` = `tote_salvageable_images_total` + `tote_not_actionable_total` + `tote_corrupt_images_total`
- If `tote_not_actionable_total` is growing fast, images are not cached on any node. Check the registry
- If `tote_salvage_failures_total` is growing, there are problems with agents or network. Check logs

---

## Logs

### Where to find logs

```bash
# Controller logs (primary):
kubectl logs -n tote-system deploy/tote --tail=200

# Agent logs (DaemonSet):
kubectl logs -n tote-system ds/tote-agent --tail=200

# Agent logs on a specific node:
kubectl logs -n tote-system -l app.kubernetes.io/component=agent --field-selector spec.nodeName=<node-name> --tail=100

# Stream in real time:
kubectl logs -n tote-system deploy/tote -f
```

### Key log messages

| Message | Meaning | Action |
|---------|---------|--------|
| `image salvageable` | Image found, salvage starting | None needed — this is good |
| `image not actionable (tag-only, no cached digest found)` | Tag-only image not found on any node | Verify the image was pulled at least once. Consider digest references |
| `querying agents for tag resolution` | Controller is querying agents | Normal fallback for tag-only images |
| `resolved tag via agent` | An agent found the image | Good |
| `agents returned no digest` | No agent found the image | Image is not cached on any node |
| `querying source registry for tag resolution` | `--registry-resolve` is enabled, querying registry | Normal when registry-resolve is on |
| `resolved tag via registry` | Tag resolved via registry | Now searching nodes for the digest |
| `resolved via registry but no node has digest cached` | Image exists in registry but not on nodes | Image was never pulled into the cluster. Cannot salvage |
| `salvage rate limited` | Concurrent salvage limit reached | Increase `--max-concurrent-salvages` or wait |
| `image ... exceeds limit ... bytes` | Image exceeds size limit (default 2 GiB) | Increase `--max-image-size` or set to 0 (unlimited) |
| `connection refused` / `Unavailable` | Agent unreachable | Check agent pods, NetworkPolicy, mTLS configuration |

### JSON log format

For integration with log aggregation systems (ELK, Loki, Datadog), enable JSON output:

```bash
helm upgrade tote charts/tote -n tote-system --set config.jsonLog=true
```

Output format:

```json
{"level":"info","ts":"2026-04-04T10:30:00Z","logger":"controller.pod","msg":"image salvageable","container":"web","digest":"sha256:abc...","nodes":["node-1"]}
```

---

## SalvageRecord CRD

After each successful transfer, tote creates a `SalvageRecord` — a record of the salvaged image.

### How to query

```bash
# All records:
kubectl get salvagerecords -A

# Detailed view:
kubectl get salvagerecords -n <namespace> -o yaml

# JSON for parsing:
kubectl get salvagerecords -A -o json | jq '.items[] | {digest: .spec.digest, source: .spec.sourceNode, target: .spec.targetNode, phase: .status.phase}'
```

### Record structure

```json
{
  "spec": {
    "podName": "web-abc123",
    "digest": "sha256:abc123...",
    "imageRef": "nginx:1.25@sha256:abc123...",
    "sourceNode": "node-1",
    "targetNode": "node-2"
  },
  "status": {
    "phase": "Completed",
    "completedAt": "2026-04-04T10:30:00Z"
  }
}
```

> Records are automatically deleted after 7 days (configurable via `--salvagerecord-ttl`).

---

## Troubleshooting decision tree

Your pod is in `ImagePullBackOff`. Follow these steps in order:

```
1. Is tote running?
   kubectl get deploy -n tote-system
   -> NO -> helm install tote ...
   -> YES |
           v
2. Are agents running?
   kubectl get ds -n tote-system
   -> NO -> helm upgrade tote ... --set agent.enabled=true
   -> YES |
           v
3. Is the namespace enabled?
   kubectl get ns <ns> -o jsonpath='{.metadata.annotations.tote\.dev/allow}'
   -> NOT "true" -> kubectl annotate namespace <ns> tote.dev/allow=true
   -> "true" |
              v
4. Is the deployment/pod annotated?
   kubectl get deploy <name> -n <ns> -o jsonpath='{.metadata.annotations.tote\.dev/auto-salvage}'
   -> NOT "true" -> kubectl annotate deployment <name> -n <ns> tote.dev/auto-salvage=true
   -> "true" |
              v
5. Is tote_detected_failures_total increasing?
   kubectl port-forward -n tote-system deploy/tote 8080:8080
   curl -s localhost:8080/metrics | grep tote_detected
   -> NO -> Controller is not seeing this pod. Check RBAC, restart the controller
   -> YES |
           v
6. Is tote_salvageable_images_total increasing?
   curl -s localhost:8080/metrics | grep tote_salvageable
   -> NO -> Image is not cached on any node. Check tote_not_actionable_total
   -> YES |
           v
7. Is tote_salvage_successes_total increasing?
   curl -s localhost:8080/metrics | grep tote_salvage_successes
   -> NO -> Salvage is failing. Check logs: kubectl logs -n tote-system deploy/tote | grep salvage
   -> YES -> Salvage worked! The pod should restart
```

---

## Common problems

| Problem | Cause | Fix |
|---------|-------|-----|
| Controller in `CrashLoopBackOff` | CRD not installed | `kubectl apply -f charts/tote/crds/` |
| Controller `OOMKilled` (exit 137) | Too many pods in cluster, informer cache exceeds memory | Increase memory limit: see controller memory tuning table below |
| Alerts not firing | Labels do not match `ruleSelector` | Check: `kubectl get prometheus -A -o jsonpath='{.items[0].spec.ruleSelector}'` |
| `403 Forbidden` in logs | Insufficient RBAC permissions | Check ClusterRole: needs `list`, `watch`, `get` for pods, nodes, deployments, etc. |
| Agent not starting | containerd socket not accessible | Check path: default is `/run/containerd/containerd.sock` |
| `image not actionable` for all images | Images use tags without digests | Enable `--registry-resolve` or switch to digest references |
| Salvage worked but pod did not restart | Pod has no ownerReferences | Tote only deletes pods with an owner (Deployment, ReplicaSet, StatefulSet, etc.) |

---

## Controller memory tuning

| Pods in cluster | Recommended memory limit |
|-----------------|--------------------------|
| < 5,000 | 256Mi (default) |
| 5,000 -- 15,000 | 512Mi |
| 15,000+ | 1Gi |

Check your pod count:

```bash
kubectl get pods -A --no-headers | wc -l
```

Increase the limit:

```bash
helm upgrade tote charts/tote -n tote-system --set resources.limits.memory=512Mi
```

---

## Upgrading tote

### Helm-only upgrade (no image rebuild)

If only Helm templates changed (alerts, values, configuration):

```bash
helm upgrade tote charts/tote -n tote-system \
  --set serviceMonitor.enabled=true \
  --set prometheusRule.enabled=true \
  --set prometheusRule.labels.release=prometheus-operator \
  --set registryResolve.enabled=true
```

### Full version upgrade (image rebuild)

If Go code changed (new functionality, bug fixes):

```bash
helm upgrade tote charts/tote -n tote-system \
  --set image.tag=0.8.0 \
  --set serviceMonitor.enabled=true \
  --set prometheusRule.enabled=true \
  --set prometheusRule.labels.release=prometheus-operator \
  --set registryResolve.enabled=true
```

> Make sure the image `ghcr.io/ppiankov/tote:0.8.0` is already built and available. Otherwise you will get `ImagePullBackOff` (the irony).

---

## Full configuration reference

All Helm chart parameters:

```yaml
# === Image ===
image:
  repository: ghcr.io/ppiankov/tote
  tag: "0.8.0"
  pullPolicy: IfNotPresent

# === Core settings ===
config:
  enabled: true              # Global kill-switch
  metricsAddr: ":8080"       # Prometheus metrics address
  jsonLog: false             # JSON log format

# === Salvage settings ===
controller:
  maxConcurrentSalvages: 2   # Max parallel transfers
  sessionTTL: "5m0s"         # Transfer session TTL
  agentGRPCPort: 9090        # Agent gRPC port
  backupRegistry: ""         # Backup registry (empty = disabled)
  backupRegistrySecret: ""   # Secret with backup registry credentials
  backupRegistryInsecure: false
  salvageRecordTTL: "168h"   # Record TTL (7 days)

# === Registry-resolve (opt-in) ===
registryResolve:
  enabled: false             # Enable tag resolution via registry
  timeout: "5s"              # Registry query timeout
  ca: ""                     # CA certificate for internal registries
  insecure: false            # Allow plain HTTP

# === Webhooks ===
notifications:
  webhookUrl: ""             # Notification URL (empty = disabled)
  events: ""                 # Types: detected, salvaged, salvage_failed, pushed, push_failed

# === mTLS ===
tls:
  enabled: false
  secretName: ""             # TLS Secret (ca.crt, tls.crt, tls.key)

# === Monitoring ===
serviceMonitor:
  enabled: false
  interval: "30s"
  labels: {}                 # IMPORTANT: must match serviceMonitorSelector

prometheusRule:
  enabled: false
  labels: {}                 # IMPORTANT: must match ruleSelector

dashboard:
  enabled: true              # Grafana dashboard ConfigMap

# === Agent ===
agent:
  enabled: true
  containerdSocket: /run/containerd/containerd.sock
  grpcPort: 9090
  metricsAddr: ":8081"
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 256Mi
```

---

## FAQ

**Q: Tote is installed but nothing happens. Why?**
A: Check that both the namespace and the deployment are annotated. Without annotations, tote **does not act**.

**Q: Everything is in `ImagePullBackOff` but tote is not helping. What is wrong?**
A: Tote can only help if the image **already exists in the containerd cache** on at least one node. If the image was never pulled into the cluster, tote cannot do anything.

**Q: What does `ImageNotActionable` mean?**
A: The image uses a tag (not a digest) and tote could not find it on any node. Enable `registryResolve.enabled=true` or switch to digest references in your deployment specs.

**Q: What does `ImageResolvedUncached` mean?**
A: Tote found the image in the registry (tag -> digest), but no node in the cluster has cached that digest. The image exists in the registry but was never pulled to any node. Tote cannot help — the image needs to be pulled through normal means.

**Q: The `ToteSalvageOccurred` alert fired. What do I do?**
A: Tote **successfully saved** your pod, but this is a warning: the image was only available from a node cache. If that node is drained or rebooted, the image will be lost. **Push the image to the registry.**

**Q: The `ToteNotActionableSustained` alert fired. Is this serious?**
A: **Yes, this is critical.** The cluster cannot salvage images and there have been zero successful salvages in 30 minutes. Likely causes: registry unreachable, network broken, images deleted. **Investigate immediately.**

**Q: How do I disable tote for a specific namespace?**
A: Remove the annotation: `kubectl annotate namespace <ns> tote.dev/allow-`

**Q: How do I disable tote entirely without uninstalling?**
A: `helm upgrade tote charts/tote -n tote-system --set config.enabled=false`

---

## Why no per-image alerts?

A common question: "Why not send an alert for each specific image that fails to pull?"

The answer is **Prometheus metric cardinality**.

Prometheus stores metrics as time series. Each unique combination of metric name and label values creates a separate series. If you add an `image` label (with the specific image name) to a metric like `tote_not_actionable_total`, **every unique image creates a new time series permanently**.

In a cluster with hundreds of deployments and thousands of images, this means:

- **Memory consumption explodes** — Prometheus keeps all active series in RAM. Thousands of unique image names = thousands of new series = gigabytes of memory
- **Queries slow down** — every PromQL query against a high-cardinality metric scans all series. Alerts based on such metrics start slowing down the entire Prometheus instance
- **Unbounded growth** — image names include tags and hashes (`registry/app:build-12345`). Every new build = a new series that is never removed until the retention period expires

This is the classic **cardinality explosion** problem — one of the top causes of Prometheus failures in production.

### What tote does instead

| Mechanism | What it shows | Where to look |
|-----------|---------------|---------------|
| **Counter metrics** (no per-image labels) | Rate of problems | Prometheus / Grafana |
| **PrometheusRule alerts** | Spikes and sustained problems | Alertmanager / PagerDuty |
| **Kubernetes Events** (per-pod) | Specific image + specific pod | `kubectl get events` |
| **Controller logs** | Full details of every case | `kubectl logs` |

This layered approach achieves:

- Fast, reliable alerting via Prometheus (based on rates, not individual images)
- Per-image detail via Events and logs (without loading Prometheus)
- No risk of killing Prometheus with high cardinality

**In short:** Prometheus answers "is there a problem?", while Kubernetes Events and logs answer "which image?". Each tool does what it was designed for.
