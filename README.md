# octopus-argocd-gateway
gRPC gateway to allow for Octopus Deploy to communicate with an Argo CD instance within a Kubernetes Cluster

# Usage 

## Installation
To install the Gateway you can use the provided Helm chart.

```bash
helm upgrade --atomic --install \ 
--create-namespace --namespace my-gateway \
--set registration.name="my-gateway" \
--set registration.serverApiUrl="http://localhost:8066" \
--set registration.serverAccessToken="{YourOctopusAuthenticationTokenHere}" \
--set registration.environments="{development,staging,production}" \
--set registration.spaceId="Spaces-1" \
--set gateway.argocd.serverGrpcUrl="localhost:30857" \
--set gateway.octopus.serverGrpcUrl="localhost:8443" \
--set gateway.octopus.serverThumbprint="{YourServerThumbprintHere}" \
--set gateway.argocd.authenticationToken="{YourArgoCDAuthenticationTokenHere}" \
my-gateway \
oci://registry-1.docker.io/octopusdeploy/octopus-argocd-gateway-chart
```

# Interface Design

## Consumer-Defined Interfaces

In this codebase, interfaces are defined by consumers rather than producers. This follows the Go proverb: "Accept interfaces, return structs."

**Why consumers define interfaces:**
- **Minimal dependencies**: Consumers only depend on the methods they actually use, not the entire producer API. If a consumer only needs `GetApplication()`, it doesn't need to mock or depend on `ListApplications()`, `DeleteApplication()`, etc.
- **Decoupled testing**: Each package defines exactly what it needs, making mocks focused and tests isolated. The `applicationresources` package defines its own `applicationService` interface with just the 3 methods it uses.
- **Evolution without breakage**: Producers can add new methods without breaking consumers. If `argocd.ApplicationService` adds a new method, existing consumer interfaces remain valid.
- **Clear contracts**: Reading a consumer's interface immediately shows what external capabilities it requires, serving as documentation of its dependencies.

**Example:**
```go
// Consumer defines only what it needs
type applicationService interface {
    GetApplication(project, name, namespace string) (*v1alpha1.Application, error)
    WatchApplicationResourceTree(ctx context.Context, ...) (Client, error)
}

// Producer returns concrete struct with many more methods
func NewApplicationService(...) ApplicationService { ... }
```

# Simulator
A load-testing CLI at `cmd/simulator` that emulates virtual gateways against an Octopus Server.

```sh
make simulator
./bin/simulator run --scenario ingest --credentials creds.json --target-rate 500 --duration 2m
```

Scenarios: `sync-latency`, `ingest`, `saturation`. Run `./bin/simulator run --help` for flags. The credentials JSON must be `chmod 600`; see `internal/simulator/credentials/credentials.go` for the schema.

