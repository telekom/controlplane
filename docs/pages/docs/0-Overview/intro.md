---
sidebar_position: 1
---

# Controlplane

<div style={{textAlign: 'center'}}>
  <img src="/controlplane/img/eni-tardis-small.png" alt="Open Telekom Integration Platform Logo" width="200"/>
  <h2>Control Plane</h2>
  <p>
    A centralized management layer that maintains the desired state of your systems by orchestrating workloads, scheduling, and system operations through a set of core and custom controllers.
  </p>
</div>

## Table of Contents

<div className="container">
  <div className="row">
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__body">
          <h3>📓 Documentation</h3>
          <ul>
            <li><a href="#about">About Control Plane</a></li>
            <li><a href="#features">Key Features</a></li>
            <li><a href="#components">Component Overview</a></li>
            <li><a href="#architecture">Architecture</a></li>
          </ul>
        </div>
      </div>
    </div>
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__body">
          <h3>🚀 Get Started</h3>
          <ul>
            <li><a href="#getting-started">Getting Started Guide</a></li>
            <li><a href="../2-Installation/quickstart.md">Quickstart Tutorial</a></li>
            <li><a href="../2-Installation/installation.md">Installation Guide</a></li>
            <li><a href="../1-Technology/intro.md">Technology Overview</a></li>
          </ul>
        </div>
      </div>
    </div>
  </div>
</div>

## About

<div className="row">
  <div className="col col--8">
    <p>
      As part of the Open Telekom Integration Platform, the Control Plane is the central management layer that governs the operation of your Kubernetes cluster. It maintains the desired state of the system, manages workloads, and provides interfaces for user interaction and automation.
    </p>
    <p>
      The Control Plane follows a modular architecture with specialized components that work together to provide a complete platform for API management and workload orchestration. It extends the native Kubernetes capabilities through custom controllers and operators that implement domain-specific logic.
    </p>
  </div>
  <div className="col col--4">
    <div className="card">
      <div className="card__body">
        <h3>Why Control Plane?</h3>
        <ul>
          <li>🌐 <b>Unified management</b> across clusters</li>
          <li>🔒 <b>Security by design</b> with OAuth 2.0</li>
          <li>🧲 <b>API-first approach</b> for integration</li>
          <li>🔄 <b>Declarative configuration</b> for consistency</li>
        </ul>
      </div>
    </div>
  </div>
</div>

<hr />

## Features

:::info Key capabilities
The Open Telekom Integration Platform Control Plane supports the complete API lifecycle and enables seamless, cloud-independent integration of services. It provides fine-grained API access control with security by design through OAuth 2.0 and integrated permission management.
:::

Key features of the Control Plane include:


<div className="container">
  <div className="row">
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h3>API Management</h3>
        </div>
        <div className="card__body">
          <p>
            Complete API lifecycle management with cloud-independent service integration. Provides fine-grained access control with OAuth 2.0 security and integrated permission management.
          </p>
        </div>
      </div>
    </div>
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h3>Approval Management</h3>
        </div>
        <div className="card__body">
          <p>
            Secure and auditable access to APIs with features like 4-eyes-principle, approval expiration, recertification, and more.
          </p>
        </div>
      </div>
    </div>
  </div>
  <div className="row">
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h3>Organization and Admin</h3>
        </div>
        <div className="card__body">
          <p>
            Administrative tools for efficient organization management, including zones, gateways, and identity providers.
          </p>
        </div>
      </div>
    </div>
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h3>Team Management</h3>
        </div>
        <div className="card__body">
          <p>
            Comprehensive team management with team creation, member management, and role-based access control.
          </p>
        </div>
      </div>
    </div>
  </div>
  <div className="row">
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h3>Secret Management</h3>
        </div>
        <div className="card__body">
          <p>
            Securely store, access, and distribute sensitive information like passwords, API keys, and certificates. Ensures encryption at rest and secure transmission.
          </p>
        </div>
      </div>
    </div>
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h3>REST APIs</h3>
        </div>
        <div className="card__body">
          <ul>
            <li>Rover API: Manage Rover functionalities</li>
            <li>Approval API: Handle approval processes</li>
            <li>Team API: Manage teams and members</li>
            <li>Catalog API: Access and manage API catalog</li>
            <li>ControlPlane API: Access controlplane resources</li>
          </ul>
        </div>
      </div>
    </div>
  </div>
