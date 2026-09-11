# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
VERSION=$(shell git describe --tags --always --dirty || echo 'develop')
GIT_COMMIT=$(shell git rev-parse HEAD)

.PHONY: all
all: build

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: generate
generate: ## Generate JSON schemas.
	go generate ./tools/...

##@ Build

.PHONY: build
build: generate fmt vet ## Run go build against code.
	mkdir -p bin
	go build -ldflags="-X 'main.version=$(VERSION)' -X 'main.buildTime=$(BUILD_TIME)' -X 'main.gitCommit=$(GIT_COMMIT)' -X 'main.goVersion=$(shell go version)'" -trimpath -mod=readonly -o bin/e2e-tester main.go

# Packages to test and measure coverage for
TEST_PACKAGES = ./pkg/...
COVER_PACKAGES = ./pkg/...

.PHONY: test
test: generate fmt vet ## Run tests.
	gotestsum --format pkgname --junitfile junit.xml --jsonfile gotest.log -- $(TEST_PACKAGES) -coverprofile cover.out --coverpkg $(COVER_PACKAGES) -mod=readonly -race

.PHONY: install
install: build 
	install -D -m 0755 bin/e2e-tester $(DESTDIR)/usr/local/bin/e2e-tester