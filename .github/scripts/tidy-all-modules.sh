#!/bin/bash
# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# This script:
# 1. Finds all go.mod files in the repository
# 2. Extracts their directories
# 3. Sorts them for consistent ordering
# 4. Runs go mod tidy in each module director

set -euo pipefail

echo "Identified modules:"
MODULES=$(find . -name 'go.mod' -exec dirname {} \; | sort)
echo "$MODULES"
echo -e "\nRunning go mod tidy on all modules..."
echo "$MODULES" | while read -r module_dir; do
  if [ -n "$module_dir" ]; then
    echo "Processing: $module_dir"
    (cd "$module_dir" && go mod tidy)
  fi
done
echo -e "\nDone!"
