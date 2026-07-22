# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Build context is the REPO ROOT (gateway depends on sibling modules via
# local replace directives: ../common, ../common-server, ../secret-manager).
FROM golang:1.26 AS build
WORKDIR /src

# Copy the modules the gateway build needs (replace targets in gateway/go.mod).
COPY common/ common/
COPY common-server/ common-server/
COPY secret-manager/ secret-manager/
COPY gateway/ gateway/

# CMD selects which command under gateway/cmd to build (xdsdemo | lms-issuer).
ARG CMD=xdsdemo
WORKDIR /src/gateway
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o /out/app ./cmd/${CMD}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
