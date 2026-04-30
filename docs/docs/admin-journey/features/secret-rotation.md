---
sidebar_position: 2
---

# Secret Rotation

Graceful secret rotation allows application teams to replace expiring client secrets without downtime. As a platform administrator, you control whether rotation is enabled, how long grace periods last, and when reminder notifications are sent.

## Enabling Secret Rotation

Secret rotation is configured per **Zone** through the identity provider settings. Add the `secretRotation` block to your Zone resource:

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: Zone
metadata:
  name: dataplane1
spec:
  identityProvider:
    secretRotation:
      enabled: true
      expirationPeriod: 2160h    # 90 days
      gracePeriod: 168h          # 7 days
      notificationThresholds:
        - before: 720h           # exactly once with 30 days remaining
        - before: 168h           # the for last 7 days
          repeat: 24h       # daily
```

### Configuration Reference

| Field | Description | Example |
| ----- | ----------- | ------- |
| `enabled` | Turns secret rotation on or off for all applications in this zone. | `true` |
| `expirationPeriod` | How long a secret is valid before it must be rotated. | `2160h` (90 days) |
| `gracePeriod` | How long the **old** secret remains valid after a rotation is triggered. Both secrets are accepted during this window. | `168h` (7 days) |
| `notificationThresholds` | Schedule of reminder notifications sent before a secret expires (see below). | — |

### Notification Thresholds

Each threshold entry defines when a reminder email is sent relative to the secret's expiration date.

| Field | Description | Example |
| ----- | ----------- | ------- |
| `before` | Time before expiry when the first notification is sent. | `720h` (30 days) |
| `repeatEvery` | *(Optional)* If set, the notification repeats at this interval until expiry or the next threshold takes over. | `24h` |

:::info Duration format
All duration fields use Go's standard duration format. Supported units are `h` (hours), `m` (minutes), and `s` (seconds). Day-based units like `d` are **not** supported — use hours instead (e.g. 7 days = `168h`, 30 days = `720h`).
:::

**Example schedule:**

```yaml
notificationThresholds:
  - before: 720h             # Single reminder 30 days before expiry
  - before: 168h             # Daily reminders starting 7 days before expiry
    repeatEvery: 24h
```

With this configuration, a team receives a single email at 30 days, then daily emails starting at 7 days until the secret expires or is rotated.

## Notification Templates

Two new notification templates ship with this feature. They are included in the default notification templates component (`install/components/notificationtemplates`):

| Template name | Purpose | When it fires |
| ------------- | ------- | ------------- |
| `secret-rotation-expiring--mail` | `secret-rotation-expiring` | Before a secret expires (per threshold schedule) |
| `secret-rotation-completed--mail` | `secret-rotation-completed` | After a secret has been successfully rotated |

Both templates include a **calendar invite attachment** (`.ics` file) so that teams can add the deadline to their calendars.

### Available Placeholders

These placeholders are available in secret rotation templates:

| Placeholder | Description |
| ----------- | ----------- |
| `{{.application}}` | Application name |
| `{{.team}}` | Team name |
| `{{.teamEmail}}` | Team notification email |
| `{{.environment}}` | Environment name |
| `{{.currentExpiresAt}}` | Expiration date of the current (new) secret |
| `{{.rotatedExpiresAt}}` | Date when the old secret stops being valid (grace period end) |
| `{{.timeUntilExpiry}}` | Human-readable time remaining until expiry |
| `{{.thresholdBefore}}` | The threshold that triggered this notification |

### Installing the Templates

If you already include the notification templates component in your Kustomize overlay, the secret rotation templates are installed automatically:

```yaml
components:
  - ../../components/notificationtemplates
```

### Customizing the Templates

You can customize the email wording, branding, and layout by creating your own `NotificationTemplate` resources. Keep the `purpose` and naming convention unchanged so the platform can find them:

- Template name: `<purpose>--mail`
- Purpose must match exactly: `secret-rotation-expiring` or `secret-rotation-completed`

See [Notification Templates](../notification-templates.md) for the general template authoring guide.

## Disabling Rotation for Individual Applications

Some applications manage their own credentials externally and should not participate in platform-managed rotation. To opt out a specific application, add the following annotation to its Identity Client:

```
identity.cp.ei.telekom.de/disable-secret-rotation: "true"
```

This is typically done by the platform team, not by application developers.

## Operational Considerations

- **Grace period sizing** — Choose a grace period long enough for teams to update all their consumers. Seven days is a common starting point.
- **Expiration period** — Shorter periods improve security posture but increase operational overhead. 90 days is a reasonable default.
- **Notification lead time** — Start reminders early enough (for example 30 days) so teams can plan ahead, then increase frequency closer to the deadline.
- **Monitoring** — Watch for applications whose `SecretRotation` condition stays in `InProgress` for an unusually long time — this may indicate a stuck consumer update.

## Related Pages

- [User Guide: Graceful Secret Rotation](../../user-journey/features/secret-rotation.mdx) — End-user perspective
- [Notification Templates](../notification-templates.md) — Template authoring and customization
- [Environments and Zones](../environments-and-zones.md) — Zone configuration
