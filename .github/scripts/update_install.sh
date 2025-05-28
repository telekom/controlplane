#!/bin/bash

# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# This script should only be run in a CI environment when a new release is created.
# It will update the install-files to point to the newly released version.
# See `.releaserc.mjs` for more information on how its integrated with semantic-release.

set -e

KUSTOMIZATION_FILE="install/kustomization.yaml"
NEXT_VERSION="$1"
if [ -z "$NEXT_VERSION" ]; then
  echo "Usage: $0 <next-version>"
  exit 1
fi

sed -i "s/ref=[^ ]*/ref=${NEXT_VERSION}/" "$KUSTOMIZATION_FILE"
sed -i "s/newTag: [^ ]*/newTag: ${NEXT_VERSION}/" "$KUSTOMIZATION_FILE"