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
  <a href="#workflow">Workflow</a> •
  <a href="#zone-integration">Zone Integration</a> •
</p>

## About

The Application operator provides a Kubernetes-native way to manage applications. 
An application is an abstraction representing a users Rover file. Once their Rover file is applied, the created Application logically encapsulates all the exposures and subscriptions. The Application can also contain an Identity client, that can be used to access the subscriptions.


## Workflow

The Application operator follows a hierarchical reconciliation pattern for Application resource management:

1. **Application Reconciliation**: The operator watches the Application resource and periodically adjust the cluster's configuration. This includes actions like managing the Identity client and Gateway consumer associated with the Application and rotating the secret. 

The controller implements a declarative approach, continuously reconciling the desired state (defined in the CRs) with the actual state in the cluster. It handles retries and error conditions to ensure eventual consistency.

## Zone Integration

Applications support **primary + failover zones** for high availability:

**Key Points**:
- Primary zone is always required
- Failover zones are optional for HA scenarios
- Each zone gets its own Identity Client and Gateway Consumer
