// Self-hosted Renovate configuration for octopus-argocd-gateway.
//
// This is run by the "Renovate update dependencies" GitHub Actions workflow
// (.github/workflows/update-dependencies.yml) using renovatebot/github-action.
//
// It manages five things in this repo, separated into clearly-labelled
// packageRules sections below:
//   1. Go modules         (go.mod / go.sum)                      -> manager "gomod"
//   2. Docker base images (Dockerfile)                           -> manager "dockerfile"
//   3. GitHub Actions     (.github/workflows/*)                  -> manager "github-actions"
//   4. npm packages       (charts/.../package.json)              -> manager "npm"
//   5. Helm chart deps    (charts/.../Chart.yaml dependencies)   -> manager "helmv3"
//
// team-yosemite and team-sierra own the dependency upkeep for this repo (see
// the reviewers list below), so a single config with one dashboard and one
// branch prefix is simpler to maintain than separate per-area configs. Anything
// that needs to differ per area is expressed as a packageRule.

const preCannedPrNotes = {
  greenMeansGo: [
    "Green means go. Any issues in this PR should be caught as part of our tests and/or builds.",
  ],
  reviewBreakingChanges: [
    "Please review breaking changes and check whether we are impacted based on our usages.",
  ],
};

module.exports = {
  timezone: "Australia/Brisbane",
  requireConfig: "optional",
  onboarding: false,

  platform: "github",
  repositories: ["OctopusDeploy/octopus-argocd-gateway"],
  reviewers: ["team:team-yosemite", "team:team-sierra"],
  labels: ["dependencies"],
  branchPrefix: "renovate/",

  // Commit as our own bot account instead of Renovate's Mend-owned default.
  gitAuthor: "team-yosemite-bot <teamyosemitebot@octopus.com>",

  enabledManagers: ["gomod", "dockerfile", "github-actions", "npm", "helmv3"],

  // Run `go mod tidy` after a gomod update so go.sum and the indirect
  // requirements in go.mod are reconciled within the same PR, instead of
  // leaving the bumped go.mod inconsistent. No-op for the dockerfile and
  // github-actions managers.
  // https://docs.renovatebot.com/configuration-options/#postupdateoptions
  postUpdateOptions: ["gomodTidy"],

  // Renovate will create a new issue in the repository with a "dashboard" that
  // gives an overview of the status of all updates.
  // https://docs.renovatebot.com/key-concepts/dashboard/
  dependencyDashboard: true,
  dependencyDashboardTitle: "Dependency Dashboard",

  // Limit the amount of PRs created at once.
  prConcurrentLimit: 10,
  prHourlyLimit: 5,

  // This repo uses Conventional Commits (release-please generates the changelog
  // from them), so keep Renovate's commit messages semantic e.g.
  // "chore(deps): update ...".
  // https://docs.renovatebot.com/semantic-commits/
  semanticCommits: "enabled",

  // Security (vulnerability) updates: emit `fix(deps): ...` instead of the
  // default `chore(deps): ...`. release-please hides `chore` from the changelog
  // but surfaces `fix` under "Bug Fixes" (and it drives a patch release), so
  // this ensures security bumps are recorded in the release notes. These
  // options apply only to vulnerability-remediation branches (the
  // `renovate/*-vulnerability` branches with `[security]` in the PR title);
  // routine dependency updates stay `chore(deps): ...` and remain hidden.
  // https://docs.renovatebot.com/configuration-options/#vulnerabilityalerts
  vulnerabilityAlerts: {
    semanticCommitType: "fix",
    // `addLabels` appends to the base `labels: ["dependencies"]`, so security
    // PRs are tagged both "dependencies" and "security".
    addLabels: ["security"],
  },

  // Per-package rules.
  // Renovate evaluates all packageRules and does not stop at the first match,
  // so order them in ascending order of importance: later rules override
  // earlier ones.
  // https://docs.renovatebot.com/configuration-options/#packagerules
  packageRules: [
    // ************************************************************************
    // GO MODULES (manager: gomod)
    // ************************************************************************
    {
      // The `go` directive / toolchain bump in go.mod. Moving Go major/minor
      // versions can have real behavioural impact and is worth a deliberate
      // look, so don't lump it in with library updates.
      matchManagers: ["gomod"],
      matchDepNames: ["go"],
      addLabels: ["go"],
      prBodyNotes: [
        ...preCannedPrNotes.reviewBreakingChanges,
        "This bumps the Go version in go.mod. Confirm the Dockerfile build image (golang:x.y-alpine) and CI Go version are aligned.",
      ],
    },
    {
      // golang.org/x/* repos move together; grouping reduces PR noise.
      groupName: "golang.org/x packages",
      matchManagers: ["gomod"],
      matchPackageNames: ["golang.org/x/**"],
      addLabels: ["go"],
      prBodyNotes: [...preCannedPrNotes.greenMeansGo],
    },
    {
      // OpenTelemetry Go libraries are versioned and released together; a
      // partial update tends to break compilation, so update them as a group.
      groupName: "opentelemetry-go",
      matchManagers: ["gomod"],
      matchSourceUrls: [
        "https://github.com/open-telemetry/opentelemetry-go",
        "https://github.com/open-telemetry/opentelemetry-go-contrib",
      ],
      addLabels: ["go"],
      prBodyNotes: [...preCannedPrNotes.greenMeansGo],
    },
    {
      // k8s.io/* (client-go, api, apimachinery, ...) share a release train and
      // must move in lockstep.
      groupName: "kubernetes packages",
      matchManagers: ["gomod"],
      matchPackageNames: ["k8s.io/**", "sigs.k8s.io/**"],
      addLabels: ["go"],
      prBodyNotes: [
        ...preCannedPrNotes.reviewBreakingChanges,
        "Kubernetes client libraries occasionally introduce breaking API changes between minor versions.",
      ],
    },
    {
      // spf13's CLI/config libraries (cobra, pflag, viper) are versioned
      // independently but routinely bumped together and are low-risk; group
      // them into one PR to cut review noise.
      groupName: "spf13 packages",
      matchManagers: ["gomod"],
      matchPackageNames: ["github.com/spf13/**"],
      addLabels: ["go"],
      prBodyNotes: [...preCannedPrNotes.greenMeansGo],
    },
    {
      // Catch-all label for any remaining Go module update.
      matchManagers: ["gomod"],
      addLabels: ["go"],
      prBodyNotes: [...preCannedPrNotes.greenMeansGo],
    },

    // ************************************************************************
    // DOCKER (manager: dockerfile)
    // ************************************************************************
    {
      // The golang build image. Pin to major.minor (e.g. 1.24) and keep it in
      // step with the `go` directive in go.mod rather than chasing every patch.
      matchManagers: ["dockerfile"],
      matchDepNames: ["golang"],
      // Match a major.minor docker tag like "1.24-alpine"; ignore patch bumps.
      versioning:
        "regex:^(?<major>\\d+)\\.(?<minor>\\d+)(-(?<compatibility>.*))?$",
      addLabels: ["docker"],
      prBodyNotes: [
        "Keep the build image Go version aligned with the `go` directive in go.mod.",
      ],
    },
    {
      // gcr.io/distroless/static is published as rolling tags with no semver, so
      // pin and update by digest to get reproducible, auditable builds.
      // https://docs.renovatebot.com/docker/#digest-pinning
      matchManagers: ["dockerfile"],
      matchDepNames: ["gcr.io/distroless/static"],
      addLabels: ["docker"],
      pinDigests: true,
      prBodyNotes: [
        ...preCannedPrNotes.greenMeansGo,
        "distroless/static updates by digest. A green build is sufficient verification.",
      ],
    },
    {
      // Catch-all label for any remaining Dockerfile image update.
      matchManagers: ["dockerfile"],
      addLabels: ["docker"],
      prBodyNotes: [...preCannedPrNotes.greenMeansGo],
    },

    // ************************************************************************
    // GITHUB ACTIONS (manager: github-actions)
    // ************************************************************************
    {
      // For security, pin third-party actions to an immutable commit SHA rather
      // than a mutable tag. Renovate keeps the "# vX.Y.Z" comment updated.
      // https://docs.renovatebot.com/modules/manager/github-actions/#digest-pinning-and-updating
      matchManagers: ["github-actions"],
      // Only pin real `uses:` action references (depType "action"). The
      // github-actions manager also extracts tool versions from `with:` inputs
      // (e.g. azure/setup-helm's `version:` becomes dep "helm", depType
      // "uses-with", datasource github-releases). Trying to pin a digest onto a
      // plain version string can't find a target and fails the whole grouped
      // pin-dependencies branch, so restrict pinning to actual actions.
      matchDepTypes: ["action"],
      matchPackageNames: [
        "!actions/**", // Trust official GitHub actions (actions/checkout, etc.); they change often and pinning is noisy.
      ],
      addLabels: ["github-actions"],
      pinDigests: true,
      // Track the full vX.Y.Z release tag so the pinned SHA gets a `# v4.2.1`
      // comment instead of the bare `# v4` from a `@v4` reference. Copied from
      // helpers:pinGitHubActionDigestsToSemver rather than extended, since that
      // preset also pins official actions/** (which we exclude above).
      // https://docs.renovatebot.com/presets-helpers/#helperspingithubactiondigeststosemver
      extractVersion: "^(?<version>v?\\d+\\.\\d+\\.\\d+)$",
      versioning:
        "regex:^v?(?<major>\\d+)(\\.(?<minor>\\d+)\\.(?<patch>\\d+))?$",
    },
    {
      // Label official GitHub actions too, but don't pin them (see above).
      matchManagers: ["github-actions"],
      matchPackageNames: ["actions/**"],
      addLabels: ["github-actions"],
    },
    {
      // Batch all `uses:` action updates (pinned third-party + official) into a
      // single PR to cut review noise; they're independent and bump frequently.
      // Restricted to depType "action" so `uses-with` tool versions (e.g. the
      // helm version from azure/setup-helm) stay in their own PRs.
      matchManagers: ["github-actions"],
      matchDepTypes: ["action"],
      groupName: "github-actions",
    },

    // ************************************************************************
    // NPM (manager: npm)
    // ************************************************************************
    {
      // The only package.json is the Helm chart's dev tooling
      // (charts/octopus-argocd-gateway/package.json) — currently just cross-env,
      // used to run the helm-unittest / helm-docs Docker images cross-platform.
      // These are low-risk dev dependencies, so group them into a single PR.
      // (Renovate ignores the committed node_modules/ via its default
      // ignorePaths, so only the top-level chart package.json is managed.)
      groupName: "npm packages",
      matchManagers: ["npm"],
      addLabels: ["npm"],
      prBodyNotes: [...preCannedPrNotes.greenMeansGo],
    },

    // ************************************************************************
    // HELM (manager: helmv3)
    // ************************************************************************
    {
      // Subchart dependencies declared in the `dependencies:` block of
      // charts/octopus-argocd-gateway/Chart.yaml. There are none today, so this
      // rule only takes effect once a subchart is added; it's wired up now so
      // future dependencies are covered. We intentionally do NOT enable the
      // helm-values manager: the chart's image.tag defaults to its own
      // appVersion (managed by release-please), so Renovate must not touch it.
      groupName: "helm chart dependencies",
      matchManagers: ["helmv3"],
      addLabels: ["helm"],
      prBodyNotes: [...preCannedPrNotes.greenMeansGo],
    },

    // ************************************************************************
    // BASE RULE (applies to everything)
    // ************************************************************************
    {
      matchPackageNames: ["*"],
      // Give third-party releases a couple of days to surface issues before we
      // pick them up, without waiting too long.
      minimumReleaseAge: "2 days",
    },

    // ************************************************************************
    // OVERRIDES (applied last, so they win over the base rule above)
    // ************************************************************************
    {
      // distroless/static is a digest-pinned rolling tag whose digest is rebuilt
      // more than daily, so there is never a digest older than the 2-day base
      // minimumReleaseAge to fall back to. With internalChecksFilter=strict
      // (Renovate's default) that means it creates the pin/digest PR anyway with
      // a permanently pending renovate/stability-days check (cf. PR #230). A
      // release-age soak is meaningless for a constantly-moving digest, so
      // disable it for this image. The soak still applies to GitHub Action SHA
      // pins, which track real, dated releases.
      matchManagers: ["dockerfile"],
      matchDepNames: ["gcr.io/distroless/static"],
      minimumReleaseAge: null,
    },
  ],
};
