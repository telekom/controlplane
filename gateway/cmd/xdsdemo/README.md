<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
SPDX-License-Identifier: CC0-1.0
-->

# Durable xDS demo

The Compose stack runs these separate processes:

- `operator`: controller-runtime envtest API server and etcd, the real `GatewayReconciler`, and a demo-only HTTP control API.
- `management`: the existing durable xDS management command with SQLite on the `xds-data` volume.
- `envoy-a` and `envoy-b`: independent nodes mapped to the same Gateway target.
- `upstream`: dummy HTTP backend.
- `lms-issuer`: optional live JWT scenario's last-mile token issuer.

The operator creates a real `gateway.cp.ei.telekom.de/v1` `Gateway` with `spec.type: envoy`. Control API route operations create, update, and delete a real `Route`; only `GatewayReconciler` compiles and publishes bundles. The Kubernetes-assigned Gateway UID is handed to management through the `target-state` volume so node mappings retain the complete target identity.

## Run

From `gateway`:

```bash
make xdsdemo-up
make xdsdemo-validate
docker compose -f cmd/xdsdemo/docker-compose.yaml down
```

`xdsdemo-validate` proves:

- route create, update, and delete without restarting Envoy;
- invalid candidate rejection and idempotent republishing;
- independent ACK and NACK status for two nodes, including last-accepted routing and recovery;
- management restart and SQLite restoration;
- management and both Envoys restart while the operator is stopped.

The script intentionally leaves the operator stopped after the outage test. Use `make xdsdemo-up` to recreate the stack. Use `docker compose ... down -v` for a clean database.

Host ports can be overridden in `.env` with `CONTROL_PORT`, `MANAGEMENT_GRPC_PORT`, `MANAGEMENT_HEALTH_PORT`, `ENVOY_A_PORT`, `ENVOY_A_ADMIN_PORT`, `ENVOY_B_PORT`, and `ENVOY_B_ADMIN_PORT`. Compose and the validation script use the same values.

## Optional JWT scenario

Copy `.env.example` to `.env` and provide `CLIENT_SECRET`. The validation script then also updates the real Route with the live issuer and consumer, verifies JWT/JWKS and RBAC, and confirms LMS replaces the upstream Authorization token. No secret is built into an image or committed.

## Control API

The demo-only API listens on `http://localhost:18081`:

- `PUT /routes/demo` with `{"path":"/v1","host":"demo-route.local"}`
- `DELETE /routes/demo`
- `GET /status`
- `POST /publish/idempotent`
- `POST /publish/invalid`
- `POST /publish/nack`

The API is deliberately fixed to one demo Gateway, Route, and upstream. It does not accept arbitrary Kubernetes objects or xDS payloads.

Compose starts management only after the operator has replaced the target handoff file with the current Gateway UID. Node mappings are static after management startup; recreating the envtest operator separately therefore requires restarting management before publishing to the new target.
