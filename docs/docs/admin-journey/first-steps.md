---
sidebar_position: 3
---

# First Steps

Your Control Plane is now installed. To set up your first zone, you also need one Gateway and one IRIS instance available. This tutorial helps you bootstrap the minimum setup so teams can start using it.

At the end, you will have:

- One environment (`controlplane`)
- One zone (`dataplane1`)
- One group (`phoenix`)
- One team (`phoenix--firebirds`)
- A sample API exposed, and two applications subscribed to it

## Before you begin

- Control Plane is running (see [Installation](./installation.md))
- One Gateway is available and reachable
- One IRIS instance is available and reachable
- You have `kubectl` access to the cluster

## Step 1 — Create your first environment

An **Environment** and its **Zones** form the foundational infrastructure that everything else depends on. An environment represents a logical workload boundary (such as `dev`, `staging`, or `production`), while each zone is a deployment target with its own gateway and identity provider.

This step creates one environment called `controlplane` and two zones (`dataplane1`, `dataplane2`).

<details>
<summary>Environment resource definition</summary>

An Environment represents a logical separation of workloads. All resources belonging to an environment — zones, groups, teams, applications — live within its namespace.

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: Environment
metadata:
  name: controlplane
  namespace: controlplane
```

:::info Architecture Reference
For the full Environment specification, see the [Admin Domain](../architecture/admin.mdx) architecture page.
:::

</details>

<details>
<summary>Zone resource definition</summary>

A Zone defines a deployment target within an environment. It holds the connection details for the gateway, identity provider, and Redis instance that serve this zone.

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: Zone
metadata:
  name: dataplane1
  namespace: controlplane
spec:
  # Who can reach APIs in this zone: World (public) or Enterprise (internal)
  visibility: Enterprise
  gateway:
    url: https://my-gateway.example.com/
    admin:
      url: https://my-gateway-admin.example.com/admin-api
      clientSecret: someSecret
  identityProvider:
    url: https://my-idp.example.com/
    admin:
      clientId: admin-cli
      userName: admin
      password: somePassword
  redis:
    host: my-redis-host
    port: 6379
    password: somePassword
```

:::caution
The zone files in the sample overlay (`install/overlays/local/resources/admin/zones/`) are provided as `.example.yaml` templates. Copy them, fill in your real credentials, and never commit secrets to version control.
:::

:::info Architecture Reference
For the full Zone specification and all configuration options, see the [Admin Domain](../architecture/admin.mdx) architecture page.
:::

</details>

Apply the bundled admin resources:

```bash
kubectl apply -k install/overlays/local/resources/admin
```

Verify:

```bash
kubectl get environments -n controlplane
kubectl get zones -n controlplane
```

You should see one environment (`controlplane`) and two sample zones (`dataplane1`, `dataplane2`).

## Step 2 — Create a group and team

**Groups** organize teams by department or business unit. **Teams** represent the people who own applications and APIs. When you create a team, the Control Plane automatically provisions everything it needs — a dedicated namespace, identity credentials, gateway access, and a notification channel.

<details>
<summary>Group resource definition</summary>

A Group is a logical container for teams. It helps you organize teams by department, business unit, or any other structure that makes sense for your organization.

```yaml
apiVersion: organization.cp.ei.telekom.de/v1
kind: Group
metadata:
  name: phoenix
  namespace: controlplane
spec:
  displayName: phoenix
  description: "This is a sample group called phoenix"
```

:::info Architecture Reference
For the full Group specification, see the [Organization Domain](../architecture/organization.mdx) architecture page.
:::

</details>

<details>
<summary>Team resource definition</summary>

A Team represents a group of people who share ownership of applications and APIs. Its name follows the pattern `{group}--{team}`.

When the team is created, the Organization operator automatically provisions:

- A **dedicated namespace** — `controlplane--phoenix--firebirds`
- An **Identity Client** — for authenticating with the platform
- A **Gateway Consumer** — for accessing APIs through the gateway
- A **Notification Channel** — for receiving platform notifications

```yaml
apiVersion: organization.cp.ei.telekom.de/v1
kind: Team
metadata:
  name: phoenix--firebirds
  namespace: controlplane
spec:
  name: firebirds
  group: phoenix
  email: firebirds-mail@example.com
  members:
    - name: user1
      email: user1@example.com
```

:::info Architecture Reference
For the full Team specification and details on auto-provisioning, see the [Organization Domain](../architecture/organization.mdx) architecture page.
:::

</details>

Apply the sample organization resources:

```bash
kubectl apply -k install/overlays/local/resources/org
```

Verify:

```bash
kubectl get groups -n controlplane
kubectl get teams -n controlplane
```

The team controller also creates a dedicated namespace for the team.

```bash
kubectl get ns | grep "controlplane--phoenix--firebirds"
```

## Step 3 — Apply a sample API exposure and subscription

The **Rover** resource is the primary entry point for application teams. A single Rover file declares which APIs an application exposes and which ones it subscribes to. The Rover operator then translates this into resources across multiple domains — creating applications, API registrations, gateway routes, and identity clients automatically.

This step creates an API specification, one application that exposes and subscribes to the API, and a second application that only subscribes to it.

