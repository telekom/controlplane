---
sidebar_position: 5
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
spec:
  displayName: Payment Services
  description: Teams responsible for payment-related capabilities
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
  name: checkout
  email: checkout-team@example.com
  members:
    - name: Alice Example
      email: alice@example.com
    - name: Bob Example
      email: bob@example.com
```

### User Membership Lookup

When the ControlPlane API receives a forwarded user identity from an authorized client (such as the BFF), it resolves team memberships using the shared ASCII email policy: only `A-Z` become `a-z`. For example, `Alice@Example.com` and `alice@example.com` match the same memberships across teams. Characters such as `%` and `_` are treated literally, not as wildcards. Dots, tags, quoting, and Unicode are not rewritten or normalized; `Üser@example.com` and `üser@example.com` remain distinct identities.

Normalization happens at Team member admission and on the API's membership query input. The Team mutating webhook writes canonical `spec.members[].email` values before sorting on create and update. Team contact emails (`spec.email`) and forwarded user identities displayed by the API retain their original casing. Deleting Teams skips email mutation and validation. Member removal uses the shared canonical comparison for its supplied email and the CR members; the removal argument itself does not reach admission. Team mutations rely on Kubernetes admission for email validation.

Contact and member emails must be bare addresses, without display names, comments, or outer whitespace. Quoted mailboxes are accepted by the shared syntax validator; Team admission also applies the existing schema's email format and length constraints. Nonempty forwarded user emails are checked after percent decoding, with malformed input returning HTTP 400 as an RFC 9457 problem. An absent email remains optional. Unicode such as `ü` is supported by Team admission and preserved, but syntax acceptance does not guarantee delivery: downstream mail systems may require SMTPUTF8 support.

The Projector trusts admitted Teams and copies member emails as received, using its normal upsert and orphan deletion logic. Ent retains its nonempty email field and existing indexes; email syntax validation and canonicalization belong to ingress. Membership requests use indexed exact lookup, so an existing noncanonical database row can appear as a missing membership. Operators are responsible for manually correcting bad existing source or database data and resolving identity collisions.

**Server-side apply:** use canonical ASCII-lowercase member emails in manifests from the first apply. Email is a map-list key: admission changes a mixed-case key after apply tracks it. Admission tests confirm that repeating the same mixed-case apply fails with duplicate member keys. Switching that manifest to canonical keys allows repeated applies, name edits, and member removals. Normal create/update requests still accept mixed-case ASCII input.

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
