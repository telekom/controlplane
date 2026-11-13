<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# legacy-route-seed.sh

Tests whether the gateway operator adopts a Kong route that the legacy operator created. The script
creates a Kong service, a route and the `acl`, `jwt-keycloak` and `request-transformer` plugins with
the legacy tags. You then apply a matching `Route.gateway.cp.ei.telekom.de` CR yourself. The operator
should take over the existing plugins under their IDs instead of creating new ones.

The fixture is a legacy dump such as `<workspace>/legacy.yaml`. The script only reads it. It removes
ANSI colours, checks the plugin tags and bindings, and keeps only allowlisted fields. Plugin configs
are synthetic: hosts, issuers, ACL groups, the realm, the environment and the backend come from the
test environment, and the legacy `jumper_config` header is seeded as `{}`, never copied.

## Steps

```bash
export KUBECONFIG=<workspace>/config/ephemeral/clusters/pr-805/kubeconfig

# 1. Preview (read-only): validates the fixture, resolves the gateway and issuer
#    from the dp1 zone, and prints the Kong payloads and the Route CR.
gateway/hack/legacy-route-seed.sh setup --context pr-805

# 2. Seed Kong. Refuses if the Route CR, the Kong service or route, or a legacy
#    plugin already exists. Journals every created ID and writes route.yaml.
gateway/hack/legacy-route-seed.sh setup --context pr-805 --run

# 3. Apply the CR yourself (the script never does).
kubectl --context pr-805 apply -f /tmp/opencode/legacy-route-seed/pr-805/route.yaml

# 4. Verify (read-only): waits until the CR is processed, then checks Kong.
gateway/hack/legacy-route-seed.sh verify --context pr-805
```

`--context` is required and must exist in the kubeconfig. Use `--expect-image` to require a specific
gateway operator image during setup.

Verification passes when the following are all true:

- The Route CR reports `Ready=True` (reason `RouteProcessed`) and `Processing=False` for its current
  generation.
- Its `status.properties` hold the seeded service, route and plugin IDs.
- Every page of the route's plugin list contains exactly the three seeded plugins, still bound to
  the route only and with protocols `[http]`.
- The plugins still carry their legacy tags.
- The plugins are enabled, and their managed config (JWT issuer and consumer matching,
  all request-transformer headers including `jumper_config:e30=`) is exactly what the operator
  generates.
- The ACL allows exactly the CR's current `defaultConsumers` plus its ConsumeRoute consumers
  (`status.consumers`), so changing `defaultConsumers` and re-running `verify` is supported. All
  other expectations stay those of the seed.
- The ownership tags are exactly `env--`, `plugin--`, `route--` and `consumer--none` for this route.

## Final configuration export

`verify` writes the Route CR (without `managedFields`), the Kong service, the Kong route and every
route plugin, exactly as it read and checked them, to a new file
`$TMPDIR/legacy-route-seed-<context>-verified.XXXXXX.json` (mode 0600) and prints its path. If the
Kong checks fail, it writes the same data labelled `failed-diagnostic`. The export holds no admin
credentials or kubeconfig, but plugin configs may contain sensitive settings: keep the file private.

## Access and state

Kong is reached through the script's own `kubectl port-forward` to `svc/gateway-<zone>-admin` on a
loopback port that kubectl picks. No request is sent before kubectl reports that this tunnel listens,
and the script stops if the tunnel exits. The script authenticates as `admin` with the `gatewayAdminApiKey` from secret `<zone>/gateway-<zone>`.
The key is only written to a private curl config in a temporary directory, which is removed on exit.

The state directory (`--out`, default `/tmp/opencode/legacy-route-seed/<context>`) must be outside any
Git work tree. It contains:

- `journal.tsv`: created IDs, written immediately after each create.
- `state.json`: written only after the readback succeeds.
- `route.yaml`: the Route CR to apply.

If a create fails, the journal is kept and nothing is deleted. Clean up manually with the journaled
IDs.

Run the offline tests with `gateway/hack/legacy-route-seed_test.sh`.
