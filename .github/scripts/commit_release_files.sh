#!/bin/bash

# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Commits the version-bump files produced by update_install.sh and
# update_chart_version.sh locally, so they become part of the commit that
# semantic-release core then tags. Deliberately does NOT push to the
# branch - see .releaserc.mjs for why: semantic-release core pushes the
# tag itself, and pushing this commit to `main` separately would bypass
# required PR review and re-trigger CI on push.
#
# This script is designed to be used with semantic-release during the
# prepare phase (see `.releaserc.mjs`).

set -e

NEXT_VERSION="$1"
if [ -z "$NEXT_VERSION" ]; then
  echo "Usage: $0 <next-version>"
  exit 1
fi

git config user.name "github-actions[bot]"
git config user.email "github-actions[bot]@users.noreply.github.com"

git add \
  install/overlays/default/kustomization.yaml \
  common-server/helm/Chart.yaml

if git diff --cached --quiet; then
  echo "No version-bump changes to commit"
  exit 0
fi

git commit -m "chore(release): ${NEXT_VERSION} [skip ci]"
