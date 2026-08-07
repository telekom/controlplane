# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

.PHONY: fmt vet test

fmt:
	go fmt ./...

vet:
	go vet ./...

test: fmt vet
	gotestsum --format pkgname --jsonfile gotest.log -- ./... -ginkgo.junit-report=ginkgo-junit.xml -race
