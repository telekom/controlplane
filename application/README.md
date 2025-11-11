<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

<p align="center">
  <h1 align="center">Application</h1>
</p>

<p align="center">
  Kubernetes operator for managing Tardis applications.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#features">Features</a> •
  <a href="#zone-integration">Zone Integration</a>
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
