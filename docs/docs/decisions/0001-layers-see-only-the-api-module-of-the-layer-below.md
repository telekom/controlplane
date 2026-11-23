---
title: "Layers see only the API module of the layer below"
status: proposed
date: 2026-11-23
origin: backfilled
domains: [rover, rover-server, rover-ctl, api, event, file, agentic, approval, application, organization, permission, spectre, projector, controlplane-api, discovery-server, gateway, identity, pubsub, sftp]
governs:
  - "rover/**/go.mod"
  - "rover-server/**/go.mod"
  - "rover-ctl/**/go.mod"
  - "api/**/go.mod"
  - "event/**/go.mod"
  - "file/**/go.mod"
  - "agentic/**/go.mod"
  - "approval/**/go.mod"
  - "application/**/go.mod"
  - "organization/**/go.mod"
  - "permission/**/go.mod"
  - "spectre/**/go.mod"
  - "projector/**/go.mod"
  - "controlplane-api/**/go.mod"
  - "discovery-server/**/go.mod"
  - "gateway/**/go.mod"
  - "identity/**/go.mod"
  - "pubsub/**/go.mod"
  - "sftp/**/go.mod"
  - ".github/ci/module-graph.json"
evidence:
  - docs/docs/architecture/overview.md#layer-architecture
  - docs/docs/architecture/overview.md#domain-interaction-model
  - docs/static/img/highlevel-architecture.svg
  - docs/docs/overview/components.md
  - docs/docs/architecture/api.mdx
  - docs/docs/architecture/event.mdx
  - docs/docs/architecture/file.mdx
  - docs/docs/architecture/agentic.mdx
  - docs/docs/architecture/pubsub.mdx
  - docs/docs/architecture/spectre.mdx
  - docs/docs/architecture/controlplane-api.mdx
  - discovery-server/internal/server/deprecated.go
  - .github/ci/module-graph.json
  - https://github.com/telekom/controlplane/pull/306
---

## Context

A customer describes what they want in a Rover file: APIs to expose, subscriptions, events, files. In the end, that wish has to become concrete settings on the components that carry the traffic, such as the API gateway, the identity provider and the event backbone. In between, the platform has to check the request, run approvals and resolve how the pieces depend on each other. Alongside, user interfaces need to see the resulting platform state without being able to change it.

## Decision

The Control Plane splits this path into three layers, and a request always moves through them in the same order. A fourth layer reads beside them, and a few services are shared by all.

- **Customer-Config layer.** The entry point for customers: the Rover file, Rover Server and Rover-CTL. It receives what the customer submits and does most of the validation, so that only well-formed requests travel further.
- **Logic layer.** The business rules: applications, organizations and permissions, API, event and file exposures and subscriptions, MCP servers, listeners, and approvals. It decides what the customer may get and resolves dependencies between domains.
- **Runtime-Config layer.** Turns the result into settings on the actual components: gateway routes and consumers, identity clients, publish and subscribe configuration, SFTP access.

Each layer hands over by creating resources that the next layer processes. A layer only sees the API module of the layer directly below it. It never skips a layer, never reaches into another domain's internals, and never looks upward. Domains within the same layer also talk to each other only through their API modules.

The UI layer, made up of the ControlPlane API, the Projector and the Discovery Server, sits next to the Customer-Config layer and follows the same rule: it sees only the Logic layer's API modules. It only reads. It never changes anything in the cluster, and no other layer depends on it, so the read side cannot leak into the business rules.

Administration, notifications and secret management sit outside the chain. They are shared by all domains in every layer.

```mermaid
flowchart TD
    subgraph SH["Shared by all layers"]
        Shared["Admin<br/>Notification<br/>Secret Manager"]
    end
    subgraph CC["Customer-Config layer"]
        Rover["Rover file<br/>Rover Server<br/>Rover-CTL"]
    end
    subgraph UI["UI layer"]
        Read["ControlPlane API<br/>Projector<br/>Discovery Server"]
    end
    subgraph LG["Logic layer"]
        Logic["Application<br/>Organization<br/>Permission<br/>API · Event · File<br/>Agentic · Spectre<br/>Approval"]
    end
    subgraph RT["Runtime-Config layer"]
        Runtime["Gateway · Identity<br/>PubSub · SFTP"]
    end
    CC -->|"creates resources<br/>via the API module"| LG
    UI -.->|"reads via<br/>the API module"| LG
    LG -->|"creates resources<br/>via the API module"| RT
    RT -->|configures| DataPlane["Gateway<br/>Identity provider<br/>Event backbone<br/>SFTP"]
```

## Options considered

- **Three layers, each seeing only the next layer's API module** (chosen): keeps the system modular, maintainable and easy to extend, and does most of the checking of customer input at the entry point.

No alternatives were evaluated. The layering was the initial design, and the team went with it.
