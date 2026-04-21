#!/usr/bin/env bash
# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Repo-level verification: runs all checks that must pass before pushing.
# Usage: hack/verify.sh [module...]
# Without arguments, checks all Go modules (excluding docs/ and tools/).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

ERRORS=0
FAILED=()

step() { printf "\n=== %s ===\n" "$1"; }
fail() { ERRORS=1; FAILED+=("$1"); echo "FAIL: $1" >&2; }

# --- Discover modules ---
if [ $# -gt 0 ]; then
  MODULES=("$@")
else
  MODULES=()
  while IFS= read -r dir; do
    MODULES+=("$dir")
  done < <(find . -name 'go.mod' -not -path './docs/*' -not -path '*/node_modules/*' -not -path './tools/*' -exec dirname {} \; | sed 's|^\./||' | sort)
fi

# --- 1. Pre-commit checks (REUSE, gitleaks) ---
step "pre-commit"
if command -v pre-commit &>/dev/null; then
  pre-commit run --all-files || fail "pre-commit"
else
  echo "SKIP: pre-commit not installed"
fi

# --- 2. Per-module checks ---
for mod in "${MODULES[@]}"; do
  pushd "$mod" > /dev/null

  # go mod tidy check
  step "$mod: go mod tidy"
  cp go.mod go.mod.bak
  cp go.sum go.sum.bak 2>/dev/null || true
  if go mod tidy 2>&1; then
    if ! diff -q go.mod go.mod.bak &>/dev/null || { [ -f go.sum.bak ] && ! diff -q go.sum go.sum.bak &>/dev/null; }; then
      fail "$mod: go.mod/go.sum not tidy"
    fi
  else
    fail "$mod: go mod tidy"
  fi
  mv go.mod.bak go.mod
  mv go.sum.bak go.sum 2>/dev/null || true

  # build
  step "$mod: make build"
  make build 2>&1 || fail "$mod: make build"

  # lint
  step "$mod: make lint"
  if grep -q '^lint:' Makefile 2>/dev/null; then
    make lint 2>&1 || fail "$mod: make lint"
  elif command -v golangci-lint &>/dev/null; then
    golangci-lint run --timeout 5m 2>&1 || fail "$mod: golangci-lint"
  else
    echo "SKIP: no lint target and golangci-lint not on PATH"
  fi

  popd > /dev/null
done

# --- Summary ---
echo ""
echo "==============================="
if [ $ERRORS -eq 0 ]; then
  echo "All checks passed."
else
  echo "FAILED:"
  printf "  - %s\n" "${FAILED[@]}"
  exit 1
fi
