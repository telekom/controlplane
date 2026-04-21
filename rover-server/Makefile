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

# Packages to test and measure coverage for
TEST_PACKAGES := $(shell go list ./internal/... ./pkg/... | grep -v '^./internal/api$$')
COVER_PACKAGES := $(shell go list ./internal/... ./pkg/... | grep -v '^./internal/api$$' | paste -sd, -)
GOTESTSUM_JUNIT_FLAGS := --junitfile junit.xml --junitfile-testsuite-name relative --junitfile-testcase-classname relative --jsonfile gotest.log

.PHONY: test
test: fmt vet ## Run tests.
	gotestsum --format pkgname $(GOTESTSUM_JUNIT_FLAGS) -- $(TEST_PACKAGES) -coverprofile cover.out --coverpkg $(COVER_PACKAGES) -mod=readonly -race
