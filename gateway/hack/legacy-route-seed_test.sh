#!/usr/bin/env bash

# Copyright 2026 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Offline tests for legacy-route-seed.sh. kubectl and curl are replaced by
# stubs; the curl stub emulates the few Kong admin endpoints the script uses.

# Commands passed to bash -c and jq filters are single-quoted on purpose.
# shellcheck disable=SC2016

set -euo pipefail

script=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/legacy-route-seed.sh
root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT

secret='s3cr3t"x\y'
export SECRET=$secret
export EXPECTED_CFG_LINE='user = "admin:s3cr3t\"x\\y"'
export LEGACY_SEED_POLL_INTERVAL=0
# Stands in for sensitive Jumper config that must never leave the fixture.
fixture_marker=FIXTURE-PRIVATE-MARKER
failures=0

# --- stubs -------------------------------------------------------------------

mkdir -p "$root/bin"
cat >"$root/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$S/kubectl.log"
case " $* " in
  *" config get-contexts "*) printf 'other\npr-805\n' ;;
  *" apply "* | *" create "* | *" delete "* | *" patch "* | *" replace "*) exit 99 ;;
  *" port-forward "*)
    [[ " $* " == *" --address 127.0.0.1 "*" :8001 "* ]] || { echo "unsafe port-forward" >&2; exit 97; }
    echo $$ >"$S/pf.pid"
    if [[ -e $S/pf-fail ]]; then echo 'error: unable to listen on any of the requested ports' >&2; exit 1; fi
    echo 'Forwarding from 127.0.0.1:43123 -> 8001'
    exec sleep 30 ;;
  *" get zones.admin.cp.ei.telekom.de "*) cat "$S/zone.json" ;;
  *" get gateways.gateway.cp.ei.telekom.de "*) echo '{"status":{"conditions":[{"type":"Ready","status":"True"}]}}' ;;
  *" get deployment "*) printf 'registry.test/gateway:fix' ;;
  *" get secret "*) printf '%s' "$SECRET" | base64 -w0 ;;
  *" get routes.gateway.cp.ei.telekom.de "*"--ignore-not-found"*) cat "$S/route-exists" 2>/dev/null || true ;;
  *" get routes.gateway.cp.ei.telekom.de "*) cat "$S/route.json" ;;
  *) echo "unexpected kubectl $*" >&2; exit 98 ;;
esac
EOF

cat >"$root/bin/curl" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$S/curl.log"
[[ $1 == -q ]] || { echo "curl stub: -q must be first" >&2; exit 2; }
method=GET out='' data='' cfg='' url=''
while [[ $# -gt 0 ]]; do
  case $1 in
    -X) method=$2; shift 2 ;;
    -o) out=$2; shift 2 ;;
    --config) cfg=$2; shift 2 ;;
    --data-binary) data=${2#@}; shift 2 ;;
    -w | -H | --connect-timeout | --max-time | --noproxy) shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
