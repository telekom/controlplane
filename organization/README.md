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

| Event                | Trigger                                      | Purpose                | Notification Name              | Properties Included                       | Hash Generation           |
|----------------------|----------------------------------------------|------------------------|--------------------------------|-------------------------------------------|---------------------------|
| **Team Onboarding**  | Team creation (generation == 1)              | `onboarded`            | `onboarded`                    | `environment`, `team`, `group`, `members` | N/A                       |
| **Token Rotation**   | Every reconciliation when team token changes | `token-rotated`        | `token-rotated--{hash}`        | `environment`, `team`, `group`, `members` | Hash of `TeamToken` value |
| **Member Changes**   | Team member list updated (generation > 1)    | `team-members-changed` | `team-members-changed--{hash}` | `environment`, `team`, `group`, `members` | Hash of `Members` list    |

> **Note**: The hash is computed using a deterministic hashing function to ensure idempotency. The same input (token or member list) always produces the same hash, preventing duplicate notifications.

### Notification Channels

The operator automatically creates a `NotificationChannel` resource for each team:
- **Name**: `{group}--{team}--mail`
- **Namespace**: Team's namespace (`{environment}--{group}--{team}`)
- **Type**: Email (MS Teams and Webhook support planned for future)
- **Recipients**: All member emails + team email

### Notification References

All sent notifications are tracked in the team's status:
```yaml
status:
  notificationChannelRef:
    name: group--team--mail
    namespace: env--group--team
  notificationsRef:
    onboarded:
      name: onboarded
      namespace: env--group--team
    token-rotated:
      name: token-rotated--abc123
      namespace: env--group--team
    team-members-changed:
      name: team-members-changed--def456
      namespace: env--group--team
```