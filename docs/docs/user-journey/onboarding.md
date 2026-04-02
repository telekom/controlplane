---
sidebar_position: 1
---

# Onboarding

This guide walks you through getting started as an application team on the Control Plane. Before you can expose APIs, subscribe to services, or publish events, your team needs to be onboarded to the platform.

## Prerequisites

Before you begin, make sure a platform administrator has:

- Created an **Environment** for you to work in (for example, `dev`)
- Created a **Group** and **Team** for your team within that environment

If you are unsure whether your team has been set up, contact your platform administrator.

## What Happens When Your Team Is Created

When a platform administrator creates your team, the Control Plane automatically provisions the following resources for you:

- A **dedicated namespace** in the Kubernetes cluster, following the pattern `{environment}--{group}--{team}`
- An **Identity Client** — your team's service account for authenticating with the platform
- A **Gateway Consumer** — grants your team access to APIs through the gateway
- A **Notification Channel** — so you receive platform notifications (approvals, token rotations, etc.)

You will receive an onboarding notification with your team credentials once this provisioning is complete.

## Setting Up Rover-CTL

Rover-CTL is the command-line tool for interacting with the Control Plane. It lets you apply Rover files, manage API configurations, and check the status of your resources.

:::tip
For a complete list of all available commands and options, see the [Rover-CTL CLI Reference](../reference/roverctl/).
:::

### Installation

Download the latest release of Rover-CTL from the [project repository](https://github.com/telekom/controlplane/releases/latest) and add it to your `PATH`:

```bash
curl -LO https://github.com/telekom/controlplane/releases/latest/download/roverctl_Linux_x86_64.tar.gz
tar -xzf roverctl_Linux_x86_64.tar.gz
sudo mv roverctl /usr/local/bin/
```

:::note
The command above is for **Linux (x86_64)**. For Windows, download `roverctl_Windows_x86_64.zip` from the [releases page](https://github.com/telekom/controlplane/releases/latest) instead.
:::

### Authentication

Rover-CTL authenticates using a **Team Token** — a base64-encoded JSON structure containing your environment, group, team, and client credentials. Set it as an environment variable:

```bash
export ROVER_TOKEN="<your-base64-encoded-team-token>"
```

Additional configuration options:

| Variable | Description |
| -------- | ----------- |
| `ROVER_TOKEN` | Your team's authentication token |

### Verify Your Setup

To confirm that Rover-CTL is correctly configured and can reach the platform:

```bash
roverctl get-info
```

This should return a list of your team's existing Rover resources (which will be empty if you are starting fresh).

## Next Steps

- [Managing Applications](./applications.mdx) — Create your first application
- [Exposing APIs](./exposing-apis.mdx) — Publish an API for other teams to use
- [Exposing Events](./exposing-events.mdx) — Publish events for other teams to consume
- [Rover-CTL CLI Reference](../reference/roverctl/) — Explore all available commands and options
