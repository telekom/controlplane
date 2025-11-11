<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: CC0-1.0    
-->

<p align="center">
  <h1 align="center">Organization</h1>
</p>

<p align="center">
  The Organization domain is responsible for managing teams and groups within the platform. 
  It provides an API to manage team memberships, namespaces, and associated resources.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#features">Features</a> •
  <a href="#notifications">Notifications</a> •
</p>


## About

This folder contains the implementation of the Organization domain, which manages teams and groups within the control plane.
The domain automatically provisions namespaces, identity clients, gateway consumers, and notification channels for each team.

## Features

- **Team Management**: Create and manage teams with members and email configurations.
- **Group Management**: Organize teams into logical groups.
- **Namespace Provisioning**: Automatically create and manage Kubernetes namespaces for teams.
- **Identity Integration**: Provision identity clients for team authentication.
- **Gateway Integration**: Create gateway consumers for API access control.
- **Notification System**: Automated notifications for team lifecycle events.

## Notifications

The organization operator automatically sends notifications for team lifecycle events. Notifications are sent via email to all team members and the team's primary email address.

### Notification Overview

| Event                | Trigger                                      | Map Key                | Purpose                | Notification Name              | Hash Generation               |
|----------------------|----------------------------------------------|------------------------|------------------------|--------------------------------|-------------------------------|
| **Team Onboarding**  | Team creation (generation == 1)              | `onboarded`            | `onboarded`            | `onboarded`                    | N/A                           |
| **Token Rotation**   | Every reconciliation (deduplicated via hash) | `token-rotated`        | `token-rotated`        | `token-rotated--{hash}`        | Hash of `TeamToken` value     |
| **Member Changes**   | Team member list updated (generation > 1)    | `team-members-changed` | `team-members-changed` | `team-members-changed--{hash}` | Hash of `Members` list        |

> ![Note]
> The hash is computed using a deterministic hashing function to ensure idempotency. The same input (token or member list) always produces the same hash, preventing duplicate notifications.

> ![Note]
> All sent notifications are tracked in the team's status under `NotificationsRef` using the map keys shown above. Take a look at [TeamStatus Structure in ./api/v1/team_types.go](./api/v1/team_types.go) to view the latest references.

### Available Properties in Notification Templates

The following properties are automatically included in all organization notifications and can be used in notification templates:

| Property      | Description                                                      | Example                                                                                                  |
|---------------|------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------|
| `environment` | The environment where the team was created                       | `prod`, `dev`                                                                                            |
| `team`        | The team name                                                    | `backend`                                                                                                |
| `group`       | The group name                                                   | `platform`                                                                                               |
| `members`     | Array of team members with name and email                        | `[{"name": "John Doe", "email": "john@example.com"}, {"name": "Jane Doe", "email": "jane@example.com"}]` |

**Note**: The `members` property is an array of objects, each containing `name` and `email` fields. In templates, you can iterate over members and access both fields:
```
{{range .members}}
- {{.name}} ({{.email}})
{{end}}
```

### Notification Channels

The operator automatically creates a `NotificationChannel` resource for each team:
- **Name**: `{group}--{team}--mail`
- **Namespace**: Team's namespace (`{environment}--{group}--{team}`)
- **Type**: Email (MS Teams and Webhook support planned for future)
- **Recipients**: All member emails + team email

## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](../CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/). Each file contains copyright and license information, and license texts can be found in the [./LICENSES](../LICENSES) folder. For more information visit https://reuse.software/.