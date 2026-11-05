#!/usr/bin/env bash

# Copyright 2026 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Seeds a Kong service, route and three route-bound plugins tagged like the
# legacy operator did, and generates the matching gateway Route CR. Applying
# the CR is left to the user; `verify` then checks that the gateway operator
# adopted the seeded entities. See gateway/hack/legacy-route-seed.md.

# jq filters are single-quoted on purpose.
# shellcheck disable=SC2016

set -euo pipefail
set +x
umask 077

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
workspace_dir=$(cd "$script_dir/../../.." && pwd)

readonly LEGACY_TAG='ei__telekom__de--realm---default'
readonly PLUGINS=(acl jwt-keycloak request-transformer)
readonly BACKEND_HOST='cosmoparrot.backend.svc.cluster.local'
readonly BACKEND_PORT=8080
readonly BACKEND_URL="http://$BACKEND_HOST:$BACKEND_PORT/"
# Authorization replacement of the request-transformer (last_mile_security.go).
readonly AUTH_HEADER="\$(headers['consumer-token'] or headers['Authorization'])"
# The operator always appends an empty Jumper config (custom_scopes.go,
# builder.go): base64 of "{}".
readonly JUMPER_CONFIG='jumper_config:e30='
readonly RTP_HEADER_KEYS='["access_token_forwarding","api_base_path","environment","jumper_config","realm","remote_api_url"]'

usage() {
  cat <<EOF
Usage:
  $(basename "$0") setup --context NAME [--run] [options]
  $(basename "$0") verify --context NAME [options]

setup   Validates the fixture, resolves the target read-only and prints the
        Kong payloads and the Route CR. With --run it creates the Kong
        entities, journals their IDs and writes route.yaml. It never applies
        the CR.
verify  Read-only: waits for the Route CR to be processed and checks that
        Kong still holds exactly the seeded entities, now owned by the operator.

Options:
  --context NAME       kubeconfig context to use (required)
  --kubeconfig PATH    default: \$KUBECONFIG or
                       <workspace>/config/ephemeral/clusters/pr-805/kubeconfig
  --fixture PATH       legacy decK-like dump (default: <workspace>/legacy.yaml)
  --environment NAME   default: pr-805
  --zone NAME          default: dp1
  --out DIR            state directory outside any Git work tree
                       (default: /tmp/opencode/legacy-route-seed/<context>)
  --expect-image REF   setup fails unless the gateway operator runs REF
  --timeout SECONDS    verify: wait for the Route CR (default: 180)
EOF
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}
warn() { printf 'warning: %s\n' "$*" >&2; }
log() { printf '%s\n' "$*" >&2; }

# --- arguments ---------------------------------------------------------------

COMMAND=${1:-}
case "$COMMAND" in
  setup | verify) shift ;;
  -h | --help) usage; exit 0 ;;
  *) usage >&2; exit 2 ;;
esac

RUN=false
CONTEXT=''
KUBECONFIG_PATH=${KUBECONFIG:-$workspace_dir/config/ephemeral/clusters/pr-805/kubeconfig}
FIXTURE=$workspace_dir/legacy.yaml
ENVIRONMENT=pr-805
ZONE=dp1
OUT=''
EXPECT_IMAGE=''
TIMEOUT=180

while [[ $# -gt 0 ]]; do
  case "$1" in
    --run) RUN=true; shift ;;
    --context | --kubeconfig | --fixture | --environment | --zone | --out | --expect-image | --timeout)
      [[ $# -ge 2 && -n $2 ]] || die "$1 needs a value"
      case "$1" in
        --context) CONTEXT=$2 ;;
        --kubeconfig) KUBECONFIG_PATH=$2 ;;
        --fixture) FIXTURE=$2 ;;
        --environment) ENVIRONMENT=$2 ;;
        --zone) ZONE=$2 ;;
        --out) OUT=$2 ;;
        --expect-image) EXPECT_IMAGE=$2 ;;
        --timeout) TIMEOUT=$2 ;;
      esac
      shift 2
      ;;
    -h | --help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ $COMMAND == setup || $RUN == false ]] || die "--run is only valid for setup"
[[ $TIMEOUT =~ ^[0-9]+$ ]] || die "--timeout must be a number of seconds"
for tool in kubectl jq yq curl base64; do
  command -v "$tool" >/dev/null || die "$tool is required"
