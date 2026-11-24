---
title: "Environments share one cluster and are isolated virtually"
status: proposed
date: 2026-11-23
origin: backfilled
domains: [all]
governs:
  - "common/pkg/client/client.go"
  - "common/pkg/config/labels.go"
  - "common/pkg/controller/controller.go"
  - "common-server/pkg/server/middleware/security/business_context.go"
  - "common-server/pkg/server/middleware/security/check_access.go"
  - "common-server/pkg/store/store.go"
  - "admin/internal/handler/zone/handlingContext.go"
  - "admin/internal/handler/remoteorganization/handler.go"
  - "organization/api/v1/team_types.go"
  - "organization/internal/handler/team/handler/namespace/build.go"
  - "discovery-server/internal/mapper/util.go"
enforced-by:
  - .agents/rules/13-environment-isolation.md
evidence:
  - common/README.md#virtual-environments
  - docs/docs/architecture/overview.md#virtual-environments
  - docs/docs/admin-journey/environments-and-zones.md#environments
  - docs/docs/admin-journey/operations.md#scaling-considerations
  - docs/docs/admin-journey/controlplane-api.md#running-multiple-environments-in-one-cluster
  - .agents/rules/13-environment-isolation.md
  - common/pkg/client/client.go
  - common/pkg/controller/controller.go
  - common-server/pkg/server/middleware/security/business_context.go
  - common-server/pkg/server/middleware/security/check_access.go
  - common-server/pkg/store/store.go
  - admin/internal/handler/zone/handlingContext.go
  - organization/internal/handler/team/handler/namespace/build.go
  - discovery-server/internal/mapper/util.go
  - 6af3eae7
  - https://github.com/telekom/controlplane/pull/7
---

## Context

A Control Plane installation serves several environments, such as dev, staging and production. Each environment is a tenant: its zones, groups, teams and applications must never see or change those of another environment.

The goal was to run multiple Control Plane instances in a single Kubernetes cluster, because a separate cluster for each environment costs more. In one cluster, the same operators manage the resources of every environment. The environments are therefore isolated only virtually, by the Control Plane itself.

The platform also has to answer questions per environment, such as "list all Rover files in environment foo". Namespaces alone cannot answer that: a team's Rover files live in `foo--my-group--my-team`, not in `foo`.

## Decision

Environments are virtual. They share one cluster, and the Control Plane keeps them apart in its own code.

- **Every resource carries an environment label.** A resource without the label is blocked and not processed.
- **Operators go through a scoped client.** It stamps the label on every resource it writes. It only lists resources with the same label, and it refuses to read or delete a resource from another environment.
- **Namespaces carry the environment name.** Each environment owns a namespace named after it. Namespaces generated for it start with the environment: `<environment>--<zone>` for a zone and `<environment>--<group>--<team>` for a team.
- **REST servers take the environment from the caller's token.** Rover Server and Discovery Server read the `env` claim. They only return stored resources whose namespace starts with that environment.

Code that bypasses the scoped client must scope its own reads and writes. [Rule 13](https://github.com/telekom/controlplane/blob/main/.agents/rules/13-environment-isolation.md) checks this during review.

```mermaid
flowchart TD
    subgraph Cluster["One Kubernetes cluster"]
        Op["Operators"]
        SC["Scoped client"]
        subgraph Dev["Environment dev"]
            DevNs["dev<br/>dev--zone<br/>dev--group--team"]
        end
        subgraph Prod["Environment prod"]
            ProdNs["prod<br/>prod--zone<br/>prod--group--team"]
        end
    end
    Op --> SC
    SC -->|"label<br/>env=dev"| Dev
    SC -.->|"refused"| Prod
```

## Options considered

- **Virtual environments in one shared cluster, marked by a label** (chosen): one cluster costs less than one per environment. The label is convenient, and it finds every resource of an environment with a single lookup.
- **A separate cluster for each environment**: isolates environments without extra code, but lost on cost.
- **Namespaces only, without a label**: generated namespaces such as `foo--my-group--my-team` hide the environment in a name prefix. A lookup like "all Rover files in environment foo" would not be possible.

## Consequences

- A missing environment filter does not fail. It silently reads, enqueues or changes another environment's resources. Rule 13 therefore checks it at review time, as a critical finding.
- The cluster grows with every environment: each zone and each team adds a namespace and its gateway and identity resources.
- Components with their own storage must isolate environments themselves. By default, one ControlPlane API and one Projector serve all environments from one database. Separate pairs per environment are an operator's choice.
