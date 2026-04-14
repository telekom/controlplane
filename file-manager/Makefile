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
	gotestsum --format pkgname --junitfile junit.xml --jsonfile gotest.log -- $$(go list ./... | grep -v /test |grep -vE "/internal/api|gen|fake|cmd") -coverprofile cover.out -mod=readonly -race