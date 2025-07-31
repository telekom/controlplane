# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

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

.PHONY: test
test: fmt vet ## Run tests.
	go test $$(go list ./... | grep -v /test) -coverprofile cover.out -mod=readonly -race -json 2>&1 | tee gotest.log | gotestfmt
