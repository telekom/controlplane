# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

.PHONY: generate build test fmt vet lint clean

generate:
	go run -mod=mod ent/entc/entc.go
	go run github.com/99designs/gqlgen

build: generate
	go build ./cmd/main.go

test:
	go test ./... -v

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: vet
	@echo "Lint passed"

clean:
	rm -f main
