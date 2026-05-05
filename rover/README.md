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
  <a href="#features">Features</a> •
  <a href="#crds">CRDs</a> •
  <a href="#difference-to-rover-server">Difference to Rover Server</a>
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
- **Specification File Management**: Extend API information with APISpecification
- **Roadmap Management**: Track future timeline and milestone information for APIs using the Roadmap CRD
- **Changelog Management**: Track version history and release information for APIs using the ApiChangelog CRD
- **Approval Workflows**: Integrate with the approval domain for subscription requests
- **Traffic Management**: Configure circuit breakers, timeouts, and retry policies

Please take a look at the [Custom Resource Definition](config/crd/bases/rover.cp.ei.telekom.de_rovers.yaml) for more information, what features are supported via Rover, and how to configure them.

## CRDs
All CRDs can be found here: [CRDs](./config/crd/bases/).

<p>The Rover domain defines the following Custom Resources (CRDs):</p>

<details>
<summary>
<strong>Rover</strong>
This CRD represents a complete application configuration including API exposures and subscriptions.
</summary>

- The Rover CR SHOULD be created in the namespace that corresponds to the team's environment.
- The Rover CR name SHOULD match the application name.
- The Rover CR MUST specify a zone where the application will be deployed.
- The Rover CR can define multiple API exposures
- The Rover CR can define multiple API subscriptions
- The Rover status tracks references to all created resources
- The Rover controller translates the Rover CR into resources in other domains (API, Application,...).

</details>
<br />

<details>
<summary>
<strong>ApiSpecification</strong>
This CRD represents an OpenAPI specification for an API exposed through a Rover.
</summary>

- The ApiSpecification CR SHOULD be created in the same namespace as the Rover that exposes the API.
- The ApiSpecification name is generated from the BasePath by removing leading/trailing slashes and replacing slashes with hyphens.
- The ApiSpecification status tracks a reference to the created API resource.
- The ApiSpecification controller extracts metadata from the OpenAPI document to enhance the API resource.

</details>
<br />

<details>
<summary>
<strong>Roadmap</strong>
This CRD represents timeline and future milestone information for APIs.
</summary>

- The Roadmap CR SHOULD be created in the namespace of the team that owns the API.
- The Roadmap CR references an ApiSpecification via specificationRef.
- The Roadmap name is generated from the API basePath with the version suffix removed (e.g., `/eni/payment-service/v1` → `eni-payment-service`).
- Only one Roadmap can exist per API (across all versions), enforced by the name generation logic.
- Roadmap items (date, title, description, optional URL) are stored as JSON in file-manager.
- The CRD stores only a reference to the file-manager content along with a hash for change detection.

</details>
<br />

<details>
<summary>
<strong>EventSpecification</strong>
This CRD represents an AsyncAPI specification for an event exposed through a Rover.
</summary>

- The EventSpecification CR SHOULD be created in the same namespace as the Rover that exposes the event.
- The EventSpecification name is generated from the event type by replacing dots with hyphens.
- The EventSpecification status tracks a reference to the created EventType resource.
- The EventSpecification controller extracts metadata from the AsyncAPI document to enhance the EventType resource.

</details>
<br />

<details>
<summary>
<strong>ApiChangelog</strong>
This CRD represents version history and release information for APIs.
</summary>

- The ApiChangelog CR SHOULD be created in the namespace of the team that owns the API.
- The ApiChangelog CR references an ApiSpecification via specificationRef.
- The ApiChangelog name is generated from the API basePath with the version suffix removed (e.g., `/eni/payment-service/v1` → `eni-payment-service`).
- Only one ApiChangelog can exist per API (across all versions), enforced by the name generation logic.
- Changelog items (date, version, description, optional versionUrl) are stored as JSON in file-manager.
- The date field supports flexible formats like "2024-03-15" or "Q1 2024" for past releases and planned releases.
- The CRD stores only a reference to the file-manager content along with a hash for change detection.

</details>
<br />

## Difference to Rover Server
It provides a REST API for creating and updating Rover resources. It is the intended way for providers and consumers to interact with the controlplane, it offloads onboarding, security checks and more to keep the controller lean.
Furthermore, it enables access to the controlplane without requiring access right to the kubernetes cluster. Please review [rover-server](../rover-server) for more information.
