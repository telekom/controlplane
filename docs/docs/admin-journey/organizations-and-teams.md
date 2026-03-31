---
sidebar_position: 3
---

# Organizations & Teams

The Organization domain manages teams and groups within the Control Plane. When you create a team, the platform automatically provisions everything that team needs to start working — a dedicated namespace, identity credentials, gateway access, and a notification channel.

## Groups

A Group is a logical container for teams. Groups help you organize teams by department, business unit, or any other structure that makes sense for your organization.

```yaml
apiVersion: organization.cp.ei.telekom.de/v1
kind: Group
metadata:
  name: payment-services
  namespace: dev
```

## Teams

A Team represents a group of people who share ownership of applications and APIs. When a team is created, the Control Plane automatically provisions:

- A **dedicated namespace** following the pattern `{environment}--{group}--{team}`
- An **Identity Client** for authentication
- A **Gateway Consumer** for API access
- A **Notification Channel** for receiving platform notifications

```yaml
apiVersion: organization.cp.ei.telekom.de/v1
kind: Team
metadata:
  name: payment-services--checkout
  namespace: dev
spec:
  group: payment-services
  team: checkout
  email: checkout-team@example.com
  members:
    - alice@example.com
    - bob@example.com
```

### Team Tokens

Each team receives a **Team Token** that can be used for CLI authentication with Rover-CTL and the Rover Server API. The token is a base64-encoded JSON structure containing the environment, group, team, and client credentials.

:::info
Team tokens are automatically generated when a team is created. They can be rotated through the platform, which triggers a notification to all team members.
:::

## Lifecycle Notifications

The Organization domain sends notifications for key events in a team's lifecycle:

- **Onboarding** — When a team is first created
- **Token rotation** — When team credentials are rotated
- **Member changes** — When team members are added or removed

These notifications are delivered through the channels configured in the [Notification Templates](./notification-templates.md) setup.

## Next Steps

- [Notification Templates](./notification-templates.md) — Configure how notifications are delivered
- [Architecture: Organization Domain](../architecture/organization.mdx) — Deep dive into the Organization domain
