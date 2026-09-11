# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Final roverctl image published to the internal Artifactory Docker registry,
# combining the plain roverctl binary (built by goreleaser's `kos` pipe and
# published to GHCR, no GPL content) with bash/jq/yq (built from
# Dockerfile.base and published to Artifactory, GPL-2.0-only/GPL-3.0-or-later
# content). Kept as two separate images/pipelines so the GPL-free binary can
# still be published on public GHCR like every other module.
#
# ROVERCTL_IMAGE must reference the just-published GHCR image for this
# release (e.g. ghcr.io/telekom/controlplane/rover-ctl:<version>), and
# ROVERCTL_BASE_IMAGE must reference the bash/jq/yq base image published by
# rover-ctl-base-image.yaml (see .github/workflows/release.yaml, which sets
# both build-args when building/pushing this Dockerfile).
ARG ROVERCTL_IMAGE
ARG ROVERCTL_BASE_IMAGE

FROM ${ROVERCTL_IMAGE} AS roverctl-builder

FROM ${ROVERCTL_BASE_IMAGE}

# ko names the binary after the last segment of the built import path
# (`rover-ctl`, matching this module's directory name) - rename it to
# `roverctl` here so it matches the CLI's actual command name.
COPY --from=roverctl-builder /ko-app/rover-ctl /usr/local/bin/roverctl

USER 65532:65532
ENTRYPOINT ["/usr/local/bin/roverctl"]
