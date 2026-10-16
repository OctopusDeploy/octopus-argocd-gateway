# Load Test Plan: octopus-argocd-gateway

## 1. Overview

This document describes how to perform load testing of the octopus-argocd-gateway in a real Kubernetes cluster with real Argo CD and Octopus Deploy instances. The goal is to understand how the gateway behaves under increasing load and to identify performance bottlenecks, resource limits, and failure modes.

The gateway is a gRPC **client** that connects outward to both Octopus Deploy and Argo CD. It does not expose any endpoints itself. Load is therefore not driven by sending requests to the gateway, but by increasing the volume of work the gateway must process: more applications, more frequent changes, larger resource trees, and more concurrent operations requested by Octopus.

---

## 2. What the Gateway Does Under Load

When running, the gateway maintains these concurrent workloads:

| Workload | Trigger | Per-app cost |
|----------|---------|-------------|
| **Application event watching** | Any ArgoCD app change (spec, status, health) | 1 shared watch stream; event processing + `GetCluster` call per event |
| **Resource tree watching** | Octopus requests resource state for apps | 3 goroutines + 1 gRPC stream per app, polls every 5s, hashes resource tree with SHA-256 |
| **Application sync** | Octopus triggers a sync | `GetApplication` + `SyncApplication` per request (synchronous) |
| **Manifest fetching** | Octopus requests resource manifests | `GetManagedResources` or `GetResourceLiveManifest` per request (synchronous) |
| **Pod log streaming** | Octopus requests pod logs | Opens log stream, collects up to 1000 tail lines per request |
| **Resource events** | Octopus requests K8s events for a resource | `GetResourceEvents` per request |
| **Info / gateway logs** | Octopus polls gateway health | `GetVersion` call to ArgoCD per request |

Key architectural constraints:
- All Octopus-facing stream services (sync, manifests, pod logs, resource events, info) use a **synchronous recv-process-send loop** -- only one request is in flight per service at a time.
- The resource watcher is the exception: it runs **one goroutine group per application** concurrently.
- Stream retry uses **recursive restart** with backoff (5s default), meaning sustained failures grow the call stack.

---

## 3. Test Environment Setup

### 3.1 Kubernetes Cluster

Provision a dedicated test cluster (or namespace) with:
- At least 3 worker nodes with 4 CPU / 8GB RAM each (to avoid node-level bottlenecks masking gateway issues)
- Metrics collection enabled (Prometheus + Grafana, or equivalent)
- The gateway Helm chart installed with resource requests/limits **removed** initially (to observe natural resource consumption), then re-tested with limits applied

### 3.2 Argo CD Instance

Install Argo CD in the test cluster. Key configuration:
- Use a dedicated Argo CD instance for load testing (not shared with production)
- Ensure the ArgoCD API server has sufficient resources (at minimum 2 CPU / 4GB)
- Consider tuning `server.repo.server.parallelism.limit` if repo operations become a bottleneck

### 3.3 Octopus Deploy Instance

Use a test Octopus Deploy instance (cloud or self-hosted):
- Register the gateway to a dedicated test space
- The Octopus instance controls how frequently it polls the gateway for manifests, events, logs, etc. -- this is a key load driver (see Section 4)

### 3.4 Gateway Deployment

Deploy the gateway using the Helm chart with these initial values:

```yaml
gateway:
  debug: false  # set to true only for troubleshooting, adds overhead
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  # no limits initially -- let it consume what it needs
```

---

## 4. Load Parameters and How to Control Them

### 4.1 Number of ArgoCD Applications (Primary Load Driver)

This is the single most important parameter. Each application tracked by the gateway creates:
- ~3 goroutines for resource watching (main loop, event listener, polling loop)
- 1 persistent gRPC stream to ArgoCD's `WatchResourceTree`
- A `GetApplicationResourceTree` poll every 5 seconds
- SHA-256 hash computation of the full resource tree on every poll

**How to control:**
- Use a script or ArgoCD ApplicationSet to create N applications pointing to a simple Git repo (e.g., a repo with a single ConfigMap or Deployment)
- Test levels: **10, 50, 100, 250, 500, 1000** applications

