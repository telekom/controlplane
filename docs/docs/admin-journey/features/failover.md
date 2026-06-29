---
sidebar_position: 3
---

# Failover

The Control Plane supports two failover mechanisms to ensure high availability across zones:

- **Provider Failover** — requires no additional platform configuration; providers simply declare a backup zone in their Rover file.
- **Consumer Failover (DTC)** — requires the platform administrator to enable the `ConsumerFailover` feature on participating zones.

This page covers the admin-side setup. For user-facing configuration, see the [Traffic Management: Failover](../../user-journey/features/traffic-management.mdx) guide.

## Provider Failover

Provider Failover works out of the box. When a provider declares a failover zone in their API exposure, the Control Plane creates the necessary secondary routes automatically. No additional zone configuration is required.

## Consumer Failover (DTC)

Consumer Failover — also called **Data Traffic Control (DTC)** — allows consumers to be transparently redirected to a different zone at the DNS level when their home zone is unavailable.

For this to work, participating zones must be explicitly configured with a dedicated gateway preset that carries the `ConsumerFailover` feature flag.

### How It Works

1. The administrator creates a **gateway preset** named `ConsumerFailover` on each zone that should participate.
2. This preset defines the hostnames and paths that the zone's gateway uses when handling traffic from redirected consumers.
3. The Control Plane automatically detects this preset and enables the `ConsumerFailover` feature on the zone's status.
4. When a consumer enables failover on their subscription (`failover.enabled: true`), the system discovers all zones with this feature and pre-configures access across them.

### Enabling Consumer Failover on a Zone

Add a gateway preset named `ConsumerFailover` with the `ConsumerFailover` feature enabled. This preset needs its own URL configuration that defines how redirected traffic reaches the zone:

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: Zone
metadata:
  name: aws-zone
  namespace: production
spec:
  gateway:
    presets:
      - name: default
        default: true
        urls:
          - hostname: api.aws-zone.example.com
            basePath: /
      - name: ConsumerFailover
        default: false
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
| `presets[].name` | Must be a valid identifier. The preset that enables consumer failover is typically named `ConsumerFailover`. |
| `presets[].features` | Must include `{name: "ConsumerFailover", enabled: true}` for the zone to participate in consumer failover. |
| `presets[].urls` | Defines the hostnames used by consumers redirected via DTC. These hostnames are added to all routes that need to accept failover traffic. |

### What Happens Automatically

Once the preset is configured:

- The zone's `status.features` will include `ConsumerFailover: enabled`.
- All API exposures that have subscribers with `failover.enabled: true` will be enriched with:
  - **Additional hostnames** from the ConsumerFailover preset (so the zone's gateway accepts traffic arriving on the failover hostname).
  - **Additional trusted identity providers** (so tokens issued by any participating zone's IDP are accepted).
- Proxy routes are created in this zone for APIs that have consumer-failover-enabled subscribers, even if no subscriber in this zone directly subscribes to those APIs.
- ConsumeRoutes are created to grant redirected consumers access to the appropriate routes.

### Verifying the Setup

After applying the zone configuration, check that the feature is enabled:

```bash
kubectl get zone aws-zone -n production -o jsonpath='{.status.features}'
```

The output should include:

```json
[{"name":"ConsumerFailover","enabled":true}]
```

### Removing Consumer Failover from a Zone

To stop a zone from participating in consumer failover, remove the `ConsumerFailover` feature from the preset (or remove the preset entirely). The Control Plane will automatically:

- Set `ConsumerFailover: false` in the zone's status.
- Stop creating new failover routes and ConsumeRoutes for this zone.

:::caution
Removing the ConsumerFailover feature from a zone while consumers are actively relying on it may cause traffic disruption during DNS failover events. Coordinate with your teams before disabling this feature.
:::
