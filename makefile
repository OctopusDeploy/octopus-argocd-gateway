tidy:
	@echo Tidy go.mod dependencies
	@go mod tidy

simulator: $(LOCALBIN)
	@echo Building simulator
	@go build -o $(LOCALBIN)/simulator ./cmd/simulator

download: tidy
	@echo Download go.mod dependencies
	@go mod download

lint: golangci-lint
	@echo Linting and formatting codebase
	$(GOLANGCI_LINT) run

format: golangci-lint
	@echo Linting and formatting codebase
	$(GOLANGCI_LINT) run --fix

# Mock generation
mocks:
	@echo Generating mock builders
	@go generate ./...

proto-clean-generated:  download
	@echo Cleaning generated protos
	@rimraf --glob internal/protos/*.pb.go &

proto-generate: proto-clean-generated
	@echo Compiling protos
	@buf generate

proto-breaking:
	@echo Checking for breaking changes
	@buf breaking --against .git

proto-lint:
	@echo Linting protos
	@buf lint

proto-format:
	@echo Formatting protos
	@buf format -w

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
GOLANGCI_LINT_VERSION ?= v2.12.2

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
