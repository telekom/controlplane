# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

.PHONY: generate build test fmt vet lint lint-fix clean

BOILERPLATE_GO = ../hack/boilerplate.go.txt
BOILERPLATE_GRAPHQL = ../hack/boilerplate.yaml.txt

generate:
	go run -mod=mod ent/entc/entc.go
	go run github.com/99designs/gqlgen
	@# Prepend SPDX headers to generated files that lack them
	@for f in $$(find ent/ -name '*.go' ! -path 'ent/schema/*' ! -path 'ent/entc/*') \
	          internal/resolvers/*.generated.go \
	          internal/resolvers/*.resolvers.go \
	          internal/resolvers/model/models.generated.go; do \
		if [ -f "$$f" ]; then \
			head -3 "$$f" | grep -q 'SPDX' || { cat $(BOILERPLATE_GO) "$$f" > "$$f.tmp" && mv "$$f.tmp" "$$f"; }; \
		fi; \
	done
	@if [ -f ent.graphql ]; then \
		head -3 ent.graphql | grep -q 'SPDX' || { cat $(BOILERPLATE_GRAPHQL) ent.graphql > ent.graphql.tmp && mv ent.graphql.tmp ent.graphql; }; \
	fi

build: generate
	go build -o bin/controlplane-api ./cmd/main.go

# Packages to test and measure coverage for
TEST_PACKAGES = ./internal/...
COVER_PACKAGES = ./internal/...
GOTESTSUM_JUNIT_FLAGS = --junitfile junit.xml --junitfile-testsuite-name relative --junitfile-testcase-classname relative --jsonfile gotest.log

test:
	gotestsum --format pkgname $(GOTESTSUM_JUNIT_FLAGS) -- $(TEST_PACKAGES) -coverprofile cover.out --coverpkg $(COVER_PACKAGES) -race

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run

lint-fix:
	golangci-lint run --fix

clean:
	rm -rf bin/
