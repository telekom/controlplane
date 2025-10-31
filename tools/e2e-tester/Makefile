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

##@ Build

.PHONY: build
build: fmt vet ## Run go build against code.
	mkdir -p bin
	go build -ldflags="-X 'main.version=$(VERSION)' -X 'main.buildTime=$(BUILD_TIME)' -X 'main.gitCommit=$(GIT_COMMIT)' -X 'main.goVersion=$(shell go version)'" -trimpath -mod=readonly -o bin/e2e-tester main.go

.PHONY: test
test: fmt vet ## Run tests.
	go test $$(go list ./... | grep -v /test) -coverprofile cover.out -mod=readonly -race -json 2>&1 | tee gotest.log | gotestfmt

.PHONY: install
install: build 
	install -D -m 0755 bin/e2e-tester $(DESTDIR)/usr/local/bin/e2e-tester