**Example ApplicationSet for generating test apps:**
```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: loadtest
  namespace: argocd
spec:
  generators:
  - list:
      elements:
      # generate entries via script, e.g., [{name: app-0}, {name: app-1}, ...]
      - name: app-0
      - name: app-1
      # ... up to app-N
  template:
    metadata:
      name: "{{name}}"
    spec:
      project: default
      source:
        repoURL: https://github.com/your-org/loadtest-manifests
        targetRevision: main
        path: "base"  # a single directory with a simple Deployment
      destination:
        server: https://kubernetes.default.svc
        namespace: "loadtest-{{name}}"
```

Or generate with a script:
```bash
for i in $(seq 0 $N); do
  argocd app create "app-$i" \
    --repo https://github.com/your-org/loadtest-manifests \
    --path base \
    --dest-server https://kubernetes.default.svc \
    --dest-namespace "loadtest-app-$i" \
    --project default
done
```

### 4.2 Resource Tree Complexity (Nodes per Application)

Each application's resource tree can have few or many nodes. More nodes means:
- Larger payloads in `WatchResourceTree` and `GetApplicationResourceTree` responses
- More expensive SHA-256 hash computation (JSON marshal + hash of all nodes)
- More data mapped and sent to Octopus per update

**How to control:**
- Use application manifests with varying complexity:
  - **Simple:** 1 Deployment (creates ~3-5 nodes: Deployment, ReplicaSet, Pod)
  - **Medium:** 1 Deployment + Service + ConfigMap + Secret + HPA (~10-15 nodes)
  - **Complex:** A Helm chart like `kube-prometheus-stack` or a custom chart that creates 50+ resources
  - **Extreme:** Deploy a chart with 100+ CRDs/resources (e.g., Istio, Crossplane)

### 4.3 Application Change Frequency

Application changes trigger events through the `Watch` stream. Higher change frequency means more `ApplicationEventUpdate` RPCs to Octopus and more cluster annotation lookups.

**How to control:**
- Run a CronJob or script that periodically modifies applications:
  ```bash
  # Modify apps every N seconds to generate events
  while true; do
    for i in $(seq 0 $APP_COUNT); do
      kubectl -n "loadtest-app-$i" annotate deployment my-app \
        loadtest-ts="$(date +%s)" --overwrite
    done
    sleep $INTERVAL  # e.g., 5, 10, 30, 60 seconds
  done
  ```
- Test levels: changes every **60s, 30s, 10s, 5s** across all apps

### 4.4 Sync Request Rate

Sync operations are triggered by Octopus. Each sync is synchronous on the gateway's single sync stream.

**How to control:**
- Trigger syncs from Octopus Deploy by creating deployments/runbooks that sync apps
- Alternatively, use the Octopus API to programmatically trigger syncs at a controlled rate
- Test levels: **1, 5, 10, 20** concurrent sync triggers (noting that the gateway processes them sequentially)

### 4.5 Manifest and Event Request Rate

These are driven by Octopus UI interactions (viewing resource details, logs, events).

**How to control:**
- Simulate user activity by opening multiple application detail pages in Octopus simultaneously
- Or use the Octopus API/UI automation to repeatedly request resource details
- Test levels: simulate **1, 5, 10, 20** concurrent users viewing app details

### 4.6 Pod Log Request Volume

Log requests stream data from Kubernetes through ArgoCD to the gateway and back to Octopus. The gateway collects up to 1000 tail lines per request in memory.

**How to control:**
- Deploy applications that produce high log volumes (e.g., a container that prints continuously)
- Request logs through Octopus for multiple pods simultaneously
- Test with pods producing: **quiet** (1 line/min), **moderate** (10 lines/s), **noisy** (1000+ lines/s)

### 4.7 Gateway Pod Resources

**How to control via Helm values:**
```yaml
resources:
  requests:
    cpu: "100m"     # vary: 100m, 250m, 500m, 1000m
    memory: "128Mi" # vary: 128Mi, 256Mi, 512Mi, 1Gi
  limits:
    cpu: "500m"     # vary to test throttling behavior
    memory: "512Mi" # vary to test OOM behavior
```

### 4.8 Network Conditions

**How to control:**
- Use network policies or tools like `tc` (traffic control) to add latency between gateway and ArgoCD/Octopus
- Test with: **0ms** (same cluster), **50ms**, **200ms**, **500ms** added latency
- Test with bandwidth limits to simulate cross-region deployments

### 4.9 Stream Retry Backoff

The `--stream-retry-backoff` flag controls how long the gateway waits before reconnecting after a stream error.

**How to control:**
- Set via Helm chart environment variable or command args
- Test with: **0s** (immediate), **5s** (default), **15s**, **30s**

