# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Image URL to use all building/pushing image targets
IMG ?= ghcr.io/telekom/controlplane/migrator:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN))
endif

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: fmt vet ## Run tests.
	go test ./... -v -coverprofile cover.out 2>&1 | tee gotest.log

.PHONY: test-verbose
test-verbose: fmt vet ## Run tests with detailed Ginkgo output.
	go test ./... -v -ginkgo.v -ginkgo.progress -coverprofile cover.out 2>&1 | tee gotest.log

.PHONY: test-focus
test-focus: ## Run tests matching FOCUS (e.g., make test-focus FOCUS="Reconcile").
	go test ./... -v -ginkgo.focus="$(FOCUS)" 2>&1 | tee gotest.log

##@ Build

.PHONY: build
build: fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: fmt vet ## Run from your host.
	go run cmd/main.go

.PHONY: docker-build
docker-build: ## Build docker image.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image.
	docker push ${IMG}

##@ Deployment

.PHONY: deploy
deploy: ## Deploy controller to the K8s cluster.
	cd config/manager && kustomize edit set image controller=${IMG}
	kubectl apply -k config/default

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster.
	kubectl delete -k config/default

##@ Dependencies

.PHONY: tidy
tidy: ## Run go mod tidy.
	go mod tidy

.PHONY: vendor
vendor: ## Run go mod vendor.
	go mod vendor
