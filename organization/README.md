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
  <a href="#crds">CRDs</a>
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

| Event                | Trigger                                      | Map Key                | Notification Name              | Hash Generation               |
|----------------------|----------------------------------------------|------------------------|--------------------------------|-------------------------------|
| **Team Onboarding**  | Team creation (generation == 1)              | `onboarded`            | `onboarded`                    | N/A                           |
| **Token Rotation**   | Every reconciliation (deduplicated via hash) | `token-rotated`        | `token-rotated--{hash}`        | Hash of `TeamToken` value     |
| **Member Changes**   | Team member list updated (generation > 1)    | `team-members-changed` | `team-members-changed--{hash}` | Hash of `Members` list        |

> [!NOTE]
> The hash is computed using a deterministic hashing function to ensure idempotency. The same input (token or member list) always produces the same hash, preventing duplicate notifications.

> [!NOTE]
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

## CRDs
All CRDs can be found here: [CRDs](./config/crd/bases/).

<p>The Organization domain defines the following Custom Resources (CRDs):</p>

<details>
<summary>
<strong>Group</strong>
This CRD represents a logical grouping of teams within the organization.
</summary>  

- The Group CR MUST be created in the environment namespace (e.g., `dev`, `prod`).
- The Group CR name MUST follow the pattern `^[a-z0-9]+(-?[a-z0-9]+)*$` (lowercase alphanumeric with optional hyphens).
- The Group CR contains a display name and description for human-readable identification.
- Groups are used to organize teams and provide a hierarchical structure to the organization.
- Groups are referenced by Team CRs to establish the organizational hierarchy.

</details>
<br />

<details>
<summary>
<strong>Team</strong>
This CRD represents a team within the organization with its members and configuration.
</summary>  

- The Team CR MUST be created in the environment namespace (e.g., `dev`, `prod`).
- The Team CR name MUST follow the pattern `{group}--{team}` where both group and team match `^[a-z0-9]+(-?[a-z0-9]+)*$`.
- The Team CR MUST reference an existing Group.
- The Team CR MUST have at least one member with name and email.
- The Team CR MUST specify a team email address.
- The Team CR can be categorized as either `Customer` or `Infrastructure` which affects access rights.
- When a Team CR is created, the following resources are automatically provisioned:
  - A dedicated namespace with pattern `{environment}--{group}--{team}`
  - An Identity Client for authentication
  - A Gateway Consumer for API access
  - A Notification Channel for team communications
- The Team status tracks references to all provisioned resources.
- The Team CR includes a TeamToken that is automatically generated and rotated for authentication with team APIs.

</details>
<br />