</div>

<hr />

## Components

:::tip Platform composition
The Control Plane consists of multiple specialized components that work together to provide a complete platform for API management and workload orchestration.
:::

### Resource Hierarchy

:::note Resource model
The Control Plane uses a hierarchical resource model with three main resource types that interact with each other. This model provides clear separation of concerns while enabling powerful resource relationships.
:::

```mermaid
graph TD
    subgraph "Admin Resources"
        Environment[Environment] --> Zone[Zone]
        Zone --> RemoteOrg[Remote Organization]
    end
    
    subgraph "Organization Resources"
        Group[Group] --> Team[Team]
    end
    
    subgraph "Workload Resources"
        Team --> Application[Application]
        Application --> Rover[Rover]
        Rover --> ApiSpec[API Specification]
        ApiSpec --> Api[API]
        Api --> ApiExposure[API Exposure]
        Api --> ApiSubscription[API Subscription]
    end
    
    %% Relationships between resources
    Zone -. "schedules on" .-> Rover
    Team -. "owns" .-> Rover
    
    %% Class definitions for styling
    classDef admin fill:#E20074,color:#ffffff,stroke:#E20074
    classDef org fill:#9B287B,color:#ffffff,stroke:#9B287B
    classDef workload fill:#6BB32E,color:#ffffff,stroke:#6BB32E
    
    %% Apply styles
    class Environment,Zone,RemoteOrg admin
    class Group,Team org
    class Application,Rover,ApiSpec,Api,ApiExposure,ApiSubscription workload
```

1. **Admin Resources**: Platform-level resources managed by administrators
   - Environments: Logical groupings of zones and clusters
   - Zones: Represent deployment targets with specific capabilities
   - RemoteOrganizations: References to organizations in remote clusters

2. **Organization Resources**: Team and project management resources
   - Groups: Logical groupings of teams
   - Teams: Represent development or operational teams with members

3. **Workload Resources**: Application and API resources
   - Rovers: Deployable units that can be scheduled on clusters
   - Applications: Collections of related services
   - APIs: Service interfaces with specifications

### Operators

<div className="row">
  <div className="col col--8">
    <p>
      In addition to the core components, the control plane may also run custom operators. These are specialized control loops designed to manage complex domain-specific applications and configurations. These operators extend Kubernetes functionality using the <a href="https://kubernetes.io/docs/concepts/extend-kubernetes/operator/">Operator pattern</a>, combining custom resource definitions (CRDs) with controllers that automate lifecycle management.
    </p>
    <p>
      Each operator encapsulates a distinct domain of responsibility, operating independently with minimal interdependencies, which promotes modularity, simplifies maintenance, and enhances the scalability of the overall control plane architecture.
    </p>
  </div>
  <div className="col col--4">
    <div className="card">
      <div className="card__header">
        <h4>Operator Benefits</h4>
      </div>
      <div className="card__body">
        <ul>
          <li>🔧 <b>Domain-specific logic</b></li>
          <li>🛠️ <b>Automated lifecycle management</b></li>
          <li>🔗 <b>Minimal interdependencies</b></li>
          <li>💡 <b>Specialized expertise</b></li>
        </ul>
      </div>
    </div>
  </div>
</div>

