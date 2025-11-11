<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: CC0-1.0    
-->

<p align="center">
  <h1 align="center">Admin</h1>
</p>

<p align="center">
  The Admin domain manages platform-level resources including environments, zones, and remote organizations.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#features">Features</a> •
</p>

## About

The Admin domain is responsible for managing the foundational infrastructure resources of the Control Plane. It provides the administrative layer that defines where and how applications can be deployed and accessed.

This domain is typically managed by platform administrators and provides the configuration for:
- **Environments**: Logical separation of workloads (e.g., dev, staging, production)
- **Zones**: Physical or logical deployment targets with their gateway and identity provider configurations
- **Remote Organizations**: External Control Plane instances for cross-platform integration

## Features

- **Environment Management**: Define and manage logical environments for API/Application/... separation
- **Zone Configuration**: Configure deployment zones with gateway, redis and identity provider settings
- **Remote Organization Integration**: Connect to external Control Plane instances for federated operations
- **Infrastructure Abstraction**: Provide a unified interface for underlying infrastructure components

