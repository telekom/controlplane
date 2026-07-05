# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
VERSION=$(shell git describe --tags --always --dirty || echo 'develop')
GIT_COMMIT=$(shell git rev-parse HEAD)

.PHONY: all
all: build

.PHONY: generate
generate: ## Run code generation (oapi-codegen + mockery).
	go generate -tags=tools ./tools/...
	@head -3 internal/api/server.gen.go | grep -q 'SPDX' || { { cat ../hack/boilerplate.go.txt; echo; cat internal/api/server.gen.go; } > internal/api/server.gen.go.tmp && mv internal/api/server.gen.go.tmp internal/api/server.gen.go; }

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

##@ Build

.PHONY: build
build: fmt vet ## Run go build against code.
	go build -ldflags="-X 'main.version=$(VERSION)' -X 'main.buildTime=$(BUILD_TIME)' -X 'main.gitCommit=$(GIT_COMMIT)' -X 'main.goVersion=$(shell go version)'" -trimpath -mod=readonly -o bin/rover-server cmd/main.go

# Packages to test and measure coverage for
TEST_PACKAGES := $(shell go list ./internal/... ./pkg/... | grep -v '^./internal/api$$')
COVER_PACKAGES := $(shell go list ./internal/... ./pkg/... | grep -v '^./internal/api$$' | paste -sd, -)

.PHONY: test
test: fmt vet ## Run tests.
	gotestsum --format pkgname --junitfile junit.xml --jsonfile gotest.log -- $(TEST_PACKAGES) -coverprofile cover.out --coverpkg $(COVER_PACKAGES) -mod=readonly -race