<details>
<summary>ApiSpecification resource definition</summary>

An ApiSpecification holds the OpenAPI document for an API. It is referenced by Rover files when exposing an API.

```yaml
apiVersion: rover.cp.ei.telekom.de/v1
kind: ApiSpecification
metadata:
  name: phoenix-echo-v1
  namespace: controlplane--phoenix--firebirds
spec:
  # The full OpenAPI specification is embedded inline
  specification: |
    openapi: "3.0.0"
    info:
      version: "1.0.0"
      title: "Echo API"
      x-api-category: "test"
    servers:
      - url: "https://example.com/phoenix/echo/v1"
    # ... (security schemes, paths, etc.)
```

:::info Architecture Reference
For the full ApiSpecification schema, see the [Rover Domain](../architecture/rover.mdx) architecture page.
:::

</details>

<details>
<summary>Rover resource definition — provider</summary>

This Rover file registers an application called `rover-echo-v1` that **exposes** the Echo API and also **subscribes** to it. Key fields include the target zone, upstream URLs with load-balancing weights, visibility, and the approval strategy.

```yaml
apiVersion: rover.cp.ei.telekom.de/v1
kind: Rover
metadata:
  name: rover-echo-v1
  namespace: controlplane--phoenix--firebirds
spec:
  zone: dataplane1                      # Target zone for this application
  clientSecret: someSecret
  exposures:
    - api:
        basePath: /phoenix/echo/v1      # Must match the ApiSpecification
        upstreams:                       # Backend services with load-balancing weights
          - url: https://httpbin.org/anything
            weight: 50
          - url: https://httpbin.org/anything/balancing-the-load
            weight: 50
        visibility: World               # Who can discover and subscribe: World, Enterprise, or Zone
        approval:
          strategy: Auto                # Auto-approve all subscriptions (alternatives: Simple, FourEyes)
        security:
          m2m:
            scopes:
              - read
  subscriptions:
    - api:
        basePath: /phoenix/echo/v1
        security:
          m2m:
            scopes:
              - read
              - write
```

</details>

<details>
<summary>Rover resource definition — consumer</summary>

A second Rover file registers an application called `rover-echo-consumer` that only **subscribes** to the Echo API. Since the API was exposed with approval strategy `Auto`, this subscription is granted immediately.

```yaml
apiVersion: rover.cp.ei.telekom.de/v1
kind: Rover
metadata:
  name: rover-echo-consumer
  namespace: controlplane--phoenix--firebirds
spec:
  zone: dataplane1
  clientSecret: someSecret
  subscriptions:
    - api:
        basePath: /phoenix/echo/v1
```

:::info Architecture Reference
For the full Rover specification including event exposures and subscriptions, see the [Rover Domain](../architecture/rover.mdx) architecture page. For details on how exposures and subscriptions are processed, see the [API Domain](../architecture/api.mdx).
:::

</details>

Apply the sample Rover resources:

```bash
kubectl apply -k install/overlays/local/resources/rover
```

Verify:

```bash
kubectl get apispecifications -n controlplane--phoenix--firebirds
kubectl get rovers -n controlplane--phoenix--firebirds
```

## What happened behind the scenes

When you applied the Rover resources in Step 3, the Control Plane did much more than just store them. The Rover operator kicked off a **reconciliation chain** that created resources across multiple domains:

1. **Rover** operator processed each Rover file and created **Application**, **Api**, **ApiExposure**, and **ApiSubscription** resources
2. **API** operator processed the exposure and subscription, creating **Gateway Route** and **ConsumeRoute** resources
3. **Gateway** operator configured the actual API gateway with the route definitions
4. **Identity** operator provisioned authentication clients in the identity provider

You can inspect these auto-created resources:

```bash
kubectl get applications -n controlplane--phoenix--firebirds
kubectl get apis -n controlplane--phoenix--firebirds
kubectl get apiexposures -n controlplane--phoenix--firebirds
kubectl get apisubscriptions -n controlplane--phoenix--firebirds
```

:::tip
This is the operator pattern in action — each domain watches for changes to its resources and reconciles them to the desired state. For a deeper understanding of how domains interact, see the [Architecture Overview](../architecture/overview.md).
:::

## Recommended next actions

Now replace the sample values with your real platform configuration:

1. Update zone endpoints and credentials (Gateway, Identity Provider, Redis)
2. Create real groups and teams for your organization
3. Add notification templates for onboarding and approval flows

| Topic | Guide | Architecture Reference |
| ----- | ----- | ---------------------- |
| Environments & Zones | [Admin Journey](./environments-and-zones.md) | [Admin Domain](../architecture/admin.mdx) |
| Organizations & Teams | [Admin Journey](./organizations-and-teams.md) | [Organization Domain](../architecture/organization.mdx) |
| Notification Templates | [Admin Journey](./notification-templates.md) | [Notification Domain](../architecture/notification.mdx) |
| Exposing APIs | [User Journey](../user-journey/exposing-apis.mdx) | [API Domain](../architecture/api.mdx) |
| Subscribing to APIs | [User Journey](../user-journey/subscribing-to-apis.mdx) | [Rover Domain](../architecture/rover.mdx) |