---

## 5. Test Scenarios

### 5.1 Baseline: Steady-State Resource Consumption

**Goal:** Establish baseline resource usage at different application counts with no active changes.

| Step | Action | Duration | Measure |
|------|--------|----------|---------|
| 1 | Deploy gateway + 10 stable apps | 10 min | CPU, memory, goroutine count, gRPC stream count |
| 2 | Scale to 50 apps | 10 min | Same |
| 3 | Scale to 100 apps | 10 min | Same |
| 4 | Scale to 250 apps | 10 min | Same |
| 5 | Scale to 500 apps | 15 min | Same |
| 6 | Scale to 1000 apps | 15 min | Same |

**Expected results:**
- Memory and goroutines should scale roughly linearly with app count
- At 1000 apps: ~3000+ goroutines, significant heap usage from 1000 concurrent `GetApplicationResourceTree` polls every 5s
- CPU should be driven by SHA-256 hash computation and JSON marshaling on every poll

**Pass criteria:**
- Gateway remains stable (no crashes, no OOM)
- Octopus receives resource updates for all applications
- No significant increase in ArgoCD API error rates

### 5.2 Event Storm: High Application Change Rate

**Goal:** Test gateway behavior when many applications change simultaneously.

| Step | Action | Duration | Measure |
|------|--------|----------|---------|
| 1 | Deploy 100 apps, let stabilize | 5 min | Baseline |
| 2 | Modify all 100 apps simultaneously (annotation change) | Observe | Event processing latency, `ApplicationEventUpdate` success rate |
| 3 | Repeat modification every 10s for 5 min | 5 min | Sustained event throughput, memory growth |
| 4 | Repeat with 500 apps, modifications every 30s | 10 min | Same |

**What to watch:**
- The event handler calls `GetCluster` for every event -- this could become a bottleneck
- Events are processed on the single `WatchRunner` loop; if processing is slow, events queue up in the gRPC stream buffer
- Check Octopus: are all events received in order? Any dropped?

### 5.3 Sync Storm: Concurrent Sync Operations

**Goal:** Test how the gateway handles many sync requests from Octopus.

| Step | Action | Duration | Measure |
|------|--------|----------|---------|
| 1 | Deploy 50 apps | 5 min | Baseline |
| 2 | Trigger sync on 10 apps simultaneously from Octopus | Observe | Time for all syncs to complete, error rate |
| 3 | Trigger sync on 50 apps simultaneously | Observe | Same + check if stream backpressure causes timeouts |

**What to watch:**
- Syncs are processed sequentially on a single stream -- 50 syncs will queue
- Each sync does `GetApplication` + `SyncApplication` (two ArgoCD API calls)
- Octopus may time out waiting for a response if the queue is deep enough

### 5.4 Large Resource Trees

**Goal:** Test gateway with applications that have complex resource trees.

| Step | Action | Duration | Measure |
|------|--------|----------|---------|
| 1 | Deploy 10 apps with ~100 resources each (e.g., kube-prometheus-stack) | 10 min | CPU, memory, hash computation time |
| 2 | Deploy 10 apps with ~500 resources each | 10 min | Same |
| 3 | Deploy 50 apps with ~100 resources each | 10 min | Combined scale + complexity |

**What to watch:**
- CPU from `hashNodes()` which JSON-marshals and SHA-256 hashes all nodes every 5s per app
- Memory from large `ApplicationTree` payloads held during processing
- gRPC message sizes approaching the 20MiB Octopus limit or 200MiB ArgoCD limit

### 5.5 Resilience: Connection Disruption

**Goal:** Test gateway recovery when connections to ArgoCD or Octopus are interrupted.

| Step | Action | Duration | Measure |
|------|--------|----------|---------|
| 1 | Deploy 100 apps, let stabilize | 5 min | Baseline |
| 2 | Kill the ArgoCD API server pod | Observe | Gateway logs, reconnection time, event loss |
| 3 | ArgoCD recovers; observe gateway | 10 min | Does it resume all 100 resource watchers? Memory/goroutine leak? |
| 4 | Simulate Octopus network partition (NetworkPolicy blocking Octopus gRPC) | 5 min | Gateway behavior during partition |
| 5 | Remove NetworkPolicy | 10 min | Recovery time, state consistency |