done

# --- kubernetes (read-only) --------------------------------------------------

kc() { KUBECONFIG="$KUBECONFIG_PATH" kubectl --context "$CONTEXT" --request-timeout=20s "$@"; }

check_context() {
  [[ -n $CONTEXT ]] || die "--context is required"
  local contexts
  contexts=$(KUBECONFIG="$KUBECONFIG_PATH" kubectl config get-contexts -o name) ||
    die "cannot read contexts from the kubeconfig"
  grep -Fxq -- "$CONTEXT" <<<"$contexts" ||
    die "context $CONTEXT not found; available: $(tr '\n' ' ' <<<"$contexts")"
  [[ -n $OUT ]] || OUT=/tmp/opencode/legacy-route-seed/$CONTEXT
}

# Resolves gateway, issuer and hostname from the zone's default preset.
resolve_target() {
  local zone gateway
  zone=$(kc -n "$ENVIRONMENT" get zones.admin.cp.ei.telekom.de "$ZONE" -o json) ||
    die "cannot read zone $ENVIRONMENT/$ZONE"
  jq -e '.status.presets[]? | select(.name == "default")
    | .gatewayRef.name and .gatewayRef.namespace and .links.gatewayIssuer and .links.gatewayUrl' \
    <<<"$zone" >/dev/null || die "zone $ZONE has no complete default preset in its status"
  GATEWAY_NAME=$(jq -r '.status.presets[] | select(.name == "default") | .gatewayRef.name' <<<"$zone")
  NAMESPACE=$(jq -r '.status.presets[] | select(.name == "default") | .gatewayRef.namespace' <<<"$zone")
  ISSUER=$(jq -r '.status.presets[] | select(.name == "default") | .links.gatewayIssuer' <<<"$zone")
  HOSTNAME_=$(jq -r '.status.presets[] | select(.name == "default") | .links.gatewayUrl
    | sub("^[a-z]+://"; "") | split("/")[0] | split(":")[0]' <<<"$zone")

  gateway=$(kc -n "$NAMESPACE" get gateways.gateway.cp.ei.telekom.de "$GATEWAY_NAME" -o json) ||
    die "cannot read gateway $NAMESPACE/$GATEWAY_NAME"
  log "gateway $NAMESPACE/$GATEWAY_NAME ready: $(jq -r '[.status.conditions[]? | select(.type == "Ready") | .status][0] // "unknown"' <<<"$gateway")"

  local image
  if image=$(kc -n controlplane-system get deployment gateway-controller-manager \
    -o jsonpath='{.spec.template.spec.containers[?(@.name=="manager")].image}' 2>/dev/null) && [[ -n $image ]]; then
    log "gateway operator image: $image"
  else
    image=''
    warn "cannot read the gateway operator image"
  fi
  [[ -z $EXPECT_IMAGE || $image == "$EXPECT_IMAGE" ]] ||
    die "gateway operator runs '${image:-unknown}', expected '$EXPECT_IMAGE'"
}

# --- fixture and payloads ----------------------------------------------------

# Strips ANSI escapes and checks the fixture holds one service, one route and
# exactly the three route-only plugins with the legacy tags.
load_fixture() {
  [[ -r $FIXTURE ]] || die "cannot read fixture $FIXTURE"
  FIXTURE_JSON=$(sed $'s/\x1b\\[[0-9;]*m//g' "$FIXTURE" | yq -o=json '.') || die "fixture is not valid YAML"
  local problems
  problems=$(jq -r --arg lt "$LEGACY_TAG" --argjson keys "$RTP_HEADER_KEYS" '
    def expected_tags($r): [$lt, .name, $r] + (if .name == "request-transformer" then ["proxy_route"] else [] end) | sort;
    .route.name as $r
    | (if ($r | type) != "string" or ($r | test("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$") | not) then "route name is not a DNS-1123 label" else empty end),
      (if .service.name != $r then "service name differs from route name" else empty end),
      (if (.route.paths | length) != 1 then "route must have exactly one path" else empty end),
      (if [.service.protocol, .service.host, .service.port, .service.path] != ["http", "localhost", 8080, "/proxy"]
        then "service is not the local Jumper endpoint http://localhost:8080/proxy" else empty end),
      (if ([.plugins[]?.name] | sort) != ["acl", "jwt-keycloak", "request-transformer"]
        then "plugins must be exactly acl, jwt-keycloak and request-transformer" else empty end),
      (.plugins[]? | select(.route == null or .service != null or .consumer != null) | "plugin \(.name) is not bound to the route only"),
      (.plugins[]? | select(.protocols != ["http"]) | "plugin \(.name) protocols are not [http]"),
      (.plugins[]? | select((.tags | sort) != expected_tags($r)) | "plugin \(.name) tags are not the legacy tags"),
      (.plugins[]? | select(.name == "request-transformer") | .config.append.headers[]?
        | split(":")[0] | select(IN($keys[]) | not) | "unexpected transformer header \(.)")
  ' <<<"$FIXTURE_JSON") || die "cannot validate fixture"
  [[ -z $problems ]] || die "invalid fixture:"$'\n'"$problems"
  ROUTE_NAME=$(jq -r '.route.name' <<<"$FIXTURE_JSON")
  ROUTE_PATH=$(jq -r '.route.paths[0]' <<<"$FIXTURE_JSON")
}

# Builds allowlisted payloads with the test environment's hosts, issuer, ACL
# group, realm and backend. Plugin configs never copy fixture values.
build_plan() {
  CONSUMER="$ENVIRONMENT-legacy-test"
  PLAN=$(jq --arg env "$ENVIRONMENT" --arg zone "$ZONE" --arg host "$HOSTNAME_" --arg iss "$ISSUER" \
    --arg consumer "$CONSUMER" --arg backend "$BACKEND_URL" --arg lt "$LEGACY_TAG" --arg auth "$AUTH_HEADER" '
    . as $f
    | ($f.plugins | map({key: .name, value: .}) | from_entries) as $p
    | [$lt, "ei__telekom__de--environment---\($env)", "ei__telekom__de--zone---\($zone)"] as $tags
    | {
      service: ($f.service | {name, protocol, host, port, path, retries, connect_timeout, read_timeout, write_timeout}
        | with_entries(select(.value != null)) | .tags = $tags),
      route: ($f.route | {name, protocols, paths, strip_path, preserve_host, path_handling, https_redirect_status_code,
          regex_priority, request_buffering, response_buffering}
        | with_entries(select(.value != null)) | .hosts = [$host] | .tags = $tags),
      plugins: {
        acl: {config: {allow: [$consumer], hide_groups_header: false}},
        "jwt-keycloak": {config: {allowed_iss: [$iss], consumer_match: true, consumer_match_claim: "azp",
          consumer_match_claim_custom_id: true, consumer_match_ignore_not_found: false}},
        "request-transformer": {config: {
          # Only the header keys come from the fixture; every value is synthetic.
          append: {headers: [($p["request-transformer"].config.append.headers // [])[] | split(":")[0]
            | {access_token_forwarding: "false", api_base_path: $f.route.paths[0], environment: $env,
               jumper_config: "{}", realm: $env, remote_api_url: $backend}[.] as $v | "\(.):\($v)"]},
          remove: {headers: ["consumer-token"]},
          replace: {headers: ["Authorization:\($auth)"]}
        }}
      }
    }
    | .plugins |= with_entries(.key as $n | .value += {name: $n, enabled: true, protocols: ["http"], tags: ($p[$n].tags | sort)})
  ' <<<"$FIXTURE_JSON") || die "cannot build payloads"
}

route_cr() {
  cat <<EOF
apiVersion: gateway.cp.ei.telekom.de/v1
kind: Route
metadata:
  name: $ROUTE_NAME
  namespace: $NAMESPACE
  labels:
    cp.ei.telekom.de/environment: $ENVIRONMENT
spec:
  gatewayRef:
    name: $GATEWAY_NAME
    namespace: $NAMESPACE
  type: primary
  hostnames:
    - $HOSTNAME_
  paths:
    - $ROUTE_PATH
  passThrough: false
  backend:
    upstreams:
      - scheme: http
        hostname: $BACKEND_HOST
        port: $BACKEND_PORT
        path: /
  traffic: {}
  security:
    realmName: $ENVIRONMENT
    disableAccessControl: false
    trustedIssuers:
      - $ISSUER
    defaultConsumers:
      - $CONSUMER
EOF
}

# --- Kong admin API ----------------------------------------------------------

WORK=''
PF_PID=''
cleanup() {
  if [[ -n $PF_PID ]]; then
    kill "$PF_PID" 2>/dev/null || true
    wait "$PF_PID" 2>/dev/null || true
  fi
  [[ -z $WORK ]] || rm -rf -- "$WORK"
}
trap cleanup EXIT
WORK=$(mktemp -d)

# tunnel_alive: succeeds while the port-forward started by this process runs.
# jobs -rp, unlike kill -0, does not report an exited (zombie) child.
tunnel_alive() { [[ -n $PF_PID ]] && jobs -rp | grep -Fxq -- "$PF_PID"; }

# kong METHOD PATH [BODY_FILE]: sets KONG_STATUS, body in $WORK/resp.json.
# Requests only go through this process's own live tunnel. -q must come first
# so that no ambient .curlrc is read; the credentials only live in the private
# curl config.
kong() {
  tunnel_alive || die "the Kong admin port-forward is not running"
  local args=(-q -sS --config "$WORK/curl.cfg" --noproxy '*' --connect-timeout 5 --max-time 20
    -o "$WORK/resp.json" -w '%{http_code}' -X "$1")
  [[ -z ${3:-} ]] || args+=(-H 'Content-Type: application/json' --data-binary "@$3")
  KONG_STATUS=$(curl "${args[@]}" "$ADMIN_URL$2") || KONG_STATUS=000
}

# kong_ok JQ_ARGS...: succeeds if the last response is 200 and matches the filter.
kong_ok() {
  [[ $KONG_STATUS == 200 ]] || return 1
  jq -e "$@" "$WORK/resp.json" >/dev/null
}

kong_body() { cat "$WORK/resp.json" 2>/dev/null || true; }

# Writes the admin credentials to a private curl config and opens a
# port-forward to the Kong admin service owned by this process.
open_admin() {
  local key
  key=$(kc -n "$ZONE" get secret "gateway-$ZONE" -o jsonpath='{.data.gatewayAdminApiKey}') ||
    die "cannot read secret $ZONE/gateway-$ZONE"
  key=$(printf '%s' "$key" | base64 -d) || die "cannot decode the admin API key"
  [[ -n $key ]] || die "admin API key is empty"
  key=${key//\\/\\\\}
  printf 'user = "admin:%s"\n' "${key//\"/\\\"}" >"$WORK/curl.cfg"
  unset key

  # kubectl picks a free loopback port and reports it once it listens; no
  # request is sent before that line comes from this process's own tunnel.
  KUBECONFIG="$KUBECONFIG_PATH" kubectl --context "$CONTEXT" -n "$ZONE" port-forward \
    --address 127.0.0.1 "svc/gateway-$ZONE-admin" :8001 >"$WORK/port-forward.log" 2>&1 &
  PF_PID=$!
  local port='' deadline=$((SECONDS + 20))
  while [[ -z $port ]]; do
    tunnel_alive || die "port-forward to svc/gateway-$ZONE-admin failed: $(head -c 300 "$WORK/port-forward.log")"
    ((SECONDS < deadline)) || die "port-forward to svc/gateway-$ZONE-admin did not become ready"
    port=$(sed -nE 's/^Forwarding from 127\.0\.0\.1:([0-9]+) -> 8001$/\1/p' "$WORK/port-forward.log" | head -n1)
    [[ -n $port ]] || sleep 0.2
  done
  ADMIN_URL=http://127.0.0.1:$port
  kong GET /
  [[ $KONG_STATUS == 200 ]] || die "Kong admin API at $ADMIN_URL answered $KONG_STATUS"
}

# --- setup -------------------------------------------------------------------

JOURNAL=''
journal() {
  printf '%s\t%s\n' "$1" "$2" >>"$JOURNAL"
  log "created $1 $2"
}

fail_partial() {
  warn "seed is incomplete: $*"
  warn "created IDs are journaled in $JOURNAL; nothing was deleted"
  exit 1
}

# create KIND PATH BODY: POSTs the body and journals the returned ID.
create() {
  printf '%s' "$3" >"$WORK/body.json"
  kong POST "$2" "$WORK/body.json"
  [[ $KONG_STATUS == 201 ]] || fail_partial "creating $1 failed ($KONG_STATUS): $(kong_body | head -c 300)"
  CREATED_ID=$(jq -r '.id // empty' "$WORK/resp.json")
  [[ -n $CREATED_ID ]] || fail_partial "Kong returned no id for $1"
  journal "$1" "$CREATED_ID"
}

preflight_kong() {
  jq -e --args '[$ARGS.positional[] as $n | .plugins.available_on_server[$n] // false] | all' \
    "${PLUGINS[@]}" <"$WORK/resp.json" >/dev/null ||
    die "Kong does not offer all of: ${PLUGINS[*]}"
  local kind name
  for kind in services routes; do
    kong GET "/$kind/$ROUTE_NAME"
    case $KONG_STATUS in
      404) ;;
      200) die "Kong ${kind%s} $ROUTE_NAME already exists; refusing to overwrite" ;;
      *) die "cannot check Kong ${kind%s} $ROUTE_NAME ($KONG_STATUS)" ;;
    esac
  done
  for name in "${PLUGINS[@]}"; do
    kong GET "/plugins?tags=$LEGACY_TAG,$name,$ROUTE_NAME"
    [[ $KONG_STATUS == 200 ]] || die "cannot list legacy $name plugins ($KONG_STATUS)"
    jq -e '.data == []' "$WORK/resp.json" >/dev/null ||
      die "a legacy $name plugin for $ROUTE_NAME already exists; refusing to overwrite"
  done
}

# Reads back the seeded entities and checks IDs, bindings and exact tags.
readback() {
  local service_id=$1 route_id=$2 name id
  kong GET "/services/$service_id"
  kong_ok --argjson want "$(jq '.service.tags | sort' <<<"$PLAN")" \
    '(.tags | sort) == $want' || fail_partial "service readback failed"
  kong GET "/routes/$route_id"
  kong_ok --arg s "$service_id" --argjson want "$(jq '.route.tags | sort' <<<"$PLAN")" \
    '.service.id == $s and (.tags | sort) == $want' || fail_partial "route readback failed"
  for name in "${PLUGINS[@]}"; do
    id=${PLUGIN_IDS[$name]}
    kong GET "/plugins/$id"
    kong_ok --arg r "$route_id" --arg n "$name" \
      --argjson want "$(jq --arg n "$name" '.plugins[$n].tags' <<<"$PLAN")" '
      .name == $n and .route.id == $r and .service == null and .consumer == null and .protocols == ["http"]
      and (.tags | sort) == $want
      and ([.tags[] | select(test("^(env|plugin|route|consumer)--"))] == [])
    ' || fail_partial "$name plugin readback failed"
  done
}

prepare_out() {
  [[ ! -e $OUT/journal.tsv && ! -e $OUT/state.json ]] ||
    die "$OUT already holds a seed journal; refusing to overwrite"
  mkdir -p -- "$OUT"
  chmod 700 -- "$OUT"
  if git -C "$OUT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    die "$OUT is inside a Git work tree; choose another --out"
  fi
  JOURNAL=$OUT/journal.tsv
  : >"$JOURNAL"
}

cmd_setup() {
  check_context
  load_fixture
  resolve_target
  build_plan
  local existing
  existing=$(kc -n "$NAMESPACE" get routes.gateway.cp.ei.telekom.de "$ROUTE_NAME" --ignore-not-found -o name) ||
    die "cannot check for Route CR $NAMESPACE/$ROUTE_NAME"
  [[ -z $existing ]] || die "Route CR $NAMESPACE/$ROUTE_NAME already exists; refusing to continue"

  if [[ $RUN != true ]]; then
    log "preview only; nothing was changed. Rerun with --run to create the Kong entities."
    printf '# Kong payloads\n'
    jq . <<<"$PLAN"
    printf '\n# Route CR (written to %s/route.yaml by --run)\n' "$OUT"
    route_cr
    return
  fi

  prepare_out
  open_admin
  preflight_kong

  local service_id route_id name
  declare -gA PLUGIN_IDS=()
  create service /services "$(jq '.service' <<<"$PLAN")"
  service_id=$CREATED_ID
  create route /routes "$(jq --arg s "$service_id" '.route + {service: {id: $s}}' <<<"$PLAN")"
  route_id=$CREATED_ID
  for name in "${PLUGINS[@]}"; do
    create "$name" "/routes/$route_id/plugins" "$(jq --arg n "$name" '.plugins[$n]' <<<"$PLAN")"
    PLUGIN_IDS[$name]=$CREATED_ID
  done
  readback "$service_id" "$route_id"

  route_cr >"$OUT/route.yaml"
  jq -n --arg context "$CONTEXT" --arg env "$ENVIRONMENT" --arg zone "$ZONE" --arg ns "$NAMESPACE" \
    --arg name "$ROUTE_NAME" --arg path "$ROUTE_PATH" --arg gw "$GATEWAY_NAME" --arg host "$HOSTNAME_" \
    --arg iss "$ISSUER" --arg consumer "$CONSUMER" --arg backend "$BACKEND_URL" \
    --arg sid "$service_id" --arg rid "$route_id" \
    --arg acl "${PLUGIN_IDS[acl]}" --arg jwt "${PLUGIN_IDS[jwt-keycloak]}" --arg rtp "${PLUGIN_IDS[request-transformer]}" \
    --argjson plan "$PLAN" '{
      context: $context, environment: $env, zone: $zone, namespace: $ns, routeName: $name, path: $path,
      gatewayName: $gw, hostname: $host, issuer: $iss, consumer: $consumer, backendUrl: $backend,
      serviceId: $sid, routeId: $rid,
      plugins: {acl: {id: $acl, tags: $plan.plugins.acl.tags},
        "jwt-keycloak": {id: $jwt, tags: $plan.plugins["jwt-keycloak"].tags},
        "request-transformer": {id: $rtp, tags: $plan.plugins["request-transformer"].tags}},
      complete: true}' >"$OUT/state.json.tmp"
  mv -- "$OUT/state.json.tmp" "$OUT/state.json"

  local self_path
  self_path="$script_dir/$(basename "$0")"
  cat <<EOF
Seeded Kong service, route and plugins for $ROUTE_NAME. State: $OUT
Apply the Route CR yourself:
  KUBECONFIG=$KUBECONFIG_PATH kubectl --context $CONTEXT apply -f $OUT/route.yaml
Then verify the adoption:
  KUBECONFIG=$KUBECONFIG_PATH $self_path verify --context $CONTEXT --out $OUT
EOF
}

# --- verify ------------------------------------------------------------------

STATE=''
state() { jq -r "$1" <<<"$STATE"; }

wait_route_ready() {
  local ns name cr deadline=$((SECONDS + TIMEOUT)) interval=${LEGACY_SEED_POLL_INTERVAL:-5}
  ns=$(state .namespace)
  name=$(state .routeName)
  while :; do
    if cr=$(kc -n "$ns" get routes.gateway.cp.ei.telekom.de "$name" -o json 2>/dev/null) &&
      jq -e '.metadata.generation as $g | .status.conditions // [] |
        any(.type == "Ready" and .status == "True" and .reason == "RouteProcessed" and .observedGeneration == $g)
        and any(.type == "Processing" and .status == "False" and .observedGeneration == $g)' <<<"$cr" >/dev/null; then
      ROUTE_CR=$cr
      return
    fi
    if ((SECONDS >= deadline)); then
      log "last conditions: $(jq -c '{generation: .metadata.generation, conditions: [.status.conditions[]? | {type, status, reason, observedGeneration}]}' <<<"${cr:-null}" 2>/dev/null || echo unavailable)"
      die "Route CR $ns/$name not processed within ${TIMEOUT}s"
    fi
    sleep "$interval"
  done
}

# Collects every page of the plugins bound to the route into $WORK/plugins.json.
list_route_plugins() {
  local route_id=$1 offset='' page=0
  local -A seen=()
  echo '[]' >"$WORK/plugins.json"
  while :; do
    ((++page <= 100)) || die "too many plugin pages"
    kong GET "/routes/$route_id/plugins?size=100${offset:+&offset=$offset}"
    [[ $KONG_STATUS == 200 ]] || die "cannot list route plugins ($KONG_STATUS)"
    jq -e '.data | type == "array"' "$WORK/resp.json" >/dev/null || die "route plugin list has no data"
    jq -s '.[0] + .[1].data' "$WORK/plugins.json" "$WORK/resp.json" >"$WORK/plugins.next"
    mv -- "$WORK/plugins.next" "$WORK/plugins.json"
    offset=$(jq -r '.offset // empty' "$WORK/resp.json")
    [[ -n $offset ]] || return 0
    [[ -z ${seen[$offset]:-} ]] || die "route plugin list repeats an offset"
    seen[$offset]=1
    offset=$(jq -rn --arg o "$offset" '$o | @uri')
  done
}

# export_final RESULT: writes the Route CR, Kong service, route and all route
# plugins exactly as read during this verify to a new private file in TMPDIR.
# Plugin configs may hold sensitive settings; the file must stay private.
export_final() {
  local dir=${TMPDIR:-/tmp} file
  if git -C "$dir" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    warn "not exporting: $dir is inside a Git work tree"
    return
  fi
  file=$(mktemp --suffix=.json "$dir/legacy-route-seed-${CONTEXT//[^A-Za-z0-9_.-]/_}-$1.XXXXXX") || die "cannot create export file"
  jq -n --arg result "$1" --arg context "$CONTEXT" --arg at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --argjson cr "$ROUTE_CR" --slurpfile service "$WORK/service.json" --slurpfile route "$WORK/route.json" \
    --slurpfile plugins "$WORK/plugins.json" '{
      result: $result, exportedAt: $at, context: $context,
      routeCR: ($cr | del(.metadata.managedFields)),
      kongService: $service[0], kongRoute: $route[0], kongRoutePlugins: $plugins[0]}' >"$file" ||
    die "cannot write export $file"
  log "final configuration ($1) exported to $file; it may contain sensitive plugin settings, keep it private"
  EXPORT_FILE=$file
}

