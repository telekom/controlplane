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
	go build -ldflags="-X 'main.version=$(VERSION)' -X 'main.buildTime=$(BUILD_TIME)' -X 'main.gitCommit=$(GIT_COMMIT)' -X 'main.goVersion=$(shell go version)'" -trimpath -mod=readonly -o bin/roverctl main.go

# Packages to test and measure coverage for
TEST_PACKAGES = ./pkg/...
COVER_PACKAGES = ./pkg/...
GOTESTSUM_JUNIT_FLAGS = --junitfile junit.xml --junitfile-testsuite-name relative --junitfile-testcase-classname relative --jsonfile gotest.log

.PHONY: test
test: fmt vet ## Run tests.
	gotestsum --format pkgname $(GOTESTSUM_JUNIT_FLAGS) -- $(TEST_PACKAGES) -coverprofile cover.out --coverpkg $(COVER_PACKAGES) -mod=readonly -race

.PHONY: install
install: build ## Install roverctl binary.
	install -D -m 0755 bin/roverctl $(DESTDIR)/usr/local/bin/roverctl