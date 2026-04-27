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

.PHONY: generate
generate: ## Run code generation.
	go generate ./tools/...

##@ Build

.PHONY: build
build: fmt vet ## Run go build against code.
	go build -o bin/server cmd/main.go

# Packages to test and measure coverage for
comma := ,
space := $(empty) $(empty)
TEST_PACKAGES = $(shell go list ./internal/... ./pkg/... | grep -v '^./internal/api$$')
COVER_PACKAGES = $(subst $(space),$(comma),$(strip $(TEST_PACKAGES)))

.PHONY: test
test: fmt vet ## Run tests.
	gotestsum --format pkgname --jsonfile gotest.log -- $(TEST_PACKAGES) -ginkgo.junit-report=ginkgo-junit.xml -coverprofile cover.out --coverpkg $(COVER_PACKAGES) -mod=readonly -race