cmd_verify() {
  check_context
  [[ -r $OUT/state.json ]] || die "no seed state in $OUT"
  STATE=$(cat "$OUT/state.json")
  jq -e '.complete == true' <<<"$STATE" >/dev/null || die "seed in $OUT is not complete"
  [[ $(state .context) == "$CONTEXT" ]] || die "seed in $OUT was made for context $(state .context)"
  ENVIRONMENT=$(state .environment)
  ZONE=$(state .zone)

  wait_route_ready
  local problems
  problems=$(jq -r --argjson s "$STATE" '
    (.status.properties // {}) as $p
    | [["serviceId", $s.serviceId], ["routeId", $s.routeId], ["kongAclPluginId", $s.plugins.acl.id],
       ["kongJwtKeycloakPluginId", $s.plugins["jwt-keycloak"].id],
       ["kongRequestTransformerPluginId", $s.plugins["request-transformer"].id]][]
    | select($p[.[0]] != .[1]) | "status.properties.\(.[0]) is \($p[.[0]] // "missing"), seeded \(.[1])"
  ' <<<"$ROUTE_CR")
  [[ -z $problems ]] || die "Route CR did not adopt the seeded IDs:"$'\n'"$problems"
  log "Route CR is processed and reports the seeded IDs"

  # The ACL groups follow the current CR: its default consumers plus the
  # consumers of its ConsumeRoutes (status.consumers), or the deny-all
  # sentinel if there are none (access_control.go).
  jq -e '.spec.security.disableAccessControl != true' <<<"$ROUTE_CR" >/dev/null ||
    die "verify does not support disableAccessControl"
  ACL_ALLOW=$(jq -c '(.spec.security.defaultConsumers // []) + (.status.consumers // []) | unique
    | if . == [] then ["__deny_all__"] else . end' <<<"$ROUTE_CR")

  open_admin
  local name service_id route_id
  name=$(state .routeName)
  service_id=$(state .serviceId)
  route_id=$(state .routeId)
  kong GET "/services/$service_id"
  kong_ok --arg n "$name" '.name == $n' ||
    die "Kong service $service_id is missing or renamed ($KONG_STATUS)"
  cp -- "$WORK/resp.json" "$WORK/service.json"
  kong GET "/routes/$route_id"
  kong_ok --arg n "$name" --arg s "$service_id" \
    '.name == $n and .service.id == $s' ||
    die "Kong route $route_id is missing, renamed or not linked to service $service_id ($KONG_STATUS)"
  cp -- "$WORK/resp.json" "$WORK/route.json"

  list_route_plugins "$route_id"
  problems=$(jq -r --argjson s "$STATE" --arg rid "$route_id" --arg auth "$AUTH_HEADER" --arg jumper "$JUMPER_CONFIG" \
    --argjson allow "$ACL_ALLOW" '
    def owner_tags: [.[] | select(split("--")[0] | IN("env", "plugin", "route", "consumer"))] | unique;
    def has_all($xs): . as $have | all($xs[]; IN($have[]));
    def list: . // [] | sort;
    # The fields the operator manages, normalized; see plugin/acl.go, jwt.go,
    # rtp.go and the access_control, last_mile_security and custom_scopes features.
    def managed($n):
      if $n == "acl" then {allow: (.allow | list), deny: (.deny | list), hide_groups_header}
      elif $n == "jwt-keycloak" then {allowed_iss: (.allowed_iss | list), consumer_match, consumer_match_claim,
        consumer_match_claim_custom_id, consumer_match_ignore_not_found}
      else . as $c | reduce ("add", "append", "remove", "replace", "rename") as $k ({};
        .[$k] = ($c[$k] // {} | {headers: (.headers | list), body: (.body | list), querystring: (.querystring | list)}))
      end;
    def none: {headers: [], body: [], querystring: []};
    ($s.environment) as $env
    | {
        acl: {allow: $allow, deny: [], hide_groups_header: false},
        "jwt-keycloak": {allowed_iss: [$s.issuer], consumer_match: true, consumer_match_claim: "azp",
          consumer_match_claim_custom_id: true, consumer_match_ignore_not_found: false},
        "request-transformer": {
          add: (none | .headers = (["environment:\($env)", "realm:\($env)"] | sort)),
          append: (none | .headers = (["access_token_forwarding:false", "api_base_path:\($s.path)", $jumper,
            "remote_api_url:\($s.backendUrl)"] | sort)),
          remove: (none | .headers = ["consumer-token"]),
          replace: (none | .headers = (["Authorization:\($auth)", "environment:\($env)", "realm:\($env)"] | sort)),
          rename: none}
      } as $want
    | ($s.plugins | to_entries | map(.value.id) | sort) as $ids
    | (if (map(.id) | sort) != $ids then "route plugins \(map("\(.name)=\(.id)") | join(", ")) are not exactly the seeded \($ids | join(", "))" else empty end),
      (.[] | select(.name as $n | $s.plugins[$n] == null or $s.plugins[$n].id != .id) | "unexpected plugin \(.name) \(.id)"),
      (.[] | select(.enabled != true) | "plugin \(.name) is not enabled"),
      (.[] | select(.route.id != $rid or .service != null or .consumer != null) | "plugin \(.name) is not bound to the route only"),
      (.[] | select(.protocols != ["http"]) | "plugin \(.name) protocols changed to \(.protocols)"),
      (.[] | .name as $n | select($s.plugins[$n] != null) | select(.tags | has_all($s.plugins[$n].tags) | not)
        | "plugin \(.name) lost legacy tags: \(.tags)"),
      (.[] | .name as $n | select($s.plugins[$n] != null)
        | (["consumer--none", "env--\($env)", "plugin--\($n)", "route--\($s.routeName)"] | sort) as $owner
        | select((.tags | owner_tags) != $owner) | "plugin \(.name) ownership tags \(.tags | owner_tags) are not \($owner)"),
      (.[] | .name as $n | select($want[$n] != null) | (.config | managed($n)) as $got
        | ($want[$n] | keys[]) as $k | select($got[$k] != $want[$n][$k])
        | "plugin \($n) config \($k) is \($got[$k] | tojson), expected \($want[$n][$k] | tojson)")
  ' "$WORK/plugins.json")
  if [[ -n $problems ]]; then
    export_final failed-diagnostic
    die "Kong does not match the adopted seed:"$'\n'"$problems"
  fi
  export_final verified
  printf 'OK: %s adopted service %s, route %s and plugins %s\n' "$name" "$service_id" "$route_id" \
    "$(state '[.plugins | to_entries[] | "\(.key)=\(.value.id)"] | join(" ")')"
  printf 'Final configuration: %s\n' "$EXPORT_FILE"
}

case "$COMMAND" in
  setup) cmd_setup ;;
  verify) cmd_verify ;;
esac
