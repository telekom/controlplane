<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0    
-->

<p align="center">
  <img src="./docs/img/Open-Telekom-Integration-Platform_Visual.svg" alt="Open Telekom Integration Platform logo" width="200">
  <h1 align="center">Control Plane</h1>
</p>

<p align="center">
 A centralized management layer that maintains the desired state of your systems by orchestrating workloads, scheduling, and system operations through a set of core and custom controllers.
</p>

<p align="center">
  <a href="#features">Features</a> •
  <a href="#components"> Components</a> •
  <a href="#getting-started">Getting started</a>
</p>

## About

As Part of Open TeleKom Integration Platform, the Control Plane is the central management layer that governs the operation of your Kubernetes cluster. It maintains the desired state of the system, manages workloads, and provides interfaces for user interaction and automation.

The Control Plane components run on one or more nodes in the cluster and coordinate all cluster activities, including scheduling, monitoring, and responding to events.  


## Features

The Open Telekom Integration Platform Control Plane supports the whole API lifecycle and allows seamless, cloud-independent integration of services. Further, it enables a fine-grained and vigilant API access control. The communication is secure by design, utilizing OAuth 2.0 and an integrated permission management. 

Key features of the Control Plane include:  


<details>
<summary><strong>API Management</strong></summary>  
Control Plane supports the whole API lifecycle and allows seamless, cloud-independent integration of services. Further, it enables a fine-grained and vigilant API access control. The communication is secure by design, utilizing OAuth 2.0 and an integrated permission management. 
</details>
<br />
<details>
<summary><strong>Approval Management</strong></summary>  
It provides secure and auditable access to APIs, with features like 4-eyes-principle, approval expiration, recertification, and more.
</details>
<br />
<details>
<summary><strong>Organization and Admin Mechanism</strong></summary>  
Provides Administrative tools for efficient organization management, including zones, gateways, and identity providers.
</details>
<br />
<details>
<summary><strong>Team Management:</strong></summary>  
Provides team management capabilities within the control plane
</details>
<br />
<details>
<summary><strong>Secret-Management</strong></summary>
Secret management involves securely storing, accessing, and distributing sensitive information such as passwords, API keys, and certificates within a Kubernetes cluster. It ensures that secrets are encrypted at rest and transmitted securely, while limiting access to only authorized workloads and users.  
</details>
<br />
<details>
<summary><strong>REST APIs for Key Actions</strong></summary> 
* Rover API: API to interact with and manage Rover functionalities.
* Approval API: API for handling approval processes and workflows.
* Team API: API for team management and related actions.
* Catalog API: API to access and manage an API catalog
* ControlPlane API
</details>

## Components

### Operators
In addition to the core components, the control plane may also run custom operators. These are specialized control loops designed to manage complex domain-specific applications and configurations. These operators extend Kubernetes functionality using the [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/), combining custom resource definitions (CRDs) with controllers that automate lifecycle management.

The following operators run on the control plane:
- [Rover Operator](./rover): Manages the lifecycle of Rover-domain resources such as Rovers and ApiSpecifications.
- [Application Operator](./application) Manages the lifecycle of resources of kind Application.
- [Admin Operator](./admin): Manages the lifecycle of Admin-domain resources such as Environments, Zones and RemoteOrganizations.
- [Organization Operator](./organization):  Manages the lifecycle of Organization-domain resources such as Groups and Teams.
- [Api Operator](./api):  Manages the lifecycle of API-domain resources such as Apis, ApiExposures, ApiSubscriptions and RemoteApiSubscriptions.
- [Gateway Operator}](./gateway):  Manages the lifecycle of Gateway-domain resources such as Gateways, Gateway-Realms, Consumers, Routes and ConsumerRoutes.
- [Identity Operator](./identity):  Manages the lifecycle of Identity-domain resources such as IdentityProviders, Identity-Realms and Clients.
- [Approval Operator](./approval):  Manages the lifecycle of resources of kind Approval.

These operators work alongside the Kubernetes API server and etcd, watching for changes to custom resources and ensuring the actual state of their managed components aligns with the desired configuration.

### API Servers
- [Secret Manager](./secret-manager): RESTful API for managing secrets. It allows you to store, retrieve, and delete secrets securely.
- [Rover-Server](./rover-server): RESTful API for managing Rover resources such as Rover Exposures and Subscriptions as well as ApiSpecifications
- [Organization-Server](./organization-server): RESTful API for managing Organization resources such as Groups and Teams
- [Controlplane API](./controlplane-api): RESTful API for reading custom resources from the control plane from all domains 

### Libraries
- [Common](./common): A library that provides shared code between the different projects
- [Common-Server](./common-server): Module used to dynamically create REST-APIs for Kubernetes-CRDs.

### Infrastructure

The Control Plane requires the following infrastructure components in order to operate correctly:

- **Kubernetes**: The Open Telekom Integration Platform Control Plane is designed to be deployed on Kubernetes. Currently, it is tested with Kubernetes version 1.31.
- **cert-manager**: [cert-manager](https://cert-manager.io/docs/) creates TLS certificates for workloads in your Kubernetes cluster and renews the certificates before they expire.
- **trust-manager**: [trust-manager](https://cert-manager.io/docs/trust/trust-manager/) is the easiest way to manage trust bundles in Kubernetes clusters.
**Prometheus CRDs**: By default, Prometheus based monitoring is enabled and hence needs the CRDs to be installed in the Control Plane
- **API Management component**
- **Identity Management component**

## Architecture
The diagram below shows the general flow and interfaces between the most important components of The Rover Control Plane.
# ![Architecture](./docs//img/CP_Architecture.drawio.svg)

## Getting started
A comprehensive guide on how to get started with the installation of the Open Telekom Integration Platform is available here: [Getting Started](./docs/files/getting_started.md)

## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

By participating in this project, you agree to abide by its [Code of Conduct](./CODE_OF_CONDUCT.md) at all times.

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/).    
Each file contains copyright and license information, and license texts can be found in the [./LICENSES](./LICENSES) folder. For more information visit https://reuse.software/.    
You can find a guide for developers at https://telekom.github.io/reuse-template/.