**What to watch:**
- The gateway uses recursive `restartWithBackoff()` -- repeated failures grow the call stack
- The `applicationresources` handler recreates watchers on reconnect -- verify no goroutine leaks (old watchers should be cleaned up)
- Check if the unbuffered `resultsChan` causes deadlocks when the stream is down

### 5.6 Sustained Load: Long-Running Stability

**Goal:** Verify the gateway is stable over an extended period under moderate load.

| Step | Action | Duration | Measure |
|------|--------|----------|---------|
| 1 | Deploy 100 apps | - | - |
| 2 | Continuously modify 10 random apps every 30s | 4 hours | Memory growth over time (leak detection) |
| 3 | Trigger 2 syncs/minute | Same | Accumulated error count |
| 4 | Request manifests for 5 random apps every minute | Same | Latency drift |

**Pass criteria:**
- Memory usage does not grow unboundedly (should stabilize after GC)
- No goroutine leaks (count should remain stable)
- Error rates do not increase over time
- Gateway does not require restart

### 5.7 Resource Limits: OOM and CPU Throttling

**Goal:** Determine appropriate resource limits for production.

| Step | Action | Limits | Measure |
|------|--------|--------|---------|
| 1 | Run scenario 5.1 (500 apps) with memory limit 256Mi | `memory: 256Mi` | OOM kill? At what app count? |
| 2 | Same with 512Mi | `memory: 512Mi` | Same |
| 3 | Same with 1Gi | `memory: 1Gi` | Same |
| 4 | Run scenario 5.2 (100 apps, event storm) with CPU limit 250m | `cpu: 250m` | Throttling? Event processing delay? |
| 5 | Same with 500m | `cpu: 500m` | Same |
| 6 | Same with 1000m | `cpu: 1000m` | Same |

---

## 6. Metrics to Collect

### 6.1 Gateway Pod Metrics (via Prometheus/kubectl top)

| Metric | How to collect | Why |
|--------|---------------|-----|
| CPU usage | `kubectl top pod` / Prometheus `container_cpu_usage_seconds_total` | Detect throttling, identify CPU-bound operations |
| Memory usage | `kubectl top pod` / Prometheus `container_memory_working_set_bytes` | Detect leaks, determine limits |
| Restarts | `kubectl get pod -w` / Prometheus `kube_pod_container_status_restarts_total` | OOM kills, crashes |
| Network I/O | Prometheus `container_network_transmit_bytes_total` | gRPC traffic volume |

### 6.2 Gateway Logs

Enable debug logging (`gateway.debug: true`) selectively and grep for:

| Log pattern | Indicates |
|------------|-----------|
| `Recoverable stream error` | Stream disconnects (should recover) |
| `Unrecoverable stream error` | Fatal errors (gateway may stop) |
| `Resource watcher TTL expired` | Watchers timing out (Octopus not re-requesting) |
| `Failed to send error to Octopus` | Stream send failures |
| `Error getting application from ArgoCD` | ArgoCD API failures under load |
| `Attempting to restart subscriber` | Stream reconnection attempts |

### 6.3 ArgoCD Metrics

| Metric | Source | Why |
|--------|--------|-----|
| API server request rate | ArgoCD Prometheus metrics | Verify gateway isn't overwhelming ArgoCD |
| API server error rate | `argocd_server_*` metrics | Detect ArgoCD-side failures |
| API server latency | `argocd_server_request_duration` | Slow ArgoCD = slow gateway |

### 6.4 Octopus Deploy Side

| What to check | How | Why |
|---------------|-----|-----|
| Resource state freshness | Compare Octopus UI state vs `kubectl` | Detect stale data from slow updates |
| Event delivery completeness | Check Octopus activity log vs ArgoCD events | Detect dropped events |
| Sync completion time | Octopus deployment logs | End-to-end latency |

### 6.5 Go Runtime Metrics (via pprof)

If the gateway is built with pprof enabled (or if you add it temporarily), you can collect:

```bash
# Port-forward to the gateway pod (requires adding pprof endpoint)
kubectl port-forward deploy/octopus-argocd-gateway 6060:6060

# Goroutine count and profile
curl http://localhost:6060/debug/pprof/goroutine?debug=1

# Heap profile
curl http://localhost:6060/debug/pprof/heap > heap.prof
go tool pprof heap.prof

# CPU profile (30 seconds)
curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof
```

> Note: Adding a pprof endpoint requires a small code change to start `net/http/pprof` on a debug port. This is recommended for load testing builds.

---

