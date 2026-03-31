---
sidebar_position: 2
---

# Environments & Zones

Environments and zones form the foundational infrastructure of the Control Plane. They define *where* applications are deployed and *which* gateway and identity provider instances are used.

## Environments

An Environment represents a logical separation of workloads — for example, `dev`, `staging`, or `production`. Each environment is a Kubernetes namespace, and all resources belonging to that environment (zones, groups, teams, applications) are created within it.

### Creating an Environment

To create an environment, apply an Environment resource. The namespace must match the environment name:

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: Environment
metadata:
  name: dev
  namespace: dev
```

:::tip
Create the Kubernetes namespace before applying the Environment resource, or use a namespace provisioning tool that handles this automatically.
:::

## Zones

A Zone represents a physical or logical deployment target within an environment. Each zone has its own gateway and identity provider configuration, and the Control Plane creates a dedicated namespace for the zone's resources.

Zones are the key building block for multi-cloud deployments. You can have zones pointing to different cloud providers (for example, one zone on AWS and another on Azure), and the Control Plane will manage API routing and event meshing across them.

### Creating a Zone

A Zone references the gateway and identity provider to use, along with Redis configuration and visibility settings:

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: Zone
metadata:
  name: dataplane1
  namespace: dev
spec:
  visibility: World
  gateway:
    url: https://gateway.example.com
    circuitBreaker: true
    admin:
      url: https://gateway-admin.example.com
      clientSecret: <your-gateway-admin-secret>
  identityProvider:
    url: https://idp.example.com
    admin:
      url: https://idp-admin.example.com
      clientId: admin-client
      userName: admin
      password: <your-idp-admin-password>
  redis:
    host: redis.example.com
    port: 6379
    password: <your-redis-password>
    enableTLS: true
```

:::caution
Credential values (`clientSecret`, `password`, etc.) should not be committed to version control. In production, use a sealed-secrets solution or external secret management.
:::

### Zone Visibility

Zones can be configured with different visibility levels:

- **World** — The zone is accessible from outside the platform (public-facing APIs).
- **Enterprise** — The zone is accessible only within the organization's network.

## Remote Organizations

For cross-platform federation, the Control Plane supports Remote Organizations. A Remote Organization represents an external Control Plane instance that your platform can exchange API subscriptions and events with.

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: RemoteOrganization
metadata:
  name: partner-platform
  namespace: dev
spec:
  clientId: federation-client
  clientSecret: <your-federation-secret>
  issuerUrl: https://partner-controlplane.example.com/auth/realms/master
  zone:
    name: dataplane1
    namespace: dev
```

## Next Steps

- [Organizations & Teams](./organizations-and-teams.md) — Set up teams within your environments
- [Architecture: Admin Domain](../architecture/admin.mdx) — Deep dive into the Admin domain