respond() {
  printf '%s' "$2" >"$out"
  printf '%s' "$1"
  # Simulates a tunnel that dies after the first request.
  if [[ -e $S/kill-tunnel ]]; then rm "$S/kill-tunnel"; kill "$(cat "$S/pf.pid")"; sleep 0.5; fi
}
[[ $url == http://127.0.0.1:43123/* ]] || { echo "curl stub: not the owned tunnel: $url" >&2; exit 7; }
grep -Fqx -- "$EXPECTED_CFG_LINE" "$cfg" || { respond 401 '{"message":"Unauthorized"}'; exit 0; }
path=/${url#*://*/}
path=${path%%\?*}
query=${url#*\?}; [[ $query == "$url" ]] && query=''
find_entity() {
  local f
  for f in "$S"/kong/"$1"/*.json; do
    [[ -e $f ]] || continue
    jq -e --arg k "$2" '.id == $k or .name == $k' "$f" >/dev/null && { cat "$f"; return 0; }
  done
  return 1
}
if [[ $method == POST ]]; then
  n=$(( $(cat "$S/posts" 2>/dev/null || echo 0) + 1 )); echo "$n" >"$S/posts"
  cp "$data" "$S/post-$n.json"
  [[ $n != "${FAIL_POST:-0}" ]] || { respond 500 '{"message":"boom"}'; exit 0; }
fi
case "$method $path" in
  "GET /")
    respond 200 '{"plugins":{"available_on_server":{"acl":true,"jwt-keycloak":true,"request-transformer":true}}}' ;;
  "GET /plugins")
    if [[ -e $S/legacy-plugin ]]; then respond 200 '{"data":[{"id":"x"}]}'; else respond 200 '{"data":[]}'; fi ;;
  "GET /routes/"*/plugins)
    offset=$(sed -n 's/.*offset=\([^&]*\).*/\1/p' <<<"$query")
    respond 200 "$(cat "$S/pages/${offset:-first}.json")" ;;
  "GET /services/"* | "GET /routes/"* | "GET /plugins/"*)
    kind=${path#/}; kind=${kind%%/*}
    if body=$(find_entity "$kind" "${path##*/}"); then respond 200 "$body"; else respond 404 '{"message":"Not found"}'; fi ;;
  "POST /services")
    mkdir -p "$S/kong/services"; jq '.id = "service-id"' "$data" >"$S/kong/services/service-id.json"
    respond 201 "$(cat "$S/kong/services/service-id.json")" ;;
  "POST /routes")
    mkdir -p "$S/kong/routes"; jq '.id = "route-id"' "$data" >"$S/kong/routes/route-id.json"
    respond 201 "$(cat "$S/kong/routes/route-id.json")" ;;
  "POST /routes/"*/plugins)
    rid=${path#/routes/}; rid=${rid%/plugins}
    mkdir -p "$S/kong/plugins"; id="$(jq -r .name "$data")-id"
    jq --arg id "$id" --arg r "$rid" '.id = $id | .route = {id: $r} | .service = null | .consumer = null' "$data" >"$S/kong/plugins/$id.json"
    respond 201 "$(cat "$S/kong/plugins/$id.json")" ;;
  *) respond 405 '{"message":"unexpected"}' ;;
esac
EOF
chmod +x "$root/bin/kubectl" "$root/bin/curl"

# --- helpers -----------------------------------------------------------------

esc=$'\e'
fixture=$root/legacy.yaml
cat >"$fixture" <<EOF
${esc}[32mservice:
  host: localhost
  id: 00000000-0000-0000-0000-000
  name: demo-echo-v1
  path: /proxy
  port: 8080
  protocol: http
  retries: 5
  tags:
  - ei__telekom__de--realm---default
  - ei__telekom__de--team---internal
route:
  hosts:
  - legacy.example.test
  id: 00000000-0000-0000-0000-000
  name: demo-echo-v1
  paths:
  - /demo/echo/v1
  protocols:
  - http
  - https
  strip_path: true
  service:
    id: 00000000-0000-0000-0000-000
plugins:
- config:
    allow:
    - legacy-group
    hide_groups_header: false
  id: 00000000-0000-0000-0000-000
  name: acl
  protocols:
  - http
  route:
    id: 00000000-0000-0000-0000-000
  tags:
  - acl
  - ei__telekom__de--realm---default
  - demo-echo-v1
- config:
    allowed_iss:
    - https://legacy-issuer.example.test/auth/realms/default
    consumer_match: true
    consumer_match_claim: azp
    run_on_preflight: true
  id: 00000000-0000-0000-0000-000
  name: jwt-keycloak
  protocols:
  - http
  route:
    id: 00000000-0000-0000-0000-000
  tags:
  - ei__telekom__de--realm---default
  - demo-echo-v1
  - jwt-keycloak
- config:
    append:
      headers:
      - api_base_path:/demo/echo/v1
      - environment:legacy
      - jumper_config:$fixture_marker
      - realm:default
      - remote_api_url:https://legacy-backend.example.test/
  id: 00000000-0000-0000-0000-000
  name: request-transformer
  protocols:
  - http
  route:
    id: 00000000-0000-0000-0000-000
  tags:
  - ei__telekom__de--realm---default
  - demo-echo-v1
  - proxy_route
  - request-transformer
${esc}[0m
EOF

# new_case NAME: fresh stub state; the script runs with $S/out as --out.
new_case() {
  CASE=$1
  export S=$root/$1
  mkdir -p "$S"
  cat >"$S/zone.json" <<'EOF'
{"status":{"presets":[{"name":"default","gatewayRef":{"name":"dp1-standard","namespace":"pr-805--dp1"},
 "links":{"gatewayIssuer":"https://iris.example.test/auth/realms/pr-805","gatewayUrl":"https://gateway-dp1-pr-805.example.test/"}}]}}
EOF
}

# run ARGS...: runs the script; sets RC, $S/stdout and $S/stderr.
run() {
  set +e
  mkdir -p "$S/tmp"
  PATH="$root/bin:$PATH" KUBECONFIG=$root/kubeconfig TMPDIR=$S/tmp "$script" "$@" --fixture "$fixture" >"$S/stdout" 2>"$S/stderr"
  RC=$?
  set -e
}

check() {
  local message=$1
  shift
  if "$@"; then
    printf 'ok   %s: %s\n' "$CASE" "$message"
  else
    printf 'FAIL %s: %s\n' "$CASE" "$message"
    sed 's/^/     | /' "$S/stderr" 2>/dev/null | tail -5
    failures=$((failures + 1))
  fi
}

no_mutation() { ! grep -q POST "$S/curl.log" 2>/dev/null && [[ ! -e $S/posts ]]; }
never_applied() { ! grep -Eq '(^| )(apply|create|delete|patch|replace)( |$)' "$S/kubectl.log"; }
contains() { grep -Fq -- "$2" "$1"; }
json() { jq -e "$@" >/dev/null; }

# --- setup -------------------------------------------------------------------

new_case preview
run setup --context pr-805 --out "$S/out"
check "succeeds" test "$RC" = 0
check "makes no Kong call" test ! -e "$S/curl.log"
check "creates no state" test ! -e "$S/out"
check "never applies" never_applied
check "substitutes the issuer" contains "$S/stdout" 'https://iris.example.test/auth/realms/pr-805'
check "substitutes the backend" contains "$S/stdout" 'remote_api_url:http://cosmoparrot.backend.svc.cluster.local:8080/'
check "substitutes the ACL group" contains "$S/stdout" '"pr-805-legacy-test"'
check "drops fixture values" bash -c '! grep -Eq "FIXTURE-PRIVATE-MARKER|legacy-group|legacy-issuer|legacy-backend|legacy.example|environment:legacy|team---internal|run_on_preflight" "$1"' _ "$S/stdout"
check "prints the Route CR" contains "$S/stdout" 'namespace: pr-805--dp1'

new_case no-context
run setup
check "requires --context" test "$RC" != 0
run setup --context missing
check "rejects an unknown context" bash -c '[[ $1 != 0 ]] && grep -q "not found" "$2"' _ "$RC" "$S/stderr"

new_case bad-fixture
good=$fixture
fixture=$S/legacy.yaml
sed 's/^  - proxy_route$/  - other/' "$good" >"$fixture"
run setup --context pr-805 --out "$S/out"
check "rejects plugin tags that are not the legacy tags" bash -c '[[ $1 != 0 ]] && grep -q "request-transformer tags" "$2"' _ "$RC" "$S/stderr"
fixture=$good

new_case cr-exists
echo route.gateway.cp.ei.telekom.de/demo-echo-v1 >"$S/route-exists"
run setup --context pr-805 --run --out "$S/out"
check "refuses an existing Route CR" bash -c '[[ $1 != 0 ]] && grep -q "already exists" "$2"' _ "$RC" "$S/stderr"
check "makes no Kong call" test ! -e "$S/curl.log"

new_case service-exists
mkdir -p "$S/kong/services"
echo '{"id":"other","name":"demo-echo-v1"}' >"$S/kong/services/other.json"
run setup --context pr-805 --run --out "$S/out"
check "refuses an existing Kong service" bash -c '[[ $1 != 0 ]] && grep -q "service demo-echo-v1 already exists" "$2"' _ "$RC" "$S/stderr"
check "writes nothing" no_mutation

new_case legacy-plugin-exists
touch "$S/legacy-plugin"
run setup --context pr-805 --run --out "$S/out"
check "refuses an existing legacy plugin" bash -c '[[ $1 != 0 ]] && grep -q "legacy acl plugin" "$2"' _ "$RC" "$S/stderr"
check "writes nothing" no_mutation

new_case partial
FAIL_POST=3 run setup --context pr-805 --run --out "$S/out"
check "fails" test "$RC" != 0
check "journals the created IDs" bash -c '[[ $(cut -f1 "$1" | tr "\n" " ") == "service route " ]]' _ "$S/out/journal.tsv"
check "warns about the partial seed" contains "$S/stderr" 'nothing was deleted'
check "writes no state" test ! -e "$S/out/state.json"
check "deletes nothing" bash -c '! grep -q DELETE "$1"' _ "$S/curl.log"

new_case wrong-image
run setup --context pr-805 --run --out "$S/out" --expect-image registry.test/gateway:old
check "rejects an unexpected operator image" bash -c '[[ $1 != 0 ]] && grep -q "expected" "$2"' _ "$RC" "$S/stderr"
check "writes nothing" no_mutation

new_case tunnel-fails
touch "$S/pf-fail"
run setup --context pr-805 --run --out "$S/out"
check "fails when the port-forward cannot bind" bash -c '[[ $1 != 0 ]] && grep -q "port-forward to svc/gateway-dp1-admin failed" "$2"' _ "$RC" "$S/stderr"
check "sends no request" test ! -e "$S/curl.log"

new_case tunnel-dies
touch "$S/kill-tunnel"
run setup --context pr-805 --run --out "$S/out"
check "fails when the port-forward exits" bash -c '[[ $1 != 0 ]] && grep -q "port-forward is not running" "$2"' _ "$RC" "$S/stderr"
check "sends only the readiness request" test "$(wc -l <"$S/curl.log")" = 1
check "writes nothing" no_mutation

new_case run
run setup --context pr-805 --run --out "$S/out" --expect-image registry.test/gateway:fix
check "succeeds" test "$RC" = 0
check "stops its port-forward" bash -c '! kill -0 "$(cat "$1")" 2>/dev/null' _ "$S/pf.pid"
check "never sends the fixture's Jumper config" bash -c '! grep -rF FIXTURE-PRIVATE-MARKER "$@"' _ "$S"/post-*.json "$S/stdout" "$S/stderr" "$S/out"
check "seeds a synthetic Jumper config" json '.config.append.headers | index("jumper_config:{}")' "$S/post-5.json"
check "journals five IDs" test "$(wc -l <"$S/out/journal.tsv")" = 5
check "marks the state complete" json '.complete and .serviceId == "service-id" and .routeId == "route-id"
  and .plugins.acl.id == "acl-id"' "$S/out/state.json"
check "keeps the state private" test "$(stat -c %a "$S/out")" = 700
check "writes the Route CR" contains "$S/out/route.yaml" 'kind: Route'
check "prints the apply command" contains "$S/stdout" "apply -f $S/out/route.yaml"
check "never applies" never_applied
check "seeds exact legacy plugin tags" json '.tags == ["demo-echo-v1", "ei__telekom__de--realm---default", "proxy_route", "request-transformer"]
  and .protocols == ["http"]' "$S/kong/plugins/request-transformer-id.json"
check "links the route to the service" json '.service.id == "service-id"' "$S/post-2.json"
check "hides the secret" bash -c '! grep -rF -- "s3cr3t" "$@"' _ "$S/stdout" "$S/stderr" "$S/curl.log" "$S/kubectl.log" "$S/out"
check "disables the ambient curl config" bash -c '! grep -v "^-q " "$1"' _ "$S/curl.log"
run setup --context pr-805 --run --out "$S/out"
check "refuses to reuse a journal" bash -c '[[ $1 != 0 ]] && grep -q "refusing to overwrite" "$2"' _ "$RC" "$S/stderr"

# --- verify ------------------------------------------------------------------

seeded=$S

# adopt GENERATION OBSERVED: emulates the operator on top of the seeded state.
adopt() {
  local f
  cp -r "$seeded/out" "$seeded/kong" "$S/"
  for f in "$S"/kong/plugins/*.json; do
    jq '.tags += ["env--pr-805", "plugin--\(.name)", "route--demo-echo-v1", "consumer--none"]
      | .enabled = true
      | if .name == "acl" then .config = {allow: ["pr-805-legacy-test"], deny: null, hide_groups_header: false}
        elif .name == "jwt-keycloak" then .config = {allowed_iss: ["https://iris.example.test/auth/realms/pr-805"],
          consumer_match: true, consumer_match_claim: "azp", consumer_match_claim_custom_id: true,
          consumer_match_ignore_not_found: false, run_on_preflight: true}
        else .config = {
          add: {headers: ["environment:pr-805", "realm:pr-805"], body: [], querystring: []},
          append: {headers: ["access_token_forwarding:false", "api_base_path:/demo/echo/v1", "jumper_config:e30=",
            "remote_api_url:http://cosmoparrot.backend.svc.cluster.local:8080/"], body: [], querystring: []},
          remove: {headers: ["consumer-token"], body: [], querystring: []},
          replace: {headers: ["Authorization:$(headers['"'"'consumer-token'"'"'] or headers['"'"'Authorization'"'"'])",
            "environment:pr-805", "realm:pr-805"], body: [], querystring: [], uri: null},
          rename: {headers: [], body: [], querystring: []}, http_method: null} end' "$f" >"$f.new"
    mv "$f.new" "$f"
  done
  mkdir -p "$S/pages"
  jq -s '{data: .[0:2], offset: "page 2"}' "$S/kong/plugins/acl-id.json" "$S/kong/plugins/jwt-keycloak-id.json" >"$S/pages/first.json"
  jq -s '{data: ., offset: null}' "$S/kong/plugins/request-transformer-id.json" >"$S/pages/page%202.json"
  jq -n --argjson g "$1" --argjson o "$2" '{metadata: {generation: $g, managedFields: [{manager: "kubectl"}]},
    spec: {security: {realmName: "pr-805", disableAccessControl: false, defaultConsumers: ["pr-805-legacy-test"]}},
    status: {
    conditions: [{type: "Ready", status: "True", reason: "RouteProcessed", observedGeneration: $o},
      {type: "Processing", status: "False", reason: "Done", observedGeneration: $o}],
    properties: {serviceId: "service-id", routeId: "route-id", kongAclPluginId: "acl-id",
      kongJwtKeycloakPluginId: "jwt-keycloak-id", kongRequestTransformerPluginId: "request-transformer-id"}}}' >"$S/route.json"
}

# mutate NAME FILTER: changes one adopted plugin in whichever page holds it.
mutate() {
  local f
  for f in "$S"/pages/*.json; do
    jq --arg n "$1" ".data |= map(if .name == \$n then $2 else . end)" "$f" >"$f.new"
    mv "$f.new" "$f"
  done
}

new_case verify-adopted
adopt 2 2
run verify --context pr-805 --out "$S/out" --timeout 0
check "accepts the adoption" bash -c '[[ $1 == 0 ]] && grep -q "^OK:" "$2"' _ "$RC" "$S/stdout"
check "reads every page" contains "$S/curl.log" 'offset=page%202'
check "never applies" never_applied
check "makes no write" bash -c '! grep -Eq -- "-X (POST|PUT|PATCH|DELETE)" "$1"' _ "$S/curl.log"
export_file=$(sed -n 's/^Final configuration: //p' "$S/stdout")
check "prints the export path" test -f "$export_file"
check "exports to a private file" test "$(stat -c %a "$export_file")" = 600
check "exports the verified Kong route, service, all plugins and the CR" json '.result == "verified"
  and .kongService.id == "service-id" and .kongRoute.id == "route-id" and .kongRoute.service.id == "service-id"
  and ([.kongRoutePlugins[].id] | sort) == ["acl-id", "jwt-keycloak-id", "request-transformer-id"]
  and (.kongRoutePlugins[] | select(.name == "request-transformer") | .config.append.headers | index("jumper_config:e30="))
  and .routeCR.status.properties.routeId == "route-id" and .routeCR.metadata.managedFields == null' "$export_file"
check "exports no credentials" bash -c '! grep -Eq "s3cr3t|user = |kubeconfig" "$1"' _ "$export_file"
check "leaves the baseline untouched" diff -r "$seeded/out" "$S/out"
run verify --context pr-805 --out "$S/out" --timeout 0
check "never overwrites an export" test "$(find "$S/tmp" -name '*verified*.json' | wc -l)" = 2

# consumers CASE DEFAULTS STATUS_CONSUMERS ACL_ALLOW: a later CR generation
# with other consumers that the operator applied to the same ACL plugin.
consumers() {
  new_case "$1"
  adopt 3 3
  jq --argjson d "$2" --argjson c "$3" '.spec.security.defaultConsumers = $d | .status.consumers = $c' "$S/route.json" >"$S/r" &&
    mv "$S/r" "$S/route.json"
  mutate acl ".config.allow = $4"
  run verify --context pr-805 --out "$S/out" --timeout 0
}

consumers verify-two-consumers '["pr-805-legacy-test", "pr-805-legacy-test-updated"]' '[]' '["pr-805-legacy-test-updated", "pr-805-legacy-test"]'
check "accepts updated default consumers on the same plugin" bash -c '[[ $1 == 0 ]] && grep -q "acl=acl-id" "$2"' _ "$RC" "$S/stdout"
consumers verify-consume-route '["pr-805-legacy-test"]' '["other-consumer"]' '["other-consumer", "pr-805-legacy-test"]'
check "accepts ConsumeRoute consumers" test "$RC" = 0
consumers verify-missing-consumer '["pr-805-legacy-test", "pr-805-legacy-test-updated"]' '[]' '["pr-805-legacy-test"]'
check "rejects an ACL that lacks a CR consumer" bash -c '[[ $1 != 0 ]] && grep -q "config allow" "$2"' _ "$RC" "$S/stderr"
check "exports a labelled diagnostic" json '.result == "failed-diagnostic"' "$(find "$S/tmp" -name '*failed-diagnostic*.json')"
consumers verify-extra-consumer '["pr-805-legacy-test"]' '[]' '["pr-805-legacy-test", "pr-805-legacy-test-updated"]'
check "rejects an ACL with a consumer the CR lacks" bash -c '[[ $1 != 0 ]] && grep -q "config allow" "$2"' _ "$RC" "$S/stderr"

new_case verify-stale
adopt 2 1
run verify --context pr-805 --out "$S/out" --timeout 0
check "rejects a stale Ready condition" bash -c '[[ $1 != 0 ]] && grep -q "not processed" "$2"' _ "$RC" "$S/stderr"

new_case verify-replaced-id
adopt 1 1
jq '.status.properties.kongAclPluginId = "new-acl-id"' "$S/route.json" >"$S/route.new" && mv "$S/route.new" "$S/route.json"
run verify --context pr-805 --out "$S/out" --timeout 0
check "rejects a replaced plugin ID" bash -c '[[ $1 != 0 ]] && grep -q "kongAclPluginId is new-acl-id" "$2"' _ "$RC" "$S/stderr"

new_case verify-duplicate
adopt 1 1
jq --slurpfile acl "$S/kong/plugins/acl-id.json" '.data += [$acl[0] | .id = "dup-acl-id"]' "$S/pages/page%202.json" >"$S/p" && mv "$S/p" "$S/pages/page%202.json"
run verify --context pr-805 --out "$S/out" --timeout 0
check "rejects a duplicate on a later page" bash -c '[[ $1 != 0 ]] && grep -q "unexpected plugin acl dup-acl-id" "$2"' _ "$RC" "$S/stderr"

# rejects CASE MESSAGE NAME FILTER: verify must fail with MESSAGE.
rejects() {
  new_case "$1"
  adopt 1 1
  mutate "$3" "$4"
  run verify --context pr-805 --out "$S/out" --timeout 0
  check "rejects: $2" bash -c '[[ $1 != 0 ]] && grep -Fq -- "$3" "$2"' _ "$RC" "$S/stderr" "$2"
}

rejects verify-disabled "is not enabled" acl '.enabled = false'
rejects verify-missing-go-tags "ownership tags" acl '.tags -= ["env--pr-805"]'
rejects verify-conflicting-tag "ownership tags" jwt-keycloak '.tags += ["route--other"]'
rejects verify-lost-legacy-tag "lost legacy tags" acl '.tags -= ["demo-echo-v1"]'
rejects verify-acl-allow "config allow" acl '.config.allow += ["other"]'
rejects verify-jwt-issuer "config allowed_iss" jwt-keycloak '.config.allowed_iss = ["https://other.example.test"]'
rejects verify-jwt-claim "config consumer_match_claim" jwt-keycloak '.config.consumer_match_claim = "sub"'
rejects verify-jwt-match "config consumer_match_ignore_not_found" jwt-keycloak '.config.consumer_match_ignore_not_found = true'
rejects verify-rtp-remove "config remove" request-transformer '.config.remove.headers = []'
rejects verify-rtp-authorization "config replace" request-transformer '.config.replace.headers -= [.config.replace.headers[0]]'
rejects verify-rtp-realm "config add" request-transformer '.config.add.headers = ["environment:pr-805", "realm:default"]'
rejects verify-rtp-stale-backend "config append" request-transformer '.config.append.headers += ["remote_api_url:https://legacy.example.test/"]'
rejects verify-rtp-legacy-jumper "config append" request-transformer '.config.append.headers |= map(if startswith("jumper_config:") then "jumper_config:{}" else . end)'
rejects verify-rtp-malformed-jumper "config append" request-transformer '.config.append.headers |= map(if startswith("jumper_config:") then "jumper_config:%%%" else . end)'
rejects verify-rtp-rename "config rename" request-transformer '.config.rename.headers = ["a:b"]'

if ((failures > 0)); then
  echo "$failures check(s) failed"
  exit 1
fi
echo "all checks passed"