## 7. Known Risks and What to Validate

| # | Risk | How it manifests | How to detect | Test scenario |
|---|------|-----------------|---------------|---------------|
| 1 | **Unbuffered results channel bottleneck** -- All resource watchers send results through a single unbuffered channel with one consumer goroutine | Watcher goroutines block on channel send; resource updates delayed at scale | Goroutine profile shows many goroutines blocked on channel send | 5.1 at 500+ apps |
| 2 | **Recursive restart stack growth** -- `restartWithBackoff()` calls `Start()` recursively on stream errors | Stack overflow after many consecutive failures | Gateway crash with stack overflow; goroutine profile shows deep stacks | 5.5 with repeated ArgoCD kills |
| 3 | **Unbounded pod log memory** -- `processPodLogs()` appends to a slice without size cap | Memory spike when collecting logs from noisy containers | Memory spike during pod log requests; possible OOM | 5.4 with noisy containers |
| 4 | **Hash computation cost at scale** -- `hashNodes()` JSON-marshals + SHA-256 hashes entire resource trees every 5s per app | High CPU at scale; proportional to total_apps x nodes_per_app | CPU profile dominated by `json.Marshal` and `sha256.Sum256` | 5.4 (large trees) |
| 5 | **Sequential stream processing** -- Sync, manifests, etc. process one request at a time | Deep request queues when Octopus sends many requests; possible timeouts | Sync/manifest requests take increasingly long as load increases | 5.3 |
| 6 | **Goroutine leak on reconnection** -- Old resource watchers may not be cleaned up on stream reconnect | Goroutine count grows over time without corresponding app count increase | Goroutine count trend in Prometheus | 5.5, 5.6 |

---

## 8. Test Execution Checklist

### Preparation

- [ ] Dedicated test cluster provisioned with monitoring (Prometheus + Grafana)
- [ ] Argo CD installed and accessible
- [ ] Octopus Deploy test instance available with a dedicated space
- [ ] Gateway registered and running
- [ ] Test application manifests repo created (simple Deployment + Service)
- [ ] ApplicationSet or script ready to create N applications
- [ ] Change-generator script ready (Section 4.3)
- [ ] Grafana dashboards configured for gateway pod CPU/memory/restarts
- [ ] Baseline metrics recorded (cluster with gateway, 0 applications)

### Execution Order

1. **5.1** Baseline (establishes resource-per-app cost)
2. **5.7** Resource limits (determines safe limits before stress tests)
3. **5.2** Event storm (tests event processing path)
4. **5.3** Sync storm (tests request processing path)
5. **5.4** Large resource trees (tests data processing path)
6. **5.5** Resilience (tests recovery behavior)
7. **5.6** Sustained load (tests long-term stability -- run last, takes hours)

### After Each Scenario

- [ ] Record peak CPU, peak memory, goroutine count
- [ ] Check gateway logs for errors
- [ ] Verify Octopus is receiving correct data
- [ ] Check for restarts/OOM kills
- [ ] Allow 5 minutes cool-down between scenarios

---

## 9. Reporting Template

For each test scenario, record:

```
Scenario: [name]
Date: [date]
Duration: [duration]
Application count: [N]
Resource nodes per app: [avg]

Gateway Pod:
  CPU peak:        [value]
  CPU average:     [value]
  Memory peak:     [value]
  Memory average:  [value]
  Restarts:        [count]
  Goroutines peak: [count]

ArgoCD:
  API error rate:  [%]
  API p99 latency: [ms]

Octopus:
  Events received: [count] / [expected]
  Sync success:    [count] / [attempted]
  Stale data:      [yes/no, details]

Findings:
  [bullet points of observations, bottlenecks, failures]

Recommendation:
  [resource limits, configuration changes, code improvements]
```

---

## 10. Recommended Production Sizing (To Be Validated)

These are starting estimates to validate through testing:

| Application count | CPU request | Memory request | CPU limit | Memory limit |
|-------------------|-------------|----------------|-----------|-------------|
| 1-50              | 100m        | 128Mi          | 500m      | 256Mi       |
| 50-200            | 250m        | 256Mi          | 1000m     | 512Mi       |
| 200-500           | 500m        | 512Mi          | 2000m     | 1Gi         |
| 500-1000          | 1000m       | 1Gi            | 2000m     | 2Gi         |

These should be refined based on actual test results, especially the per-app memory cost and hash computation CPU cost.
