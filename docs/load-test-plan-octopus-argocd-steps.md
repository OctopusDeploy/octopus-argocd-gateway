# Load Test Plan: Octopus Deploy ArgoCD Deployment Steps

## 1. Overview

This document describes how to load test the Octopus Deploy server-side ArgoCD deployment steps -- the operations that pull, modify, commit, and push changes to Git repositories watched by ArgoCD. These steps run inside the Octopus Deploy execution engine (Calamari) and are the most Git-intensive part of the ArgoCD integration.

Unlike the gateway (which is a gRPC relay), these steps perform real Git operations (clone, checkout, file modification, commit, push) and interact with Git hosting providers (GitHub, GitLab, BitBucket, Azure DevOps) for pull request creation. Repository structure, size, and hosting provider performance are all significant load factors.

---

## 2. What the Deployment Steps Do

### Step Type 1: Update ArgoCD Application Image Tags

Full workflow per application source:
1. **Clone** the Git repository to a working directory
2. **Checkout** the target branch/revision
3. **Parse** YAML files to find container image references
4. **Replace** image tags in one of three modes:
   - **Directory mode**: Regex replacement in raw YAML manifests across 11 K8s resource types (Deployment, StatefulSet, DaemonSet, Job, CronJob, ReplicaSet, ReplicationController, Pod, PodTemplate, plus Argo Rollouts)
   - **Helm mode**: Update `values.yaml` (or custom values files) at specified YAML paths
   - **Kustomize mode**: Update `kustomization.yaml` images section and optionally patch files
5. **Commit** changes (author: `Octopus <octopus@octopus.com>`)
6. **Push** to target branch, or to a feature branch if pull request mode is enabled
7. **Create pull request** via Git provider API (GitHub/GitLab/BitBucket/Azure DevOps) if configured
8. Push retries on failure: fetch + rebase, up to 2 retries with exponential backoff

### Step Type 2: Update ArgoCD Application Manifests

1. **Extract** files from a package (NuGet/Zip)
2. **Apply** Octopus variable substitution to manifest files
3. **Clone** the Git repository
4. **Optionally purge** all files in the target directory (except `.git/`)
5. **Copy** package files into the repository at the application source path
6. **Commit and push** (same push/PR logic as above)

### Key Multipliers

A single Octopus deployment can trigger updates to **multiple ArgoCD applications**, each with **multiple sources**, each pointing to **different repositories and branches**. So one deployment might:
- Clone 5 different repositories
- Modify files across 10 application sources
- Push 10 commits to 5 repos
- Create 5 pull requests

---

## 3. Load Parameters and How to Control Them

### 3.1 Number of ArgoCD Applications per Deployment

Each application in the deployment scope gets processed. More applications = more clone/modify/push cycles.

**How to control:**
- Configure the deployment step with varying numbers of ArgoCD applications in scope
- Use environment/project/tenant scoping annotations to include/exclude applications
- Test levels: **1, 5, 10, 25, 50** applications per deployment

### 3.2 Number of Sources per Application (Multi-Source Apps)

ArgoCD supports multi-source applications. Each source is processed independently (separate clone, modify, push cycle).

**How to control:**
- Create ArgoCD Application manifests with 1 vs. multiple sources
- Each source can point to the same or different repository
- Test levels: **1, 3, 5** sources per application

### 3.3 Repository Structure: Mono-repo vs. Multi-repo

This is a critical parameter because it determines clone cost, push contention, and file discovery scope.

**Configurations to test:**

