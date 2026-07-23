# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.32.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif


# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role,headerFile="../hack/boilerplate.yaml.txt" crd:headerFile="../hack/boilerplate.yaml.txt" paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen generate-xds-api ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="../hack/boilerplate.go.txt" paths="./..."

.PHONY: generate-xds-api
generate-xds-api: buf protoc-gen-go protoc-gen-go-grpc addlicense ## Generate the internal xDS publication API.
	cd internal/xds/api && PATH="$(LOCALBIN):$$PATH" $(BUF) generate
	$(ADDLICENSE) -c "Deutsche Telekom AG" -y 2026 internal/xds/api/v1/*.pb.go

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

# Packages to test and measure coverage for
TEST_PACKAGES = ./internal/... ./pkg/kong/client/...
# Coverage scope: exclude generated mocks (internal/features/mock, pkg/kong/client/mock)
COVER_PACKAGES = ./internal/controller/...,./internal/features,./internal/features/feature,./internal/features/feature/config,./internal/handler/...,./pkg/kong/client,./pkg/kong/client/plugin

.PHONY: test
test: manifests generate fmt vet envtest gotestsum ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GOTESTSUM) --format pkgname --jsonfile gotest.log -- $(TEST_PACKAGES) -ginkgo.junit-report=ginkgo-junit.xml -coverprofile cover.out --coverpkg $(COVER_PACKAGES) -race

.PHONY: test-xds
test-xds: generate-xds-api gotestsum ## Run focused xDS management tests.
	go fmt ./internal/xds/... ./cmd/xds-management-server/...
	go vet ./internal/xds/... ./cmd/xds-management-server/...
	$(GOTESTSUM) --format pkgname -- ./internal/xds/... ./cmd/xds-management-server/... -race

.PHONY: test-xdsdemo
test-xdsdemo: gotestsum ## Run focused durable xDS demo tests.
	go fmt ./cmd/xdsdemo/...
	go vet ./cmd/xdsdemo/...
	$(GOTESTSUM) --format pkgname -- ./cmd/xdsdemo/... -race

.PHONY: tidy
tidy: ## Update module metadata.
	go mod tidy

# Utilize Kind or modify the e2e tests to load the image locally, enabling compatibility with other vendors.
.PHONY: test-e2e  # Run the e2e tests against a Kind k8s instance that is spun up.
test-e2e:
	go test ./test/e2e/ -v -ginkgo.v

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-xds
lint-xds: golangci-lint ## Run golangci-lint for the xDS management implementation.
	$(GOLANGCI_LINT) run ./internal/xds/... ./cmd/xds-management-server/...

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: build-xds-management-server
build-xds-management-server: generate-xds-api ## Build the xDS management server.
	go fmt ./internal/xds/... ./cmd/xds-management-server/...
	go vet ./internal/xds/... ./cmd/xds-management-server/...
	CGO_ENABLED=0 go build -o bin/xds-management-server ./cmd/xds-management-server

.PHONY: build-xdsdemo
build-xdsdemo: ## Build the envtest-backed durable xDS demo command.
	go fmt ./cmd/xdsdemo/...
	go vet ./cmd/xdsdemo/...
	CGO_ENABLED=0 go build -o bin/xdsdemo ./cmd/xdsdemo

.PHONY: xdsdemo-compose-config
xdsdemo-compose-config: ## Validate the durable xDS demo Compose model.
	docker compose -f cmd/xdsdemo/docker-compose.yaml config --quiet

.PHONY: xdsdemo-up
xdsdemo-up: ## Build and start the durable xDS demo.
	docker compose -f cmd/xdsdemo/docker-compose.yaml up -d --build

.PHONY: xdsdemo-validate
xdsdemo-validate: ## Run the durable xDS demo validation pipeline.
	bash cmd/xdsdemo/scripts/validate.sh

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go



.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
BUF ?= $(LOCALBIN)/buf
PROTOC_GEN_GO ?= $(LOCALBIN)/protoc-gen-go
PROTOC_GEN_GO_GRPC ?= $(LOCALBIN)/protoc-gen-go-grpc
ADDLICENSE ?= $(LOCALBIN)/addlicense
GOTESTSUM ?= $(LOCALBIN)/gotestsum

## Tool Versions
KUSTOMIZE_VERSION ?= v5.4.3
CONTROLLER_TOOLS_VERSION ?= v0.20.1
ENVTEST_VERSION ?= release-0.19
GOLANGCI_LINT_VERSION ?= v2.11.2
BUF_VERSION ?= v1.50.1
PROTOC_GEN_GO_VERSION ?= v1.36.11
PROTOC_GEN_GO_GRPC_VERSION ?= v1.6.1
ADDLICENSE_VERSION ?= v1.1.1
GOTESTSUM_VERSION ?= v1.12.3

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: buf
buf: $(BUF)
$(BUF): $(LOCALBIN)
	$(call go-install-tool,$(BUF),github.com/bufbuild/buf/cmd/buf,$(BUF_VERSION))

.PHONY: protoc-gen-go
protoc-gen-go: $(PROTOC_GEN_GO)
$(PROTOC_GEN_GO): $(LOCALBIN)
	$(call go-install-tool,$(PROTOC_GEN_GO),google.golang.org/protobuf/cmd/protoc-gen-go,$(PROTOC_GEN_GO_VERSION))

.PHONY: protoc-gen-go-grpc
protoc-gen-go-grpc: $(PROTOC_GEN_GO_GRPC)
$(PROTOC_GEN_GO_GRPC): $(LOCALBIN)
	$(call go-install-tool,$(PROTOC_GEN_GO_GRPC),google.golang.org/grpc/cmd/protoc-gen-go-grpc,$(PROTOC_GEN_GO_GRPC_VERSION))

.PHONY: addlicense
addlicense: $(ADDLICENSE)
$(ADDLICENSE): $(LOCALBIN)
	$(call go-install-tool,$(ADDLICENSE),github.com/google/addlicense,$(ADDLICENSE_VERSION))

.PHONY: gotestsum
gotestsum: $(GOTESTSUM)
$(GOTESTSUM): $(LOCALBIN)
	$(call go-install-tool,$(GOTESTSUM),gotest.tools/gotestsum,$(GOTESTSUM_VERSION))

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
