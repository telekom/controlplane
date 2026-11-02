#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

prefix="${LABEL_KEY_PREFIX:-cp.ei.telekom.de}"
apply=false

if [[ $# -gt 1 || ( $# -eq 1 && "$1" != "--apply" ) ]]; then
	echo "Usage: LABEL_KEY_PREFIX=<prefix> $0 [--apply]" >&2
	exit 2
fi

if [[ $# -eq 1 ]]; then
	apply=true
fi

command -v kubectl >/dev/null || { echo "kubectl is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }

shared_key="${prefix}/basepath"
domain_key="${prefix}/domain"

label_object() {
	local resource="$1" namespace="$2" name="$3" key="$4" value="$5"
	shift 5
	local labels=("$key=$value" "$@")

	if [[ "$apply" == true ]]; then
		kubectl label "$resource" -n "$namespace" "$name" "${labels[@]}" --overwrite
	else
		printf '[dry-run] kubectl label %q -n %q %q' "$resource" "$namespace" "$name"
		printf ' %q' "${labels[@]}"
		printf ' --overwrite\n'
	fi
}

migrate_legacy_label() {
	local resource="$1" legacy_key="$2"
	local items

	items="$(kubectl get "$resource" --all-namespaces -o json |
		jq -r --arg key "$legacy_key" '
			.items[]
			| select(.metadata.labels[$key] != null)
			| [.metadata.namespace, .metadata.name, .metadata.labels[$key]]
			| @tsv
		')"

	while IFS=$'\t' read -r namespace name value; do
		[[ -n "$namespace" && -n "$name" && -n "$value" ]] || continue
		if [[ "$resource" == "routes.gateway.cp.ei.telekom.de" ]]; then
			label_object "$resource" "$namespace" "$name" "$shared_key" "$value" "$domain_key=agentic"
		else
			label_object "$resource" "$namespace" "$name" "$shared_key" "$value"
		fi
	done <<< "$items"
}

migrate_legacy_label "mcpservers.agentic.cp.ei.telekom.de" "${prefix}/mcpbasepath"
migrate_legacy_label "agenticexposures.agentic.cp.ei.telekom.de" "${prefix}/mcpbasepath"
migrate_legacy_label "agenticsubscriptions.agentic.cp.ei.telekom.de" "${prefix}/mcpbasepath"
migrate_legacy_label "agentcards.agentic.cp.ei.telekom.de" "${prefix}/agentbasepath"
migrate_legacy_label "routes.gateway.cp.ei.telekom.de" "${prefix}/mcpbasepath"
migrate_legacy_label "routes.gateway.cp.ei.telekom.de" "${prefix}/agentbasepath"

# Existing API routes need an ownership-domain label so API cleanup cannot select
# agentic routes that now share the basepath label.
api_routes="$(kubectl get routes.gateway.cp.ei.telekom.de --all-namespaces -o json |
	jq -r --arg basepath "$shared_key" --arg domain "$domain_key" '
		.items[]
		| select(.metadata.labels[$basepath] != null and .metadata.labels[$domain] == null)
		| [.metadata.namespace, .metadata.name]
		| @tsv
	')"

while IFS=$'\t' read -r namespace name; do
	[[ -n "$namespace" && -n "$name" ]] || continue
	label_object "routes.gateway.cp.ei.telekom.de" "$namespace" "$name" "$domain_key" "api"
done <<< "$api_routes"
