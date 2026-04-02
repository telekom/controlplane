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

All operators produce structured JSON logs. Key log fields include the resource kind, namespace, name, and reconciliation result. Use these logs to trace the lifecycle of any resource through the system.

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
