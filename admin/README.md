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

## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/).    
Each file contains copyright and license information, and license texts can be found in the [./LICENSES](./LICENSES) folder. For more information visit https://reuse.software/.    
You can find a guide for developers at https://telekom.github.io/reuse-template/.