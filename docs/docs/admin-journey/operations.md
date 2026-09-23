---
sidebar_position: 7
---

# Operations & Monitoring

Once the Control Plane is up and running, this page covers the day-to-day operational tasks and observability tools available to platform administrators.

## Observability

All Control Plane operators and services expose metrics and structured logs following Kubernetes-native conventions.

### Metrics

Each operator exposes Prometheus-compatible metrics on its metrics endpoint. Key metrics to monitor include:

- **Reconciliation duration** — How long each reconciliation cycle takes
- **Reconciliation errors** — Failed reconciliation attempts
- **Queue depth** — Number of resources waiting to be reconciled
- **Resource counts** — Total number of managed resources per type

### Structured Logs

All operators produce structured JSON logs. Log fields vary by component and event; use the available structured context to trace resource lifecycle and reconciliation activity.

The shared controller logs a full resource object for successful create/update and deletion reconciliations at debug verbosity (`V(1)`). Those log entries omit `metadata.managedFields`. This Kubernetes bookkeeping metadata tracks which field manager last touched each field; it is primarily useful to the API server and adds noise to logs without aiding troubleshooting. Only the log-facing copy is sanitized — the underlying object used for reconciliation and persistence is unaffected.

This sanitization is enabled by default and can be disabled, for example when troubleshooting field-manager conflicts, by setting `DISABLE_MANAGED_FIELDS_SANITIZATION` to a true boolean value:

```yaml
env:
  - name: DISABLE_MANAGED_FIELDS_SANITIZATION
    value: "true"
```

Boolean values are parsed using Go's standard boolean syntax (for example, `true`, `1`, or `t`). An unset, empty, false, or invalid value keeps sanitization enabled.

#### Configuring log verbosity

Each operator's log level can be configured via the `LOG_LEVEL` environment variable, without requiring a code change or rebuild. Supported values are the standard zap level names (case-insensitive): `debug`, `info`, `warn`, `error` (and other zap levels such as `dpanic`, `panic`, `fatal`). If `LOG_LEVEL` is unset, empty, or set to an unrecognized value, the operator defaults to `info`. Set it to `debug` to emit the resource object logs described above.

```yaml
env:
  - name: LOG_LEVEL
    value: "debug"
```

Log output defaults to JSON regardless of the configured level; `LOG_LEVEL` only affects verbosity, not the log format. The controller-runtime `--zap-log-level` flag remains available and overrides the environment-derived level when explicitly provided.

## Operational Tools

The Control Plane includes several tools to assist with day-to-day operations:

| Tool | Purpose |
| ---- | ------- |
| **Snapshotter** | Captures, compares, and manages snapshots of the API gateway state (routes, consumers). Useful for auditing and CI/CD validation. |
| **Route Tester** | Tests whether a gateway route is correctly configured and reachable. Automates the manual `curl`-based verification process. |
| **E2E Tester** | Runs comprehensive end-to-end tests validating Rover-CTL commands. Designed for CI/CD pipelines. |

## Secret Rotation

The Secret Manager supports secret rotation for team credentials and environment-level secrets. When a rotation occurs:

1. The Secret Manager updates the stored secret.
2. The Identity domain provisions the new credentials in the identity provider.
3. The Organization domain triggers a notification to the affected team.

## Scaling Considerations

The Control Plane is designed to run in a single Kubernetes cluster, with operators managing resources across multiple environments and zones. Key scaling factors:

- **Number of environments and zones** — Each zone creates additional namespaces and gateway/identity resources.
- **Number of teams** — Each team provisions a namespace, identity client, and gateway consumer.
- **Number of APIs and subscriptions** — Each subscription creates gateway routes and consume-routes.

## Next Steps

- [Architecture Overview](../architecture/overview.md) — Understand the overall system design
- [Components](../overview/components.md) — Review all platform components
