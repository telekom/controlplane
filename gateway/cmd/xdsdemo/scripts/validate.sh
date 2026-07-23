#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
# SPDX-License-Identifier: Apache-2.0

# Validates the durable xDS demo. Run after `docker compose up -d --build`.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose=(docker compose -f "${here}/docker-compose.yaml")
if [[ -f "${here}/.env" ]]; then
  # shellcheck disable=SC1091
  set -a; source "${here}/.env"; set +a
fi
control="${CONTROL_URL:-http://localhost:${CONTROL_PORT:-18081}}"
management_health="${MANAGEMENT_HEALTH_URL:-http://localhost:${MANAGEMENT_HEALTH_PORT:-18080}}"
envoy_a="${ENVOY_A_URL:-http://localhost:${ENVOY_A_PORT:-10000}}"
envoy_b="${ENVOY_B_URL:-http://localhost:${ENVOY_B_PORT:-10001}}"
route_host="${ROUTE_HOST:-demo-route.local}"
pass=0

pass() {
  printf 'PASS: %s\n' "$1"
  pass=$((pass + 1))
}

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

wait_http() {
  local url="$1" expected="$2" host="${3:-$route_host}" deadline=$((SECONDS + 60)) code
  while (( SECONDS < deadline )); do
    code=$(curl -sS -o /dev/null -w '%{http_code}' -H "Host: ${host}" "$url" || true)
    [[ "$code" == "$expected" ]] && return 0
    sleep 1
  done
  fail "${url} did not return HTTP ${expected}; last response ${code}"
}

wait_generation_after() {
  local previous="$1" deadline=$((SECONDS + 30)) current
  while (( SECONDS < deadline )); do
    current=$(curl -fsS "${control}/status" 2>/dev/null | jq -r '.generation // 0' || true)
    [[ "$current" =~ ^[0-9]+$ ]] && (( current > previous )) && { printf '%s\n' "$current"; return 0; }
    sleep 1
  done
  fail "active generation did not advance beyond ${previous}"
}

status() {
  curl -fsS "${control}/status"
}

put_route() {
  local path="$1" issuer="${2:-}" consumer="${3:-}"
  jq -nc --arg path "$path" --arg host "$route_host" --arg issuer "$issuer" --arg consumer "$consumer" \
    '{path:$path,host:$host,issuer:$issuer,consumer:$consumer}' |
    curl -fsS -X PUT -H 'Content-Type: application/json' --data-binary @- "${control}/routes/demo" >/dev/null
}

assert_two_node_acks() {
  local generation="$1" deadline=$((SECONDS + 30)) current
  while (( SECONDS < deadline )); do
    current=$(status 2>/dev/null || true)
    if [[ -n "$current" ]] && jq -e --argjson generation "$generation" '
      .converged == true and
      (.connectedNodes | sort) == ["demo-envoy-a", "demo-envoy-b"] and
      ([.observations[] | select(.generation == $generation and .state == "DELIVERY_STATE_ACK" and .nodeId == "demo-envoy-a")] | length) >= 4 and
      ([.observations[] | select(.generation == $generation and .state == "DELIVERY_STATE_ACK" and .nodeId == "demo-envoy-b")] | length) >= 4
    ' <<<"$current" >/dev/null; then
      return 0
    fi
    sleep 1
  done
  fail "two Envoy nodes did not independently ACK generation ${generation}: ${current}"
}

assert_two_node_nacks() {
  local generation="$1" deadline=$((SECONDS + 30)) current
  while (( SECONDS < deadline )); do
    current=$(status 2>/dev/null || true)
    if [[ -n "$current" ]] && jq -e --argjson generation "$generation" '
      .converged == false and
      ([.observations[] | select(.generation == $generation and .state == "DELIVERY_STATE_NACK" and .nodeId == "demo-envoy-a")] | length) >= 1 and
      ([.observations[] | select(.generation == $generation and .state == "DELIVERY_STATE_NACK" and .nodeId == "demo-envoy-b")] | length) >= 1
    ' <<<"$current" >/dev/null; then
      return 0
    fi
    sleep 1
  done
  fail "two Envoy nodes did not independently NACK generation ${generation}: ${current}"
}

printf '== waiting for separated stack ==\n'
wait_http "${control}/healthz" 200 localhost
initial=$(status | jq -r '.generation')
wait_http "${envoy_a}/created" 404
wait_http "${envoy_b}/created" 404

printf '== route create through real Gateway reconciler ==\n'
put_route /created
created=$(wait_generation_after "$initial")
wait_http "${envoy_a}/created" 200
wait_http "${envoy_b}/created" 200
assert_two_node_acks "$created"
pass "route create reached both Envoys without restart"

printf '== route update without Envoy restart ==\n'
put_route /updated
updated=$(wait_generation_after "$created")
wait_http "${envoy_a}/created" 404
wait_http "${envoy_a}/updated" 200
wait_http "${envoy_b}/updated" 200
assert_two_node_acks "$updated"
pass "route update replaced active routing on both nodes"

