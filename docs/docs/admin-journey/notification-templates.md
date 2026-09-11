---
sidebar_position: 6
---

# Notification Templates

The Notification domain handles delivery of platform notifications. As a platform administrator, you define **templates** that control how messages look for each use case and channel.

Templates give you a clean separation between:

- **When** a notification is triggered (by domain logic)
- **How** the message is rendered (by your template)

## Channels

The Control Plane supports three notification channel types:

| Channel | Description |
| ------- | ----------- |
| **Email** | Delivers notifications via email. Requires SMTP configuration. |
| **MS Teams** | Delivers notifications to Microsoft Teams channels via webhooks. |
| **Webhook** | Delivers notifications to any HTTP endpoint. |

### Automatic channel provisioning

When a team is created through the Organization domain, a **NotificationChannel** is automatically created for that team. You can configure channel details (recipients, webhook settings, filters) to control what each team receives.

## How template selection works

At send time, the platform selects a template by combining:

- the notification **purpose** (for example, `onboarded`)
- the channel type suffix (for email channels this is `mail`)

So an onboarding email template is expected to be named:

`onboarded--mail`

If no matching template exists, the notification cannot be rendered.

## Templates

A Notification Template defines how a notification for a specific purpose and channel type is formatted. Templates use placeholder variables that are filled in at delivery time.

```yaml
apiVersion: notification.cp.ei.telekom.de/v1
kind: NotificationTemplate
metadata:
  name: approvalrequest--subscribe--created--decider--mail
  namespace: dev
spec:
  purpose: approvalrequest--subscribe--created--decider
  channelType: Email
  subjectTemplate: "[{{.environment}}] New subscription request for {{.resource_name}}"
  template: |
    Hello team {{.decider_team}},

    A new subscription request has been created by team {{.requester_team}}
    for {{.resource_type}} {{.resource_name}}.

    Please review and approve or reject the request.
```

### Required fields

- `metadata.name` — must follow the lookup convention (`<purpose>--<channelSuffix>`)
- `spec.purpose` — event type that this template handles
- `spec.channelType` — `Email`, `MsTeams`, or `Webhook`
- `spec.template` — message body (text or HTML)
- `spec.subjectTemplate` — optional, but typically required for `Email`

### Placeholders

Placeholders are dynamic values injected at delivery time.

Common examples:

- `{{.environment}}`
- `{{.requester_team}}`, `{{.requester_group}}`, `{{.requester_application}}`
- `{{.decider_team}}`, `{{.decider_group}}`, `{{.decider_application}}`
- `{{.resource_type}}`, `{{.resource_name}}`
- `{{.state_old}}`, `{{.state_new}}`
- `{{.scopes}}`

You can also use basic Go-template logic such as `if` and `range`.

### Attachments

Notification templates can include **file attachments** that are generated from their own templates. This is useful for attaching calendar invites, structured data files, or any other content that should accompany the notification.

Add an `attachments` array to your template spec:

```yaml
spec:
  purpose: secret-rotation-expiring
  channelType: Email
  subjectTemplate: "[{{.environment}}] Action required: Secret expiring"
  template: |
    ...
  attachments:
    - filename: "reminder.ics"
      contentType: "text/calendar; charset=utf-8"
      template: |
        BEGIN:VCALENDAR
        VERSION:2.0
        PRODID:-//Control Plane//EN
        BEGIN:VEVENT
        DTSTART:{{ icsTime .currentExpiresAt }}
        SUMMARY:Secret expiring for {{ .application }}
        END:VEVENT
        END:VCALENDAR
```

Each attachment has three fields:

| Field | Description |
| ----- | ----------- |
| `filename` | The file name recipients see (e.g. `reminder.ics`). |
| `contentType` | MIME type of the attachment (e.g. `text/calendar; charset=utf-8`). |
| `template` | A Go template that produces the attachment content. The same placeholders available to the main template body are available here. |

Attachments are rendered at delivery time, just like the main template body. You can include multiple attachments per template.

:::tip
The built-in secret rotation templates use attachments to send `.ics` calendar invites so teams can add expiration deadlines to their calendars.
:::

### Built-in purposes (default templates)

The installation component `install/components/notificationtemplates` ships templates for these built-in purposes:

| Purpose | Typical trigger |
| ------- | --------------- |
| `approvalrequest--subscribe--created--decider` | New subscription request created |
| `approvalrequest--subscribe--updated--decider` | Request state changed (decider view) |
| `approvalrequest--subscribe--updated--requester` | Request state changed (requester view) |
| `approval--subscribe--updated--decider` | Subscription updated (decider view) |
| `approval--subscribe--updated--requester` | Subscription updated (requester view) |
| `onboarded` | Team created |
| `token-rotated` | Team token rotated |
| `team-members-changed` | Team members updated |
| `secret-rotation-expiring` | Application secret approaching expiration |
| `secret-rotation-completed` | Application secret successfully rotated |

## Enabling default templates

The default templates are provided as a Kustomize component.

1. Add the component to your overlay:

```yaml
components:
  - ../../components/notificationtemplates
```

2. Apply your overlay:

```bash
kubectl apply -k install/overlays/local
```

Adjust the overlay path to match your installation model.

## Recommended admin workflow

1. **Start from defaults**: enable the notification template component.
2. **Brand safely**: update wording, layout, and logos without changing `purpose` and template naming conventions.
3. **Test in non-production**: trigger an onboarding or approval flow and verify rendered output.
4. **Promote with GitOps**: version template changes like any other platform config.

## Troubleshooting

- **Notification exists but not delivered**
  - Check that the team has a ready `NotificationChannel`.
- **Template not found errors**
  - Verify `metadata.name` matches `<purpose>--<channelSuffix>`.
  - Verify `spec.purpose` exactly matches the emitted purpose.
- **Broken rendering / empty fields**
  - Check placeholder names (they are case-sensitive).
  - Ensure the expected properties are provided by the source notification.

## Next Steps

- [Operations & Monitoring](./operations.md) — Day-2 operational tasks
- [Architecture: Notification Domain](../architecture/notification.mdx) — Deep dive into the Notification domain
