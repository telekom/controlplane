# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Build stage
FROM golang:1.23 AS builder

WORKDIR /workspace

# Copy go mod files
COPY go.mod go.mod
COPY go.sum go.sum

# Copy the source code
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY internal/ internal/

# Copy dependencies (approval and migration modules)
COPY ../approval /workspace/approval
COPY ../migration /workspace/migration

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager cmd/main.go

# Runtime stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /workspace/manager .

USER 65532:65532

ENTRYPOINT ["/manager"]
