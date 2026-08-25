#!/usr/bin/env bash
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

# Generates .github/ci/module-graph.json: a map of each CI module (as
# defined in .github/ci/modules.yaml) to the list of other CI modules it
# locally depends on, derived from `replace` directives in its go.mod.
#
# This is the single source of truth for cross-module dependencies used by
# .github/scripts/select-scope.sh to compute the "optimized" CI scope (changed
# modules + everything that transitively depends on them).
#
# Usage:
#   .github/scripts/gen-module-graph.sh > .github/ci/module-graph.json
#
# Run `make ci-graph` to regenerate the committed file, or `make
# ci-graph-check` to verify it is up to date (used in CI).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

MODULES_FILE=".github/ci/modules.yaml"
GO_MODULE_PREFIX="github.com/telekom/controlplane"

if ! command -v yq &>/dev/null; then
  echo "error: yq is required (https://github.com/mikefarah/yq)" >&2
  exit 1
fi
if ! command -v jq &>/dev/null; then
  echo "error: jq is required" >&2
  exit 1
fi

# module_paths: "name<TAB>path" for every configured module, longest path
# first so prefix matching below picks the most specific owner (e.g.
# "admin/api" must resolve to a module with path "admin", not a shorter
# unrelated prefix).
module_paths="$(yq -o=json '.modules[] | .name + "\t" + .path' "$MODULES_FILE" | jq -r . | awk -F'\t' '{print length($2)"\t"$0}' | sort -rn | cut -f2-)"

# Resolves a repo-root-relative directory (e.g. "admin/api") to the CI
# module name that owns it (e.g. "admin"), by longest-prefix match against
# configured module paths. Empty output if no module owns it.
owning_module() {
  local target="$1"
  while IFS=$'\t' read -r name path; do
    [ -z "$path" ] && continue
    if [ "$target" = "$path" ] || [[ "$target" == "$path"/* ]]; then
      echo "$name"
      return 0
    fi
  done <<<"$module_paths"
}

echo "{" >/tmp/module-graph.$$.json
first_module=true
while IFS=$'\t' read -r name path; do
  [ -z "$name" ] && continue
  gomod="$path/go.mod"
  deps=()
  if [ -f "$gomod" ]; then
    # Matches both single-line (`replace X => ../Y`) and block
    # (`replace (\n  X => ../Y\n)`) forms; only local (relative-path)
    # replace targets are dependency edges - module replacements pointing
    # at other hosts/versions are irrelevant here.
    while IFS= read -r target_rel; do
      [ -z "$target_rel" ] && continue
      # Resolve relative to the module's own directory, then make it
      # relative to the repo root.
      abs="$(cd "$path" && cd "$target_rel" 2>/dev/null && pwd || true)"
      [ -z "$abs" ] && continue
      rel="${abs#"$REPO_ROOT"/}"
      dep_name="$(owning_module "$rel")"
      if [ -n "$dep_name" ] && [ "$dep_name" != "$name" ]; then
        deps+=("$dep_name")
      fi
    done < <(grep -E "^[[:space:]]*(replace[[:space:]]+)?${GO_MODULE_PREFIX}/[^[:space:]]+[[:space:]]*=>[[:space:]]*\.\.?/" "$gomod" |
      sed -E 's#.*=>[[:space:]]*(\.\.?/[^[:space:]]+).*#\1#' || true)
  fi

  # Deduplicate and sort dependency names for a stable, diffable output.
  if [ "${#deps[@]}" -gt 0 ]; then
    deps_json="$(printf '%s\n' "${deps[@]}" | sort -u | jq -R . | jq -sc .)"
  else
    deps_json="[]"
  fi

  $first_module || echo "," >>/tmp/module-graph.$$.json
  first_module=false
  printf '  %s: {"path": %s, "depends_on": %s}' \
    "$(jq -R . <<<"$name")" "$(jq -R . <<<"$path")" "$deps_json" >>/tmp/module-graph.$$.json
done <<<"$(yq -o=json '.modules[] | .name + "\t" + .path' "$MODULES_FILE" | jq -r . | sort)"
echo "" >>/tmp/module-graph.$$.json
echo "}" >>/tmp/module-graph.$$.json

jq . /tmp/module-graph.$$.json
rm -f /tmp/module-graph.$$.json
