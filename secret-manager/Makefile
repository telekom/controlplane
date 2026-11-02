# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

ENVTEST_K8S_VERSION ?= 1.32.0
ENVTEST_VERSION ?= release-0.19
LOCALBIN ?= $(shell pwd)/bin
ENVTEST ?= $(LOCALBIN)/setup-envtest

.PHONY: all
all: build

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

##@ Build

.PHONY: build
build: fmt vet ## Run go build against code.
	go build -o bin/server cmd/server/server.go

# Packages to test and measure coverage for
TEST_PACKAGES = $(shell go list ./internal/... ./pkg/... | grep -v '^./internal/api$$')
COVER_PACKAGES = $(shell go list ./internal/... ./pkg/... | grep -v '^./internal/api$$' | paste -sd, -)

.PHONY: test
test: fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$$( $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" && \
	test -n "$$KUBEBUILDER_ASSETS" && \
	export KUBEBUILDER_ASSETS && \
	gotestsum --format pkgname --junitfile junit.xml --jsonfile gotest.log -- $(TEST_PACKAGES) -coverprofile cover.out --coverpkg $(COVER_PACKAGES) -mod=readonly -race

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.

$(ENVTEST): | $(LOCALBIN)
	GOBIN="$(LOCALBIN)" go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION)
