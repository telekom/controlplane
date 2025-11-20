<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

<p align="center">
  <h1 align="center">Application</h1>
</p>

<p align="center">
  Kubernetes operator for managing applications as an abstract.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#features">Features</a> •
  <a href="#zone-integration">Zone Integration</a> •
  <a href="#crds">CRDs</a>
</p>

## About

The Application operator provides a Kubernetes-native way to manage applications. 
An application is an abstraction representing a users Rover file. 
Once their Rover file is applied, the created Application logically encapsulates all the exposures and subscriptions. 
The Application can also contain an Identity client and Gateway Consumers, that can be used to access Controlplane Server endpoints.

## Features

- **Automatic Resource Provisioning**: Automatically creates Identity clients and Gateway consumers for applications to Access administrative controlplane server endpoints like [rover-server](../rover-server).
- **Multi-Zone Support**: Primary and failover zone configuration for high availability
- **Secret Management Integration**: Seamless integration with Secret Manager for credential handling
- **IP Restriction Support**: Configure IP-based access control for Gateway consumers

The Application operator follows a hierarchical reconciliation pattern for Application resource management.
The operator watches the Application resource and manages the identity client and gateway consumer associated with the application to configure the access point for controlplane server endpoints.

## Zone Integration

Applications support **primary + failover zones** for high availability:

**Key Points**:
- Primary zone is always required
- Failover zones are optional for HA scenarios
- Each zone gets its own Identity Client and Gateway Consumer

## CRDs
All CRDs can be found here: [CRDs](./config/crd/bases/).

<p>The Application domain defines the following Custom Resource (CRD):</p>

<details>
<summary>
<strong>Application</strong>
This CRD represents an application abstraction that encapsulates API exposures and subscriptions.
</summary>  

- The Application CR MUST be created in the namespace of the team that owns the application.
- The Application name SHOULD follow the team's naming convention for applications.
- The Application creates and manages:
  - Identity Client: Created when `needsClient: true` (default) for authentication with Control Plane services
  - Gateway Consumer: Created when `needsConsumer: true` (default) for accessing gateway endpoints
- The client ID `status.clientId` is constructed as `{team}--{application-name}`.
- References to created resources are stored in `status.clients` and `status.consumers`.

</details>
<br />
