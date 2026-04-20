---
sidebar_position: 1
---

# API Reference

This page provides a complete reference of all Custom Resource Definitions (CRDs) and REST APIs exposed by the Control Plane.

## Custom Resource Definitions

The Control Plane models its configuration as **Kubernetes Custom Resources**. Each resource represents a specific concept — an API, a subscription, a gateway route, and so on. Operators watch these resources and reconcile the desired state with the actual infrastructure.

All CRDs share the base domain **`cp.ei.telekom.de`**, use API version **`v1`**, and are **namespace-scoped**.

:::tip Rover is the primary entry point
Most users do not create these resources directly. Instead, they write a single **Rover** file that declaratively describes their application's API and event posture. The Rover operator then creates and manages the underlying resources automatically. See the [Rover domain](../architecture/rover.mdx) for details.
:::

### Summary

| Domain | API Group | CRDs |
| ------ | --------- | :--: |
| [Admin](#admin) | `admin.cp.ei.telekom.de/v1` | 3 |
| [API](#api) | `api.cp.ei.telekom.de/v1` | 5 |
| [Application](#application) | `application.cp.ei.telekom.de/v1` | 1 |
| [Approval](#approval) | `approval.cp.ei.telekom.de/v1` | 2 |
| [Event](#event) | `event.cp.ei.telekom.de/v1` | 4 |
| [Gateway](#gateway) | `gateway.cp.ei.telekom.de/v1` | 5 |
| [Identity](#identity) | `identity.cp.ei.telekom.de/v1` | 3 |
| [Notification](#notification) | `notification.cp.ei.telekom.de/v1` | 3 |
| [Organization](#organization) | `organization.cp.ei.telekom.de/v1` | 2 |
| [PubSub](#pubsub) | `pubsub.cp.ei.telekom.de/v1` | 3 |
| [Rover](#rover) | `rover.cp.ei.telekom.de/v1` | 3 |
| | **Total** | **34** |

---

### Admin

**API Group:** `admin.cp.ei.telekom.de/v1` · [Architecture →](../architecture/admin.mdx)

Platform-wide settings such as environments, zones, and federation.

| Kind | Description |
| ---- | ----------- |
| **Environment** | Represents a deployment environment — the top-level organizational unit. |
| **Zone** | Configures a deployment zone with identity provider, gateway, and connection settings. Controls visibility (World/Enterprise) and circuit breaker settings. |
| **RemoteOrganization** | Represents a federated remote organization for cross-organization API subscription flows. *(Planned feature — not yet fully supported.)* |

---

### API

**API Group:** `api.cp.ei.telekom.de/v1` · [Architecture →](../architecture/api.mdx)

API lifecycle management — definitions, exposures, and subscriptions.

| Kind | Description |
| ---- | ----------- |
| **Api** | A registered API definition with version, base path, category, and optional OAuth2 scopes. |
| **ApiCategory** | Defines an API category with team restrictions, linting configuration, and naming requirements. |
| **ApiExposure** | Declares that a team exposes an API at a given base path with visibility, approval strategy, traffic management, and security settings. |
| **ApiSubscription** | Represents a subscription to an API by a consuming application, with security and failover configuration. |
| **RemoteApiSubscription** | Enables cross-organization API subscriptions between federated organizations. |

---

### Application

**API Group:** `application.cp.ei.telekom.de/v1` · [Architecture →](../architecture/application.mdx)

Application registration and zone assignment.

| Kind | Description |
| ---- | ----------- |
| **Application** | Represents a deployed application belonging to a team, with zone configuration, failover zones, and security settings. |

---

### Approval

**API Group:** `approval.cp.ei.telekom.de/v1` · [Architecture →](../architecture/approval.mdx)

Access request approvals with multiple strategies.

| Kind | Description |
| ---- | ----------- |
| **Approval** | Manages the approval lifecycle for access requests, supporting Auto, Simple, and FourEyes strategies. |
| **ApprovalRequest** | A specific versioned approval request linked to an Approval, tracking individual decisions. |

---

### Event

**API Group:** `event.cp.ei.telekom.de/v1` · [Architecture →](../architecture/event.mdx)

Asynchronous event publishing and subscription.

| Kind | Description |
| ---- | ----------- |
| **EventConfig** | Per-zone configuration for the event system with connection settings and mesh topology. |
| **EventType** | Registry entry for a known event type with a dot-separated identifier and JSON schema. |
| **EventExposure** | Declares that an application publishes events of a specific type, with visibility and approval settings. |
| **EventSubscription** | Declares that an application subscribes to events, with delivery configuration and trigger filters. |

---

### Gateway

**API Group:** `gateway.cp.ei.telekom.de/v1` · [Architecture →](../architecture/gateway.mdx)

API gateway instances, routes, and consumer management.

| Kind | Description |
| ---- | ----------- |
| **Gateway** | Represents a gateway instance with admin API access and connection settings. |
| **Realm** | Represents a gateway realm (token issuer scope) with URL and consumer configuration. |
| **Route** | Defines a gateway route with upstreams, downstreams, traffic management, and security settings. |
| **ConsumeRoute** | Binds a consumer to a gateway route with optional security and rate limit configuration. |
| **Consumer** | Represents a gateway consumer identity with optional IP restrictions. |

---

### Identity

**API Group:** `identity.cp.ei.telekom.de/v1` · [Architecture →](../architecture/identity.mdx)

Identity providers, realms, and OAuth2 clients.

| Kind | Description |
| ---- | ----------- |
| **IdentityProvider** | Manages an identity provider instance (e.g. Keycloak) with admin access configuration. |
| **Realm** | Represents an identity realm within a provider, exposing issuer URLs and admin credentials. |
| **Client** | Represents an OAuth2 client within an identity realm. |

---

### Notification

**API Group:** `notification.cp.ei.telekom.de/v1` · [Architecture →](../architecture/notification.mdx)

Notification delivery across email, webhooks, and messaging platforms.

| Kind | Description |
| ---- | ----------- |
| **Notification** | Triggers sending a notification using a template, with sender info and channel references. |
| **NotificationChannel** | Configures a notification delivery channel — email, MS Teams webhook, or generic webhook. |
| **NotificationTemplate** | Defines a notification template for a specific purpose and channel type. |

---

### Organization

**API Group:** `organization.cp.ei.telekom.de/v1` · [Architecture →](../architecture/organization.mdx)

Organizational structure — groups and teams.

| Kind | Description |
| ---- | ----------- |
| **Group** | Represents an organizational group (top-level tenant) with display name and description. |
| **Team** | Represents a team within a group, with members, email, and optional category. |

---

### PubSub

**API Group:** `pubsub.cp.ei.telekom.de/v1` · [Architecture →](../architecture/pubsub.mdx)

Low-level event infrastructure resources managed by controllers.

| Kind | Description |
| ---- | ----------- |
| **EventStore** | Connection details for the event configuration backend, created by the EventConfig controller. |
| **Publisher** | An event publisher registration, created by the EventExposure controller. |
| **Subscriber** | An event subscription registration with delivery and trigger config, created by the EventSubscription controller. |

---

### Rover

**API Group:** `rover.cp.ei.telekom.de/v1` · [Architecture →](../architecture/rover.mdx)

The primary user-facing interface for declarative application configuration.

| Kind | Description |
| ---- | ----------- |
| **Rover** | The primary user-facing resource — defines an application's complete API and event posture declaratively. |
| **ApiSpecification** | Stores an uploaded OpenAPI specification and creates the corresponding Api resource. |
| **EventSpecification** | Stores event type metadata and creates the corresponding EventType resource. |

---

## REST API

In addition to the Kubernetes CRDs, the Control Plane exposes two REST APIs.

### ControlPlane API

The ControlPlane API provides a read-only view over the resources managed by the Control Plane. It is primarily used by dashboards and internal tooling to query the current state of environments, APIs, subscriptions, and events without direct Kubernetes access.

### Rover Server

The Rover Server is the main REST endpoint for external users. It accepts Rover file submissions, validates configurations, manages file uploads (OpenAPI specs, event schemas), and creates the corresponding Kubernetes resources. The command-line tool **Rover-CTL** communicates with this API.

For more details on how these components fit together, see the [Architecture Overview](../architecture/overview.md).