printf '== independent downstream NACK recording and recovery ==\n'
nack=$(curl -fsS -X POST "${control}/publish/nack" | jq -r '.generation')
assert_two_node_nacks "$nack"
wait_http "${envoy_a}/updated" 200
wait_http "${envoy_b}/updated" 200
put_route /recovered
recovered=$(wait_generation_after "$nack")
wait_http "${envoy_a}/updated" 404
wait_http "${envoy_a}/recovered" 200
wait_http "${envoy_b}/recovered" 200
assert_two_node_acks "$recovered"
updated="$recovered"
active_path=/recovered
pass "both Envoys recorded NACKs, retained last accepted routing, and recovered"

printf '== idempotent publication ==\n'
before=$(status)
idempotent=$(curl -fsS -X POST "${control}/publish/idempotent")
after=$(status)
jq -e '.idempotent == true' <<<"$idempotent" >/dev/null || fail "republish was not idempotent: ${idempotent}"
[[ "$(jq -r '.generation' <<<"$before")" == "$(jq -r '.generation' <<<"$after")" ]] || fail "idempotent republish changed generation"
[[ "$(jq -r '.digest' <<<"$before")" == "$(jq -r '.digest' <<<"$after")" ]] || fail "idempotent republish changed digest"
pass "identical publication retained generation and digest"

printf '== invalid bundle rejection ==\n'
invalid=$(curl -fsS -X POST "${control}/publish/invalid")
after_invalid=$(status)
jq -e '.rejected == true and .code == "InvalidArgument"' <<<"$invalid" >/dev/null || fail "invalid bundle was not rejected: ${invalid}"
[[ "$(jq -r '.generation' <<<"$after")" == "$(jq -r '.generation' <<<"$after_invalid")" ]] || fail "invalid bundle changed generation"
[[ "$(jq -r '.digest' <<<"$after")" == "$(jq -r '.digest' <<<"$after_invalid")" ]] || fail "invalid bundle changed digest"
wait_http "${envoy_a}${active_path}" 200
pass "invalid publication preserved last-known-good generation"

printf '== route delete by omission ==\n'
curl -fsS -X DELETE "${control}/routes/demo" >/dev/null
deleted=$(wait_generation_after "$updated")
wait_http "${envoy_a}${active_path}" 404
wait_http "${envoy_b}${active_path}" 404
pass "route delete reached both Envoys without restart"

if [[ -n "${CLIENT_ID:-}" && -n "${CLIENT_SECRET:-}" && -n "${TOKEN_URL:-}" ]]; then
  printf '== optional live JWT/JWKS, RBAC, and LMS ==\n'
  issuer="${ISSUER:-${TOKEN_URL%/protocol/openid-connect/token}}"
  put_route /secure "$issuer" "$CLIENT_ID"
  secured=$(wait_generation_after "$deleted")
  wait_http "${envoy_a}/secure" 401
  token=$(curl -fsS -X POST "$TOKEN_URL" -d grant_type=client_credentials \
    -d "client_id=${CLIENT_ID}" -d "client_secret=${CLIENT_SECRET}" | jq -er '.access_token')
  code=$(curl -sS -o /dev/null -w '%{http_code}' -H "Host: ${route_host}" \
    -H "Authorization: Bearer ${token}" "${envoy_a}/secure")
  [[ "$code" == 200 ]] || fail "valid live token returned HTTP ${code}"
  upstream_auth=$(curl -fsS -H "Host: ${route_host}" -H "Authorization: Bearer ${token}" "${envoy_a}/secure" |
    jq -r '.headers.Authorization[0]? // .headers.Authorization // empty')
  [[ -n "$upstream_auth" && "$upstream_auth" != "Bearer ${token}" ]] || fail "LMS token did not replace consumer token"
  assert_two_node_acks "$secured"
  pass "live JWT/JWKS, RBAC, and LMS route"
  previous="$secured"
else
  printf 'SKIP: optional live JWT/JWKS scenario; configure .env to enable\n'
  previous="$deleted"
fi

printf '== prepare last-known-good outage route ==\n'
put_route /outage
outage=$(wait_generation_after "$previous")
wait_http "${envoy_a}/outage" 200
wait_http "${envoy_b}/outage" 200
assert_two_node_acks "$outage"

printf '== management restart and SQLite restore ==\n'
"${compose[@]}" restart management >/dev/null
wait_http "${management_health}/readyz" 200 localhost
wait_http "${envoy_a}/outage" 200
restored=$(status)
[[ "$(jq -r '.generation' <<<"$restored")" == "$outage" ]] || fail "management restart restored wrong generation"
pass "management restart restored active SQLite generation"

printf '== operator outage plus management and Envoy restart ==\n'
"${compose[@]}" stop operator >/dev/null
"${compose[@]}" restart management >/dev/null
wait_http "${management_health}/readyz" 200 localhost
"${compose[@]}" restart envoy-a envoy-b >/dev/null
wait_http "${envoy_a}/outage" 200
wait_http "${envoy_b}/outage" 200
pass "both Envoys recovered last-known-good routing with operator unavailable"

printf '== summary: %d checks passed ==\n' "$pass"
