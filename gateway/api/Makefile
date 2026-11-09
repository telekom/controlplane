# Copyright 2026 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

.PHONY: build fmt vet test tidy

build: fmt vet
	go build ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

test: fmt vet
	gotestsum --format pkgname --jsonfile gotest.log -- ./... -ginkgo.junit-report=ginkgo-junit.xml -race

tidy:
	go mod tidy
