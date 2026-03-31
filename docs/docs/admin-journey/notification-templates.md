---
sidebar_position: 4
---

# Notification Templates

The Notification domain handles the delivery of platform notifications. As a platform administrator, you define **templates** that control how notifications are formatted and which channels they are delivered through.

## Channels

The Control Plane supports three notification channel types:

| Channel | Description |
| ------- | ----------- |
| **Email** | Delivers notifications via email. Requires SMTP configuration. |
| **MS Teams** | Delivers notifications to Microsoft Teams channels via webhooks. |
| **Webhook** | Delivers notifications to any HTTP endpoint. |

### Automatic Channel Provisioning

When a team is created through the Organization domain, a **Notification Channel** is automatically created for that team. You can configure each channel with authentication settings and filter which notification purposes it should receive.

## Templates

A Notification Template defines how a notification for a specific purpose and channel type is formatted. Templates use placeholder variables that are filled in at delivery time.

```yaml
apiVersion: notification.cp.ei.telekom.de/v1
kind: NotificationTemplate
metadata:
  name: approval-created--email
  namespace: dev
spec:
  purpose: approval-created
  channelType: Email
  subject: "New approval request: {{.ApiName}}"
  body: |
    A new approval request has been created for API {{.ApiName}}
    by team {{.RequesterTeam}}.

    Please review and approve or reject the request.
```

### Notification Purposes

Templates are linked to specific purposes. The following purposes are built into the platform:

- **Approval lifecycle** — Request created, state changed (granted, rejected, suspended, expired)
- **Team lifecycle** — Onboarding, token rotation, member changes
- **Subscription events** — API or event subscription created, updated, or removed

## Next Steps

- [Operations & Monitoring](./operations.md) — Day-2 operational tasks
- [Architecture: Notification Domain](../architecture/notification.mdx) — Deep dive into the Notification domain