| Structure | Description | Clone cost | Push contention | File scan scope |
|-----------|-------------|-----------|----------------|----------------|
| **Multi-repo** | 1 repo per application | N separate clones (each small) | None (independent repos) | Small (only that app's files) |
| **Mono-repo, directory-per-app** | All apps in 1 repo, separated by directories (`apps/app-1/`, `apps/app-2/`, ...) | 1 large clone reused? No -- Calamari clones per source, so N clones of the same large repo | High -- N pushes to same repo/branch, triggering fetch+rebase retries | Scoped to source path, but clone is full repo |
| **Mono-repo, branch-per-env** | 1 repo with branches `dev`, `staging`, `prod` | N clones of same repo at different branches | Moderate -- contention only between apps on same branch | Full branch content |
| **Mono-repo, single branch** | All apps on same branch, same directory structure | N clones of same repo, same branch | Highest -- every push to same branch races with others | Full repo |

**How to set up:**

For **multi-repo**:
```bash
for i in $(seq 1 $N); do
  # Create separate repos, each with a simple Deployment manifest
  gh repo create "org/argocd-app-$i" --private
done
```

For **mono-repo, directory-per-app**:
```
argocd-manifests/
├── apps/
│   ├── app-1/
│   │   ├── deployment.yaml
│   │   └── values.yaml
│   ├── app-2/
│   │   ├── deployment.yaml
│   │   └── values.yaml
│   └── ... (up to app-N)
```

For **mono-repo, branch-per-env**: same repo with branches `dev`, `staging`, `prod`, each containing the app manifests.

### 3.4 Repository Size

Repo size directly affects clone time, which is the first operation in every deployment step. Calamari does a full clone (not shallow) per source.

**How to control:**

| Dimension | How to inflate | What it tests |
|-----------|---------------|--------------|
| **Working tree size** | Add large files (generated configs, CRDs, etc.) to the repo | Clone checkout time, disk I/O |
| **Git history depth** | Script N commits: `for i in $(seq 1 $N); do echo $i >> file; git commit -am "commit $i"; done` | Clone fetch time (object transfer), `.git` directory size |
| **Number of files** | Generate many small manifest files in the repo | File discovery/scanning time, `git add` staging time |
| **Branch count** | Create many branches (simulating branch-per-env at scale) | `git clone` ref negotiation time |

**Test levels:**

| Level | History | Files | Working tree | `.git` size |
|-------|---------|-------|-------------|-------------|
| Small | 100 commits | 50 files | ~500KB | ~5MB |
| Medium | 1,000 commits | 500 files | ~5MB | ~50MB |
| Large | 10,000 commits | 2,000 files | ~50MB | ~500MB |
| Extreme | 50,000 commits | 10,000 files | ~200MB | ~2GB |

**To generate a repo with controlled size:**
```bash
mkdir loadtest-repo && cd loadtest-repo && git init

# Generate N files
for i in $(seq 1 $FILE_COUNT); do
  mkdir -p "apps/app-$(( (i-1) / 5 ))"
  cat > "apps/app-$(( (i-1) / 5 ))/resource-$i.yaml" <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: deploy-$i
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: app
        image: nginx:1.0.0
EOF
done
git add -A && git commit -m "initial"

# Generate commit history
for i in $(seq 1 $COMMIT_COUNT); do
  echo "# change $i" >> "apps/app-0/resource-1.yaml"
  git commit -am "history commit $i"
done
```

### 3.5 Number of Container Images per Application

The image tag update step scans YAML files for image references matching the packages configured in the step. More images = more regex matching and file modifications.

**How to control:**
- Configure the deployment step with 1 vs. many container packages
- Create manifests with multiple containers per pod, multiple deployments per app
- Test levels: **1, 3, 5, 10** images per application source

### 3.6 Source Type (Directory / Helm / Kustomize)

Each source type has different file parsing and modification costs.

**How to control:**
- Create ArgoCD applications with different source types
- **Directory**: Raw YAML manifests -- regex scanning across all YAML files
- **Helm**: Parses `values.yaml` and updates specific YAML paths -- deeper YAML parsing
- **Kustomize**: Updates `kustomization.yaml` + optionally discovers and patches overlay files -- most file I/O

Test each source type independently, then in combination (multi-source app with mixed types).

### 3.7 Commit Method: Direct Push vs. Pull Request

Pull request mode adds Git provider API calls (create branch, create PR, set labels) on top of the Git push.

**How to control:**
- Set `Octopus.Action.ArgoCD.CommitMethod` to direct push vs. pull request
- Test with different Git providers (GitHub, GitLab, BitBucket, Azure DevOps) as PR API performance varies

### 3.8 Concurrent Deployments

Multiple Octopus deployments running simultaneously, all targeting the same or different repos.

**How to control:**
- Trigger N deployments simultaneously from Octopus (via API, scheduled triggers, or runbooks)
- Test levels: **1, 5, 10, 20, 50** concurrent deployments

**Critical interaction with repo structure:**
- Multi-repo: concurrent deployments to different repos don't contend
- Mono-repo: concurrent deployments all push to the same repo, causing push failures, fetch+rebase retries, and potential merge conflicts

### 3.9 Git Hosting Provider

Clone and push performance depends heavily on the Git hosting provider and its rate limits.

**Providers to test:** GitHub, GitLab, BitBucket, Azure DevOps (whichever are relevant to your users)

**Rate limits to be aware of:**
- GitHub: 5,000 API requests/hour (authenticated), clone/push limits vary
- GitLab: Configurable rate limits per group/project
- BitBucket: 1,000 API requests/hour (cloud)
- Azure DevOps: Generally more generous but has throughput limits

### 3.10 Network Latency to Git Provider

**How to control:**
- Test with Git provider in same region vs. cross-region
- If self-hosted (GitLab, BitBucket Server): deploy in same cluster vs. remote
- Measure clone time as a function of latency + repo size

---

## 4. Test Scenarios

### 4.1 Baseline: Single Application, Single Source

**Goal:** Establish baseline timing for the atomic unit of work.

| Variant | Apps | Sources | Repo size | Images | Commit method |
|---------|------|---------|-----------|--------|---------------|
| A | 1 | 1 | Small | 1 | Direct push |
| B | 1 | 1 | Small | 1 | Pull request |
| C | 1 | 1 | Medium | 1 | Direct push |
| D | 1 | 1 | Large | 1 | Direct push |
| E | 1 | 1 | Extreme | 1 | Direct push |

**Measure:** Total step duration, broken down into: clone time, file modification time, commit time, push time, PR creation time (if applicable).

**This establishes the cost curve for repo size and the overhead of PR creation.**

### 4.2 Source Type Comparison

**Goal:** Compare per-source cost across Directory, Helm, and Kustomize.

For each source type, run with 1 app, 1 source, medium repo, 5 images:

| Source type | Files to scan | Modification pattern |
|-------------|--------------|---------------------|
| Directory | All `.yaml` files in source path | Regex replace `image:` lines across 11 resource kinds |
| Helm | `values.yaml` + custom values files | YAML path-based updates |
| Kustomize | `kustomization.yaml` + patch files | YAML section updates + patch file discovery |

**Measure:** File scan time, modification time, number of files touched, commit size.

### 4.3 Scaling Applications

**Goal:** Understand cost of adding more applications to a single deployment.

| Variant | Apps | Sources/app | Repo structure | Repo size |
|---------|------|-------------|---------------|-----------|
| A | 5 | 1 | Multi-repo (separate repos) | Small |
| B | 10 | 1 | Multi-repo | Small |
| C | 25 | 1 | Multi-repo | Small |
| D | 50 | 1 | Multi-repo | Small |
| E | 5 | 1 | Mono-repo (dir-per-app) | Medium |
| F | 10 | 1 | Mono-repo (dir-per-app) | Medium |
| G | 25 | 1 | Mono-repo (dir-per-app) | Large |
| H | 50 | 1 | Mono-repo (dir-per-app) | Large |

**What to watch:**
- Multi-repo: total time should scale linearly (N independent clones)
- Mono-repo: each app source clones the **entire** repo separately, so clone cost = N x full repo clone. Plus push contention: the 2nd push may fail and need fetch+rebase, the 3rd even more so
- At what app count does mono-repo push contention become unacceptable?

### 4.4 Mono-repo Push Contention

**Goal:** Specifically measure push failure/retry rates in a mono-repo setup.

Setup: 1 mono-repo with 20 app directories, medium size.

| Variant | Concurrent sources pushing to same branch | Expected behavior |
|---------|------------------------------------------|-------------------|
| A | 1 (sequential) | No contention |
| B | 5 (from single deployment, processed sequentially by Calamari) | Some contention -- later pushes need rebase |
| C | 10 | More contention |
| D | 20 | Significant contention |
| E | 20 + another concurrent deployment to same repo | Contention from both within and across deployments |

**Measure:**
- Push success rate on first attempt
- Number of fetch+rebase retries per push
- Total deployment duration vs. sequential baseline
- Whether any pushes exhaust the 2-retry limit and fail permanently
- Merge conflicts: do rebases succeed or fail?

### 4.5 Concurrent Deployments

**Goal:** Test multiple Octopus deployments running in parallel.

| Variant | Concurrent deployments | Repo structure | Target |
|---------|----------------------|---------------|--------|
| A | 5 | Multi-repo (each deployment targets different apps/repos) | No contention |
| B | 5 | Mono-repo (all targeting same repo, different app dirs) | Push contention |
| C | 10 | Multi-repo | Octopus server resource contention |
| D | 10 | Mono-repo | Combined contention |
| E | 20 | Multi-repo | Octopus server limits |
| F | 20 | Mono-repo | Worst case |

**What to watch:**
- Octopus server CPU/memory under concurrent deployments (Calamari workers)
- Git provider rate limiting (especially for PR creation)
- Total wall-clock time: does it scale linearly with concurrency, or worse?
- Disk I/O: concurrent clones of large repos may saturate disk

### 4.6 Repository History Depth

**Goal:** Isolate the effect of Git history on clone performance.

Same repo structure (20 app directories, 100 files), varying history:

| Variant | Commits | Approximate `.git` size | Clone type |
|---------|---------|------------------------|-----------|
| A | 100 | ~5MB | Full clone |
| B | 1,000 | ~50MB | Full clone |
| C | 10,000 | ~500MB | Full clone |
| D | 50,000 | ~2GB | Full clone |

**Measure:** Clone time for each. This determines whether recommending shallow clones or Git protocol optimizations is necessary.

> **Note:** Calamari currently does full clones via LibGit2Sharp. If history depth proves to be a major bottleneck, this is a potential optimization area (shallow clone, treeless clone, or blobless clone).

### 4.7 Large Working Tree (Many Files)

**Goal:** Test file scanning performance with repos containing many manifest files.

| Variant | Files in repo | Files in source path | Source type |
|---------|--------------|---------------------|-------------|
| A | 100 | 20 | Directory |
| B | 500 | 100 | Directory |
| C | 2,000 | 500 | Directory |
| D | 10,000 | 2,000 | Directory |
| E | 500 | 100 | Kustomize (with patch discovery) |

**Measure:** File scanning time, regex matching time, `git add` staging time.

### 4.8 Pull Request Creation at Scale

**Goal:** Test PR creation throughput against Git provider APIs.

| Variant | PRs created | Provider | Concurrency |
|---------|------------|----------|-------------|
| A | 5 sequential | GitHub | 1 |
| B | 10 sequential | GitHub | 1 |
| C | 5 concurrent (from 5 parallel deployments) | GitHub | 5 |
| D | 10 concurrent | GitHub | 10 |
| E-H | Same as A-D | GitLab / BitBucket / Azure DevOps | Same |

**What to watch:**
- API rate limit headers in responses
- 429 (Too Many Requests) errors
- PR creation latency under load
- Whether Octopus retries on rate limit errors

### 4.9 Update Manifests Step (Full File Copy)

**Goal:** Test the manifest templating step which copies entire packages into Git repos.

| Variant | Package size | Files in package | Purge enabled | Repo size |
|---------|-------------|-----------------|---------------|-----------|
| A | 100KB (10 files) | 10 | No | Small |
| B | 1MB (50 files) | 50 | No | Medium |
| C | 10MB (200 files) | 200 | Yes | Medium |
| D | 50MB (1000 files) | 1000 | Yes | Large |

**Measure:** Package extraction time, file copy time, purge time, commit size, push time.

### 4.10 Sustained Deployment Cadence

**Goal:** Test realistic continuous deployment patterns over time.

| Pattern | Description | Duration |
|---------|-------------|----------|
| A | 1 deployment every 5 minutes, 10 apps, mono-repo | 2 hours |
| B | 1 deployment every 2 minutes, 10 apps, mono-repo | 2 hours |
| C | 1 deployment every 5 minutes, 50 apps, multi-repo | 2 hours |
| D | 3 concurrent deployments every 10 minutes, mono-repo | 2 hours |

**What to watch:**
- Does Git repo size grow meaningfully over the test (commit accumulation)?
- Does clone time increase as history grows during the test?
- Any resource leaks on the Octopus server (temp directories not cleaned up, etc.)?
- Git provider rate limit recovery patterns

---

## 5. Metrics to Collect

### 5.1 Per-Deployment Timing Breakdown

| Phase | What to measure | How |
|-------|----------------|-----|
| **Clone** | Time from start to repository available on disk | Octopus task log timestamps |
| **Checkout** | Time to switch to target branch | Task log |
| **File scan** | Time to discover and parse YAML files | Task log (debug mode) |
| **Modification** | Time to apply image tag replacements | Task log |
| **Commit** | Time to stage + commit | Task log |
| **Push** | Time for `git push` (including retries) | Task log |
| **PR creation** | Time for API call to Git provider | Task log |
| **Total step** | End-to-end deployment step duration | Octopus deployment duration |

### 5.2 Octopus Server Metrics

| Metric | Why |
|--------|-----|
| Worker CPU/memory | Calamari processes running git operations |
| Disk I/O | Clone operations write to disk |
| Disk space (temp dirs) | Each clone creates a temporary working directory |
| Concurrent task count | How many deployment tasks are running |
| Task queue depth | Deployments waiting for a worker |

### 5.3 Git Provider Metrics

| Metric | Why |
|--------|-----|
| API rate limit remaining | Approaching limit = PR creation failures |
| Clone throughput (MB/s) | Network/provider bottleneck |
| Push success/failure rate | Contention indicator |
| Push retry count | Mono-repo contention severity |
| PR creation latency | Provider API performance |

### 5.4 Repository Metrics

| Metric | Why |
|--------|-----|
| Repo size on disk (working tree) | Affects clone time and disk usage |
| `.git` directory size | Affects clone transfer size |
| Commit count growth | Accumulation over sustained tests |
| Branch count | Affects ref negotiation on clone |

---

## 6. Known Risks and What to Validate

| # | Risk | Trigger | Expected symptom | Scenario |
|---|------|---------|-----------------|----------|
| 1 | **Mono-repo push contention** -- multiple sources in same deployment all push to same repo/branch sequentially; later pushes need fetch+rebase | >5 app sources in same mono-repo, or concurrent deployments | Push retries, exponentially growing deployment time, potential permanent push failures after 2 retries | 4.4 |
| 2 | **Full clone cost at scale** -- Calamari does full `git clone` (not shallow) per source, even if multiple sources share the same repo | Large mono-repo with deep history | Clone time dominates deployment duration; disk I/O saturation with concurrent clones | 4.6 |
| 3 | **Git provider rate limiting** -- PR creation and push operations consume API quota | Many concurrent deployments with PR mode, especially on GitHub/BitBucket with lower limits | 429 errors, failed PR creation, partial deployments | 4.8 |
| 4 | **Disk space exhaustion** -- each source clone creates a full copy of the repo in a temp directory | Large repos x many sources x concurrent deployments | Worker node disk full, deployment failures | 4.5 with large repos |
| 5 | **Rebase merge conflicts** -- fetch+rebase during push retry may fail if concurrent pushes modify overlapping files | Multiple deployments updating same files in mono-repo (e.g., shared values.yaml) | Permanent push failure, deployment fails | 4.4E |
| 6 | **YAML regex fragility under load** -- image tag replacement uses regex on YAML; unusual formatting or large files may cause performance issues | Repos with many large YAML files, complex quoting, or generated manifests | Slow modification phase, incorrect replacements | 4.7 |
| 7 | **LibGit2Sharp memory usage** -- in-process Git operations may hold repo data in memory | Large repos, many concurrent operations | Worker OOM | 4.5 + 4.6 combined |

---

## 7. Test Environment Setup

### Git Repositories

Create dedicated test repositories (do not use production repos):

1. **Small multi-repo set**: 50 separate repos, each with 1 Deployment manifest + values.yaml
2. **Medium mono-repo**: 1 repo with 50 app directories, ~500 files, ~1000 commits
3. **Large mono-repo**: 1 repo with 50 app directories, ~2000 files, ~10000 commits
4. **Extreme mono-repo**: 1 repo with 100 app directories, ~10000 files, ~50000 commits

### ArgoCD Applications

For each repo structure, create matching ArgoCD Application resources:
- Annotated with Octopus scoping annotations
- Configured with correct source paths
- Mix of Directory, Helm, and Kustomize source types

### Octopus Deploy

- Dedicated test space
- Deployment steps configured for each test scenario
- Container image packages with version sequences for driving updates (e.g., `myapp:1.0.0` through `myapp:1.0.100`)
- Workers with monitoring enabled (CPU, memory, disk)
- API access for triggering concurrent deployments programmatically

### Monitoring

- Worker node metrics (CPU, memory, disk I/O, disk space)
- Octopus server task log extraction (for timing breakdown)
- Git provider API monitoring (rate limit headers, error rates)

---

## 8. Test Execution Checklist

### Preparation

- [ ] Test Git repositories created (multi-repo set, mono-repos at various sizes)
- [ ] ArgoCD Applications configured and scoped
- [ ] Octopus deployment steps configured for each scenario
- [ ] Container image packages with version sequences uploaded
- [ ] Worker monitoring in place
- [ ] Git provider rate limits documented for the test account
- [ ] Baseline clone times measured for each repo size (manual `git clone` + timing)
- [ ] Disk space available on workers: at least 10x the largest repo size

### Execution Order

1. **4.1** Baseline (atomic unit cost)
2. **4.2** Source type comparison
3. **4.6** Repository history depth (clone cost curve)
4. **4.7** Large working tree (file scanning cost)
5. **4.3** Scaling applications (mono vs. multi-repo)
6. **4.4** Mono-repo push contention (critical risk area)
7. **4.5** Concurrent deployments
8. **4.8** Pull request creation at scale
9. **4.9** Update manifests step
10. **4.10** Sustained cadence (run last, takes hours)

### Between Scenarios

- [ ] Clean up temporary directories on workers
- [ ] Reset test repos to known state (`git reset --hard; git push --force`)
- [ ] Check Git provider rate limit status
- [ ] Record all metrics before proceeding

---

## 9. Reporting Template

```
Scenario: [name]
Date: [date]

Configuration:
  Applications: [count]
  Sources per app: [count]
  Repo structure: [multi-repo / mono-repo (dir-per-app) / mono-repo (branch-per-env)]
  Repo size: [small/medium/large/extreme] ([X] files, [Y] commits, [Z]MB .git)
  Source type: [Directory / Helm / Kustomize / Mixed]
  Images per source: [count]
  Commit method: [direct push / pull request]
  Concurrent deployments: [count]

Timing (per deployment, averaged):
  Total step duration:     [seconds]
  Clone time (per source): [seconds]
  Modification time:       [seconds]
  Commit time:             [seconds]
  Push time:               [seconds]
  PR creation time:        [seconds] (if applicable)

Contention:
  Push retries:            [count] / [total pushes]
  Push failures:           [count]
  Rebase conflicts:        [count]

Resources:
  Worker peak CPU:         [value]
  Worker peak memory:      [value]
  Worker disk usage:       [value]
  Temp directory cleanup:  [yes/no]

Git Provider:
  API calls made:          [count]
  Rate limit remaining:    [value at end]
  429 errors:              [count]

Findings:
  [observations, bottlenecks, failures]

Recommendations:
  [repo structure guidance, resource sizing, configuration changes]
```

---

## 10. Expected Outcomes and Guidance

After completing all scenarios, the results should enable:

1. **Repo structure guidance**: Quantified cost of mono-repo vs. multi-repo at different scales, with concrete thresholds (e.g., "mono-repo push contention becomes significant above N apps")

2. **Repo size guidance**: Maximum practical repo size before clone time dominates deployment duration, and whether shallow clone support should be prioritized

3. **Concurrency limits**: Maximum safe concurrent deployments per repo structure, accounting for push contention and Git provider rate limits

4. **Resource sizing**: Worker CPU/memory/disk recommendations based on observed consumption

5. **Provider-specific guidance**: Which Git providers handle load best, and at what thresholds rate limiting kicks in

6. **Architecture recommendations**: When to split a mono-repo, when to use PR mode vs. direct push, and how to structure repos for best deployment performance
