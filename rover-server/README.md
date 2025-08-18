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

# About

The Rover Server serves as the primary entrypoint for all customer configurations in the control plane. It exposes a simplified REST API that abstracts away the underlying Kubernetes API complexity.

- Acts as a gateway for all customer configurations
- Provides a simple REST API interface for configuration management. See [OpenAPI Specification](./api/openapi.yaml) for details.
- Handles initial validation and processing of configurations
- Passes validated configurations to the [Rover Domain](../rover/README.md) for reconciliation

# Configuration

The server can be configured using environment variables or configuration files:

- `SECURITY_TRUSTEDISSUERS`: Comma-separated list of trusted issuers for JWT validation
- `SECURITY_LMS_BASEPATH`: Base path for the LMS (Last Mile Security) checking
- `SECURITY_DEFAULTSCOPE`: Default scope if token does not contain one

# Installation

See [kustomize](./config/default/kustomization.yaml) for the default installation configuration. And [installation](../install/kustomization.yaml) for more details on how to deploy it with the entire Controlplane.
