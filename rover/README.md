<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

<p align="center">
  <h1 align="center">Rover</h1>
</p>

<p align="center">
  The Rover domain provides the user-facing API for defining and managing API exposures and subscriptions through declarative Rover files.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#features">Features</a>
</p>

## About

The Rover domain is the primary entry point for users to interact with the Control Plane. Users define their API exposures and subscriptions in a Rover file, which the operator translates into the appropriate resources across other domains (API, Application, Gateway, Identity).

A Rover resource represents a complete application configuration, including:
- API exposures with upstream configurations
- API subscriptions to consume other APIs
- Traffic management (rate limiting, load balancing)
- Approval requirements for subscriptions


## Features

- **Declarative API Management**: Define API exposures and subscriptions in a single Rover file
- *Specification File Management**: Extend API information with APISpecification
- **Approval Workflows**: Integrate with the approval domain for subscription requests
- **Traffic Management**: Configure circuit breakers, timeouts, and retry policies

Please take a look at the [Custom Resource Definition](config/crd/bases/rover.cp.ei.telekom.de_rovers.yaml) for more information, what features are supported via Rover, and how to configure them.

## Difference to Rover Server
It provides a REST API for creating and updating Rover resources. It is the intended way for providers and consumers to interact with the controlplane, it offloads onboarding, security checks and more to keep the controller lean.
Furthermore, it enables access to the controlplane without requiring access right to the kubernetes cluster. Please review [rover-server](../rover-server) for more information.

