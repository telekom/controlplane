<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

<p align="center">
  <h1 align="center">Rover Server</h1>
</p>

<p align="center">
  The Rover Server is the API gateway for the control plane, providing a simplified REST interface
  for managing API exposures and event configurations.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#configuration">Configuration</a> •
  <a href="#installation">Installation</a>
</p>

## About

The Rover Server serves as the primary entrypoint for all customer configurations in the control plane. It exposes a simplified REST API that abstracts away the underlying Kubernetes API complexity.

- Acts as a gateway for all customer configurations
- Provides a simple REST API interface for configuration management. See [OpenAPI Specification](./api/openapi.yaml) for details.
- Handles initial validation and processing of configurations
- Passes validated configurations to the [Rover Domain](../rover/README.md) for reconciliation

## Configuration

The server can be configured using environment variables or configuration files:

- `SECURITY_TRUSTEDISSUERS`: Comma-separated list of trusted issuers for JWT validation
- `SECURITY_LMS_BASEPATH`: Base path for the LMS (Last Mile Security) checking
- `SECURITY_DEFAULTSCOPE`: Default scope if token does not contain one
- `DATABASE_FILEPATH`: This enables the database to store data also in the filesystem. If empty, the database will be in-memory only.

## Installation

See [kustomize](./config/default/kustomization.yaml) for the default installation configuration. And [installation](../install/kustomization.yaml) for more details on how to deploy it with the entire Controlplane.


## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/).    
Each file contains copyright and license information, and license texts can be found in the [./LICENSES](./LICENSES) folder. For more information visit https://reuse.software/.    
You can find a guide for developers at https://telekom.github.io/reuse-template/.