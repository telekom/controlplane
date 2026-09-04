---
sidebar_position: 3
---

# Failover

The Control Plane supports two failover mechanisms to ensure high availability across zones:

- **Provider Failover** — requires no additional platform configuration; providers simply declare a backup zone in their Rover file.
- **Consumer Failover (DTC)** — requires the platform administrator to enable the `ConsumerFailover` feature on participating zones.

This page covers the admin-side setup. For user-facing configuration, see the [Traffic Management: Failover](../../user-journey/features/traffic-management.mdx) guide.

## Provider Failover

Provider Failover works out of the box. When a provider declares a failover zone in their API exposure, the Control Plane creates the necessary **secondary routes** automatically. No additional zone configuration is required.

A secondary route is a backup copy of the provider's API placed in the failover zone. While the provider's primary zone is healthy, traffic flows to it as normal; if a health check finds the primary zone unavailable, the gateway redirects to the secondary route instead. For the underlying route model, see [Gateway Domain: Route Types & Cross-Zone Meshing](../../architecture/gateway.mdx#route-types--cross-zone-meshing).

## Consumer Failover (DTC)

Consumer Failover — also called **Dynamic Traffic Control (DTC)** — allows consumers to be transparently redirected to a different zone at the DNS level when their home zone is unavailable.

For this to work, participating zones must be explicitly configured with a dedicated gateway preset that carries the `ConsumerFailover` feature flag.

### How It Works

1. The administrator creates a **gateway preset** carrying the `ConsumerFailover` feature on each zone that should participate.
2. This preset defines the hostnames and paths that the zone's gateway uses when handling traffic from redirected consumers.
3. When a consumer enables failover on their subscription (`failover.enabled: true`), the system discovers all zones whose presets carry this feature and pre-configures access across them.

### Enabling Consumer Failover on a Zone

Add a preset with the `ConsumerFailover` feature enabled. This preset needs its own URL configuration that defines how redirected traffic reaches the zone:

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: Zone
metadata:
  name: aws-zone
  namespace: production
spec:
  gateways:
    - name: standard
      admin:
        identityProviderRef: primary
        url: https://gateway-admin.aws-zone.example.com
  presets:
    - name: default
      type: API
      default: true
      gatewayRef: standard
      identityProviderRef: primary
      urls:
        - hostname: api.aws-zone.example.com
          basePath: /
    - name: consumer-failover
      type: API
      default: false
      gatewayRef: standard
      identityProviderRef: primary
      urls:
        - hostname: failover.aws-zone.example.com
          basePath: /
      features:
        - name: ConsumerFailover
          enabled: true
  # ... rest of zone configuration
```

### Configuration Details

| Field | Description |
| ----- | ----------- |
| `presets[].name` | Must be a valid identifier. The preset that enables consumer failover is typically named `consumer-failover`. |
| `presets[].type` | The traffic kind this preset routes. Failover routing is implemented for `API` only; see the note below. |
| `presets[].gatewayRef` | Must reference a gateway declared in `spec.gateways`. |
| `presets[].features` | Must include `{name: "ConsumerFailover", enabled: true}` for the zone to participate in consumer failover. |
| `presets[].urls` | Defines the hostnames used by consumers redirected via DTC. These hostnames are added to all routes that need to accept failover traffic. |

:::note Failover routing is implemented for API only
The data model permits `ConsumerFailover` on a preset of any traffic type, and the admission
webhook accepts one on an `AI` or `Event` preset. The implementation is narrower:

- An **`API`** failover preset is fully supported — credentials, route enrichment, additional
  hostnames, trusted issuers and proxy routes.
- An **`AI`** failover preset provisions consumer credentials on its gateway but **no routing**.
  No exposure or subscription is enriched for it, because failover routing is API-specific.
- An **`Event`** failover preset provisions **nothing**. An Application carries no traffic kind
  and is provisioned only on `API` and `AI` gateways, so an `Event` failover preset yields no
  consumer and does not by itself make its zone a failover target.

This is a current implementation limit, not a rule of the model — AI and Event failover can be
added later without a schema change.
:::

A preset-scoped feature such as `ConsumerFailover` may be enabled on at most one preset **per traffic type** — the webhook rejects a zone that enables it twice for the same type, because selection could not then be single-valued. Enabling it once for `API` and once for `AI` is accepted by the webhook (with a warning), but only the `API` one produces failover routing.

### What Happens Automatically

Once the preset is configured:

- All API exposures that have subscribers with `failover.enabled: true` will be enriched with:
  - **Additional hostnames** from the consumer-failover preset (so the zone's gateway accepts traffic arriving on the failover hostname).
  - **Additional trusted identity providers** (so tokens issued by any participating zone's IDP are accepted).
- Proxy routes are created in this zone for APIs that have consumer-failover-enabled subscribers, even if no subscriber in this zone directly subscribes to those APIs.
- ConsumeRoutes are created to grant redirected consumers access to the appropriate routes.

### Verifying the Setup

Consumer failover is driven entirely by the zone spec. After applying the zone configuration, check that a preset carries the feature:

```bash
kubectl get zone aws-zone -n production -o jsonpath='{.spec.presets[*].features}'
```

The output should include:

```json
[{"name":"ConsumerFailover","enabled":true}]
```

### Removing Consumer Failover from a Zone

To stop a zone from participating in consumer failover, remove the `ConsumerFailover` feature from the preset (or remove the preset entirely). The Control Plane will automatically stop creating new failover routes and ConsumeRoutes for this zone.

:::caution
Removing the ConsumerFailover feature from a zone while consumers are actively relying on it may cause traffic disruption during DNS failover events. Coordinate with your teams before disabling this feature.
:::
