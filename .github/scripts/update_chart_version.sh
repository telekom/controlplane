#!/bin/bash

# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# This script updates version and appVersion in a Helm chart's Chart.yaml
# It is designed to be used with semantic-release during the release process.

set -e

CHART_PATH="$1"
VERSION="$2"

if [ -z "$CHART_PATH" ] || [ -z "$VERSION" ]; then
  echo "Usage: $0 <chart-path> <version>"
  echo "Example: $0 common-server/helm 1.2.3"
  exit 1
fi

# Strip 'v' prefix if present
VERSION="${VERSION#v}"

# Ensure Chart.yaml exists
if [ ! -f "$CHART_PATH/Chart.yaml" ]; then
  echo "Error: Chart.yaml not found at $CHART_PATH/Chart.yaml"
  exit 1
fi

echo "Updating $CHART_PATH/Chart.yaml to version $VERSION"

# Update version and appVersion in Chart.yaml
sed -i "s/^version: .*/version: $VERSION/" "$CHART_PATH/Chart.yaml"
sed -i "s/^appVersion: .*/appVersion: $VERSION/" "$CHART_PATH/Chart.yaml"

echo "Chart version updated successfully"