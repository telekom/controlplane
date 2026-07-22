<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# xDS demo harness

Runs the Envoy feature builder end to end with no operator and no Kubernetes:
the demo builds an xDS snapshot for one fake `Route`, serves it over ADS, and a
real Envoy connects, fetches the config, and proxies to a dummy upstream.

## Run

The whole stack (operator/xDS server + Envoy + dummy upstream) runs in compose:

```bash
cd gateway/cmd/xdsdemo

# 1. Configure the client secret (gitignored):
cp .env.example .env        # then edit .env, set CLIENT_ID + CLIENT_SECRET

# 2. Build the operator and start the stack:
docker compose up -d --build

# 3. Run the validation pipeline (host):
./scripts/validate.sh
```

`validate.sh` asserts:

- **no token → 401** (jwt_authn rejects)
- **valid client_credentials token → 200** (JWT signature + issuer verified
  against the live JWKS, `azp` matched the RBAC allow-list, proxied upstream)

The allowed consumer must equal the token's `azp` claim (the client id). It is
sourced from `.env` `CLIENT_ID` and passed to the operator as `CONSUMERS`, so
`.env` is the single source of truth for both sides.

Manual check with a minted token:

```bash
set -a; source .env; set +a
TOKEN=$(curl -s -X POST "$TOKEN_URL" \
  -d grant_type=client_credentials \
  -d "client_id=$CLIENT_ID" -d "client_secret=$CLIENT_SECRET" | jq -r .access_token)

curl -H "Authorization: Bearer $TOKEN" http://localhost:10000/get   # 200
curl http://localhost:10000/get                                     # 401
```

A valid token whose `azp` is not in the allow-list gets **403** (RBAC deny).

Envoy admin (config dump, clusters, stats): http://localhost:9901

## Configuration

Operator flags (also settable via env for the compose service):

- `-issuers` / `ISSUERS` — comma-separated trusted issuers. Default: the rover
  realm. Empty disables JWT/RBAC entirely (plain proxy).
- `-consumers` / `CONSUMERS` — comma-separated allowed consumers matched against
  the token `azp` claim. Compose sets this from `.env` `CLIENT_ID`.
- `-upstream` — upstream URL Envoy proxies to. Default: `http://upstream:80`.

## Notes

- JWKS is fetched live via `remote_jwks` from `{issuer}/protocol/openid-connect/certs`
  (Keycloak convention) through a per-issuer-host TLS cluster (`jwks_<host>`,
  port 443, default public-CA trust). **Envoy must have network egress to the
  issuer host** (VPN as needed).
- The consumer identity is the token `azp` claim; for a `client_credentials`
  token that is the full client id (e.g. `eni--system--dev-luminary`), which is
  why `CONSUMERS` must equal `CLIENT_ID`.
- Single node id `poc-gateway-node` (`envoy.PocNodeID`); the bootstrap `node.id`
  must match it.