The following operators run on the control plane:
- [Rover Operator](https://github.com/telekom/controlplane/blob/main/rover/README.md): Manages the lifecycle of Rover-domain resources such as Rovers and ApiSpecifications.
- [Application Operator](https://github.com/telekom/controlplane/blob/main/application/README.md): Manages the lifecycle of resources of kind Application.
- [Admin Operator](https://github.com/telekom/controlplane/blob/main/admin/README.md): Manages the lifecycle of Admin-domain resources such as Environments, Zones and RemoteOrganizations.
- [Organization Operator](https://github.com/telekom/controlplane/blob/main/organization/README.md): Manages the lifecycle of Organization-domain resources such as Groups and Teams.
- [Api Operator](https://github.com/telekom/controlplane/blob/main/api/README.md): Manages the lifecycle of API-domain resources such as Apis, ApiExposures, ApiSubscriptions and RemoteApiSubscriptions.
- [Gateway Operator](https://github.com/telekom/controlplane/blob/main/gateway/README.md): Manages the lifecycle of Gateway-domain resources such as Gateways, Gateway-Realms, Consumers, Routes and ConsumerRoutes.
- [Identity Operator](https://github.com/telekom/controlplane/blob/main/identity/README.md): Manages the lifecycle of Identity-domain resources such as IdentityProviders, Identity-Realms and Clients.
- [Approval Operator](https://github.com/telekom/controlplane/blob/main/approval/README.md): Manages the lifecycle of resources of kind Approval.

These operators work alongside the Kubernetes API server and etcd, watching for changes to custom resources and ensuring the actual state of their managed components aligns with the desired configuration.

### API Servers

<div className="row">
  <div className="col col--7">
    <p>
      API Servers are RESTful APIs for managing Kubernetes custom resources. They provide standardized HTTP-based interfaces to create, read, update, and delete (CRUD) custom-defined objects within the Kubernetes cluster. 
    </p>
    <p>
      These custom resources are typically defined using Custom Resource Definitions (CRDs) and extend the Kubernetes API with domain-specific objects (e.g., Application, Gateway, Organization). The API follows REST principles and standard HTTP methods (GET, POST, PUT, DELETE) to interact with resources. It supports authentication and authorization, enabling automation and integration with UIs and external systems.
    </p>
  </div>
  <div className="col col--5">
    <div className="card">
      <div className="card__header">
        <h4>API Server Capabilities</h4>
      </div>
      <div className="card__body">
        <ul>
          <li>🔐 <b>Authentication & Authorization</b></li>
          <li>🔄 <b>CRUD operations</b> on custom resources</li>
          <li>📚 <b>OpenAPI specifications</b></li>
          <li>🔌 <b>Integration</b> with external systems</li>
        </ul>
      </div>
    </div>
  </div>
</div>

:::info API Servers
The following API Servers run on the control plane:
:::

<div className="container">
  <div className="row">
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h4>Secret Manager</h4>
        </div>
        <div className="card__body">
          <p>
            RESTful API for managing secrets. It allows you to store, retrieve, and delete secrets securely.
          </p>
          <a href="https://github.com/telekom/controlplane/blob/main/secret-manager/README.md">Documentation →</a>
        </div>
      </div>
    </div>
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h4>Rover Server</h4>
        </div>
        <div className="card__body">
          <p>
            RESTful API for managing Rover resources such as Rover Exposures and Subscriptions as well as ApiSpecifications.
          </p>
          <a href="https://github.com/telekom/controlplane/blob/main/rover-server/README.md">Documentation →</a>
        </div>
      </div>
    </div>
  </div>
  <div className="row">
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h4>Organization Server</h4>
        </div>
        <div className="card__body">
          <p>
            RESTful API for managing Organization resources such as Groups and Teams.
          </p>
          <a href="https://github.com/telekom/controlplane/blob/main/organization-server/README.md">Documentation →</a>
        </div>
      </div>
    </div>
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h4>Controlplane API</h4>
        </div>
        <div className="card__body">
          <p>
            RESTful API for reading custom resources from the control plane from all domains.
          </p>
          <a href="https://github.com/telekom/controlplane/blob/main/cpapi/README.md">Documentation →</a>
        </div>
      </div>
    </div>
  </div>
</div>

### Libraries

:::note Shared code
The Control Plane uses several shared libraries to provide common functionality across different components.
:::

<div className="container">
  <div className="row">
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h4>Common</h4>
        </div>
        <div className="card__body">
          <p>
            A core library that provides shared code and utilities used across the different Control Plane projects. Includes common types, helpers, and utilities.
          </p>
          <a href="https://github.com/telekom/controlplane/blob/main/common/README.md">Documentation →</a>
        </div>
      </div>
    </div>
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h4>Common-Server</h4>
        </div>
        <div className="card__body">
          <p>
            Module used to dynamically create REST-APIs for Kubernetes-CRDs. Provides consistent API generation and handling across multiple components.
          </p>
          <a href="https://github.com/telekom/controlplane/blob/main/common-server/README.md">Documentation →</a>
        </div>
      </div>
    </div>
  </div>
</div>

### Infrastructure Components

:::info Required components
The Control Plane requires the following infrastructure components to operate correctly. These components provide essential services for the platform's functionality.
:::

<div className="container">
  <div className="row">
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h4>⎈ Kubernetes</h4>
        </div>
        <div className="card__body">
          <p>The underlying platform where the Control Plane is deployed. Currently tested with Kubernetes version 1.31.</p>
          <a href="https://kubernetes.io/">Learn more →</a>
        </div>
      </div>
    </div>
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h4>🔒 cert-manager</h4>
        </div>
        <div className="card__body">
          <p>Creates and manages TLS certificates for workloads in your Kubernetes cluster.</p>
          <a href="https://cert-manager.io/docs/">Learn more →</a>
        </div>
      </div>
    </div>
  </div>
  <div className="row">
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h4>🔐 trust-manager</h4>
        </div>
        <div className="card__body">
          <p>Manages trust bundles in Kubernetes clusters.</p>
          <a href="https://cert-manager.io/docs/trust/trust-manager/">Learn more →</a>
        </div>
      </div>
    </div>
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h4>📈 Prometheus CRDs</h4>
        </div>
        <div className="card__body">
          <p>Enables monitoring based on Prometheus, required by the kubebuilder framework.</p>
          <a href="https://book.kubebuilder.io/reference/metrics">Learn more →</a>
        </div>
      </div>
    </div>
  </div>
  <div className="row">
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h4>🔎 Gateway (Kong)</h4>
        </div>
        <div className="card__body">
          <p>A Kong-based managed gateway providing hybrid API management.</p>
          <a href="https://github.com/telekom/gateway-kong-charts">GitHub →</a> | <a href="https://konghq.com/products/kong-gateway">Kong →</a>
        </div>
      </div>
    </div>
    <div className="col col--6">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h4>👤 Iris (Keycloak)</h4>
        </div>
        <div className="card__body">
          <p>A Keycloak-based Machine-to-Machine (M2M) Identity Provider for authentication and authorization.</p>
          <a href="https://github.com/telekom/identity-iris-keycloak-charts">GitHub →</a> | <a href="https://www.keycloak.org/">Keycloak →</a>
        </div>
      </div>
    </div>
  </div>
</div>

<hr />

## Architecture

:::info Modular design
The Control Plane follows a modular architecture with specialized components that work together to provide a complete platform for API management and workload orchestration. This approach enables extensibility, maintainability, and scalability.
:::

### Component Interactions

The diagram below illustrates how the different components of the Control Plane interact with each other and with external systems:

<div className="container">
  <div className="row">
    <div className="col">
      <div className="card">
        <div className="card__header">
          <h3>Component Architecture</h3>
        </div>
        <div className="card__body">
          <p>The Control Plane consists of operators, API servers, and libraries that interact with Kubernetes and external infrastructure components:</p>
          <img src="/controlplane/img/CP_Architecture.drawio.svg" alt="Architecture Diagram" style={{maxWidth: '100%', height: 'auto'}} />
        </div>
        <div className="card__footer">
          <p><strong>Key interactions:</strong></p>
          <ul>
            <li>Operators use the Kubernetes API to manage custom resources</li>
            <li>API Servers provide RESTful interfaces for clients and services</li>
            <li>Integration with external components like Gateway and Identity Providers</li>
          </ul>
        </div>
      </div>
    </div>
  </div>
</div>

<hr />

## Getting Started

:::tip Start your journey
The Control Plane can be approached in different ways depending on your needs. Choose the path that suits you best.
:::

<div className="container">
  <div className="row">
    <div className="col col--4">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h3>🧠 Learn</h3>
        </div>
        <div className="card__body">
          <p>
            Understand the technologies and frameworks used in the Control Plane and how they work together.
          </p>
          <ul>
            <li>Core technologies overview</li>
            <li>Architecture principles</li>
            <li>Component interactions</li>
          </ul>
        </div>
        <div className="card__footer">
          <a href="../1-Technology/intro.md" className="button button--primary button--block">Technology Overview</a>
        </div>
      </div>
    </div>
    <div className="col col--4">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h3>🚀 Try</h3>
        </div>
        <div className="card__body">
          <p>
            Set up a local development environment quickly to try out the Control Plane functionality.
          </p>
          <ul>
            <li>5-minute local setup</li>
            <li>Sample resources</li>
            <li>Quick verification</li>
          </ul>
        </div>
        <div className="card__footer">
          <a href="../2-Installation/quickstart.md" className="button button--primary button--block">Quickstart Guide</a>
        </div>
      </div>
    </div>
    <div className="col col--4">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h3>🛠️ Install</h3>
        </div>
        <div className="card__body">
          <p>
            Get detailed instructions for installing the Control Plane in a Kubernetes environment.
          </p>
          <ul>
            <li>Production setup</li>
            <li>Configuration options</li>
            <li>Integration guides</li>
          </ul>
        </div>
        <div className="card__footer">
          <a href="../2-Installation/installation.md" className="button button--primary button--block">Installation Guide</a>
        </div>
      </div>
    </div>
  </div>
</div>

<hr />

## Next Steps

:::tip Ready to dive deeper?
Now that you've explored the Control Plane overview, take your next steps with these resources:
:::

<div className="container">
  <div className="row">
    <div className="col col--4">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h3>📚 Documentation</h3>
        </div>
        <div className="card__body">
          <ul>
            <li><a href="../1-Technology/intro.md">Technology Guides</a></li>
            <li><a href="../2-Installation/installation.md">Installation Guide</a></li>
            <li><a href="../2-Installation/quickstart.md">Quickstart Guide</a></li>
          </ul>
        </div>
      </div>
    </div>
    <div className="col col--4">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h3>💻 Development</h3>
        </div>
        <div className="card__body">
          <ul>
            <li><a href="https://github.com/telekom/controlplane">GitHub Repository</a></li>
            <li><a href="https://github.com/telekom/controlplane/blob/main/CONTRIBUTING.md">Contributing Guide</a></li>
            <li><a href="https://github.com/telekom/controlplane/issues">Issue Tracker</a></li>
          </ul>
        </div>
      </div>
    </div>
    <div className="col col--4">
      <div className="card margin-bottom--lg">
        <div className="card__header">
          <h3>👨‍💻 Community</h3>
        </div>
        <div className="card__body">
          <ul>
            <li><a href="https://stackoverflow.com/questions/tagged/controlplane">Stack Overflow</a></li>
            <li><a href="https://github.com/telekom/controlplane/discussions">GitHub Discussions</a></li>
            <li><a href="https://github.com/telekom/controlplane/blob/main/CODE_OF_CONDUCT.md">Code of Conduct</a></li>
          </ul>
        </div>
      </div>
    </div>
  </div>
</div>

<hr />

## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](https://github.com/telekom/controlplane/blob/main/CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/telekom/controlplane/blob/main/CODE_OF_CONDUCT.md) at all times.

<hr />

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/).    
Each file contains copyright and license information, and license texts can be found in the [./LICENSES](https://github.com/telekom/controlplane/tree/main/LICENSES) folder. For more information visit https://reuse.software/.    
You can find a guide for developers at https://telekom.github.io/reuse-template/.