# Protobuf Guidelines
- All Protobuf definitions should be placed in the `internal/protos/definitions` directory.
- Updates to existing definitions should be backward compatible.
- Follow the [1-1-1 rule](https://protobuf.dev/best-practices/1-1-1/): All proto definitions should have one top-level element and build target per file.
- Language specific proto properties shouldn't be added directly in the Proto file but rather in the [buf.gen.yaml](buf.gen.yaml) configuration under the key `managed.override`

# Prerequisites
- [Buf CLI](https://buf.build/docs/cli/installation/)

Optionally
- [Buf account](https://buf.build/) - To avoid rate limiting issues when generating SDKs

# How To
## Add a new Protobuf contract
1. Create new `*.proto` file(s) within the `protos` directory - Depending on what you are adding you might need multiple proto files to follow the 1-1-1 rule
2. Run `buf format -w` or `make proto-format` in the root directory of the repository to automatically format the proto files
3. Run `buf lint` or `make proto-lint` to ensure that the proto files meet the linting requirements. (If you don't run this locally it will run as a PR required check)
4. Run `buf generate` or `make proto-generate to generate the SDKs
5. Ensure that the changes are duplicated in Octopus Deploy's codebase.

## Update an existing Protobuf contract
1. Make changes to your desired Proto file, ensure the changes are backward compatible see: [Protobuf best practices](https://protobuf.dev/best-practices/dos-donts/)
2. Run `buf format -w` or `make proto-format` in the root directory of the repository to automatically format the proto files
3. Run `buf lint` or `make proto-lint` to ensure that the proto files meet the linting requirements. (If you don't run this locally it will run as a PR required check)
4. Run `buf breaking --against .git` or `make proto-breaking` to detect any breaking changes(If you don't run this locally it will run as a PR required check)
5. Run `buf generate` or `make proto-generate` to generate the SDKs
6. Ensure that the changes are duplicated in Octopus Deploy's codebase.

## Breaking Changes
Breaking changes should be avoided as much as possible but if they are necessary. If you have to make a breaking change ideally it would be done by introducing a new version of the contract whilst still keeping old version around but if that isn't possible
you can make the breaking change by doing the following:
1. Make the necessary changes to your Proto file(s)
2. Create a PR and add the label `buf skip breaking` to the PR to skip the breaking change check

## Linting
Buf is configured to use the [STANDARD](https://buf.build/docs/lint/rules/#standard) linting rules with some exceptions which can be seen in the [buf.yaml](buf.yaml) file.

## Release Please Troubleshooting

### Forgot conventional commit message when merging PR into main or a non-releasable conventional commit message was used(e.g. chore, docs, etc.)
If you forgot to use a conventional commit message when merging a PR into main, you can do the following to get Release Please to pick up the changes:

#### Option 1: Leave it
If you leave it, Release Please will pick up the changes when the next PR with a conventional commit message is merged into main.

#### Option 2: Add `release-as` to the `release-please.config.json` file 
You can add a `release-as` field to the `release-please.config.json` file in the root of the repository. This will force Release Please to create a new release with the specified version.
However you should ensure that you remove the `release-as` field after the release is created to avoid creating a new release with the same version in the future.

```json
{
  ...
  "packages": {
    ".": {
      "release-as": "0.4.0", <--- Add this with your desired next version
      ...Rest of the config
    },
    "charts/octopus-argocd-gateway": {
      "release-as": "0.9.0", <--- Add this with your desired next version
      ...Rest of the config
    }
  }
}
```

### Not sure what Release Please is going to do
You can dry-run the Release Please cli locally to see what commits and release PRs that Release Please will create.
```bash 
# Get GH PAT using GH cli
gh auth token

# Dry-run the Release Please against your target branch
release-please release-pr \
  --token={{GITHUB_PAT}} \
  --repo-url=octopusdeploy/octopus-argocd-gateway \
  --target-branch=tl/fix-versions \
  --dry-run
```
Example output:
```bash 
❯ Fetching release-please-config.json from branch tl/fix-versions
❯ Fetching .release-please-manifest.json from branch tl/fix-versions
...Rest of the output

Would open 2 pull requests
fork: false
title: feat(tl/fix-versions): release 0.4.0
branch: release-please--branches--tl/fix-versions
draft: false
body: :robot: I have created a release *beep* *boop*
---
{{CHANGELOG HERE}}
---
This PR was generated with [Release Please](https://github.com/googleapis/release-please). See [documentation](https://github.com/googleapis/release-please#release-please).
updates: 3
❯ Fetching charts/octopus-argocd-gateway/CHANGELOG.md from branch tl/fix-versions
❯ Fetching charts/octopus-argocd-gateway/Chart.yaml from branch tl/fix-versions
✔ updating from 0.9.0 to 0.9.0
❯ Fetching .release-please-manifest.json from branch tl/fix-versions
  charts/octopus-argocd-gateway/CHANGELOG.md:  [class Changelog extends DefaultUpdater]
  charts/octopus-argocd-gateway/Chart.yaml:  [class ChartYaml extends DefaultUpdater]
  .release-please-manifest.json:  [class ReleasePleaseManifest extends DefaultUpdater]
```
