# SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
# SPDX-License-Identifier: Apache-2.0

# Build context is the controlplane repository root because gateway uses local
# module replacements for sibling modules.
FROM golang:1.26 AS build
WORKDIR /src

COPY common/ common/
COPY common-server/ common-server/
COPY secret-manager/ secret-manager/
COPY gateway/ gateway/

WORKDIR /src/gateway
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o /out/xdsdemo ./cmd/xdsdemo && \
    CGO_ENABLED=0 go build -o /out/xds-management-server ./cmd/xds-management-server && \
    CGO_ENABLED=0 go build -o /out/lms-issuer ./cmd/lms-issuer

# setup-envtest is pinned, and the Kubernetes version matches gateway/Makefile.
RUN --mount=type=cache,target=/go/pkg/mod \
    GOBIN=/tools go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.19 && \
    /tools/setup-envtest use 1.32.0 --bin-dir /envtest

FROM debian:bookworm-slim
RUN groupadd --system --gid 65532 nonroot && \
    useradd --system --uid 65532 --gid nonroot --home-dir /home/nonroot --create-home nonroot && \
    install -d -o nonroot -g nonroot -m 0750 /state /data
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/ /
COPY --from=build /envtest/k8s/1.32.0-linux-amd64/ /envtest/
COPY gateway/config/crd/bases/ /crds/
USER nonroot:nonroot
ENTRYPOINT ["/xdsdemo"]
