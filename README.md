<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0    
-->

<p align="center">
  <img src="docs/pages/static/img/Open-Telekom-Integration-Platform_Visual.svg" alt="Open Telekom Integration Platform logo" width="200">
  <h1 align="center">Control Plane</h1>
</p>

<p align="center">
 A centralized management layer that maintains the desired state of your systems by orchestrating workloads, scheduling, and system operations through a set of core and custom controllers.
</p>

## About

As part of the [Open Telekom Integration Platform](https://github.com/telekom), the Control Plane is the central management layer that governs the operation of your Kubernetes cluster. It maintains the desired state of the system, manages workloads, and provides interfaces for user interaction and automation.

Built on the [Kubernetes Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/), the Control Plane extends native Kubernetes with custom controllers and resources to provide a complete platform for API management, identity, gateway configuration, and organizational governance. It enables teams to declaratively define and expose APIs, manage subscriptions with approval workflows, and integrate with external systems like Kong Gateway and Keycloak — all via Kubernetes-native custom resources.

## Key Features

- **API Lifecycle Management** — Declaratively register, expose, and subscribe to APIs using Rover files or the REST API. Supports full lifecycle from creation to deprecation.
- **Approval Workflows** — Configurable approval strategies (auto-approve, single-approver, four-eyes principle) with expiration, recertification, and audit trail.
- **API Gateway Integration** — Automatic configuration of Kong Gateway with routes, consumers, rate limiting, JWT/OAuth2 authentication, and request/response transformation.
- **Identity & Access Management** — Integration with Keycloak for service account provisioning, realm management, and OAuth2/OIDC token validation.
- **Organization & Team Management** — Hierarchical group and team structure with automatic namespace provisioning and role-based access control.
- **Secret & File Management** — Secure secret storage with pluggable backends (Kubernetes Secrets, Conjur) and S3-compatible file storage for OpenAPI specifications.
- **Notification System** — Multi-channel event notifications via Email, Microsoft Teams, and Webhooks with customizable templates.
- **Declarative Configuration** — All resources are managed as Kubernetes custom resources, enabling GitOps workflows and infrastructure-as-code practices.

## Architecture

The Control Plane follows a modular architecture organized into three categories:

### Operators (Kubernetes Controllers)

| Operator | Responsibility |
|----------|---------------|
| [admin](admin/) | Platform-level resources: Environments, Zones, Remote Organizations |
| [api](api/) | API lifecycle: APIs, Exposures, Subscriptions, Categories |
| [application](application/) | Application abstraction with Identity/Gateway provisioning |
| [approval](approval/) | Approval workflows for API subscription requests |
| [gateway](gateway/) | Kong Gateway configuration: Routes, Consumers, Realms |
| [identity](identity/) | Keycloak integration: Clients, Realms, Identity Providers |
| [organization](organization/) | Team & Group management with namespace auto-provisioning |
| [rover](rover/) | Declarative user-facing API for exposures and subscriptions |
| [notification](notification/) | Event-driven notifications via Email, Teams, Webhook |

### API Servers (REST APIs)

| Server | Responsibility |
|--------|---------------|
| [rover-server](rover-server/) | REST API for managing Rover exposures, subscriptions, and API specs |
| [secret-manager](secret-manager/) | RESTful secret storage and retrieval |
| [file-manager](file-manager/) | File storage for OpenAPI specifications (S3/MinIO backend) |
| [cpapi](cpapi/) | Read-only REST API across all Control Plane domains |

### Shared Libraries

| Library | Responsibility |
|---------|---------------|
| [common](common/) | Shared controller utilities, error handling, and conditions |
| [common-server](common-server/) | HTTP server library with CRUD, OAuth2, and audit logging |

### CLI Tools

| Tool | Responsibility |
|------|---------------|
| [rover-ctl](rover-ctl/) | CLI for CI/CD-friendly access to Rover Server |

## Technology Stack

| Category | Technologies |
|----------|-------------|
| **Language** | Go 1.24+ |
| **Framework** | Kubernetes, Kubebuilder, controller-runtime |
| **HTTP** | Fiber v2, OAPI-Codegen |
| **Gateway** | Kong Gateway |
| **Identity** | Keycloak (OAuth2/OIDC) |
| **Storage** | Kubernetes etcd (CRDs), S3/MinIO, Redis, Conjur |
| **Testing** | Ginkgo, Gomega, Testify, go-snaps, Mockery |
| **Deployment** | Kustomize, Helm |
| **Documentation** | Docusaurus 3, OpenAPI/Swagger |

## Documentation

For complete documentation, please visit the Control Plane documentation site:

- [Control Plane Documentation](https://telekom.github.io/controlplane/)

The documentation includes:

- [Overview and Architecture](https://telekom.github.io/controlplane/docs/Overview/controlplane)
- [Component Details](https://telekom.github.io/controlplane/docs/Overview/components)
- [Operators](https://telekom.github.io/controlplane/docs/Overview/operators)
- [Technology Overview](https://telekom.github.io/controlplane/docs/Technology/technology)
- [Installation Guide](https://telekom.github.io/controlplane/docs/Installation/installation)

## Getting Started

To quickly get started with Control Plane:

```bash
# Clone the repository
git clone https://github.com/telekom/controlplane.git

# Navigate to the local installation directory
cd controlplane/install/local

# Install Control Plane components
kubectl apply -k .
```

For detailed installation instructions and configuration options, refer to the [Installation Guide](https://telekom.github.io/controlplane/docs/Installation/installation).

## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/).    
Each file contains copyright and license information, and license texts can be found in the [./LICENSES](./LICENSES) folder. For more information visit https://reuse.software/.    
You can find a guide for developers at https://telekom.github.io/reuse-template/.