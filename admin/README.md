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
  <a href="#crds">CRDs</a>
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

## CRDs
All CRDs can be found here: [CRDs](./config/crd/bases/).

<p>The Admin domain defines the following Custom Resources (CRDs):</p>

<details>
<summary>
<strong>Environment</strong>
This CRD represents a logical environment for workload separation (e.g., dev, staging, production).
</summary>  

- The Environment CR MUST be created in a namespace that matches its name (e.g., environment `dev` in namespace `dev`).
- The Environment CR MUST have a label `cp.ei.telekom.de/environment` with the value of the environment name.
- Environments are used to logically separate workloads and are referenced by other resources like Zones.
- The environment name is used as a label selector for related resources across the platform.

</details>
<br />

<details>
<summary>
<strong>Zone</strong>
This CRD represents a physical or logical deployment target with gateway and identity provider configurations.
</summary>  

- The Zone CR MUST be created in a namespace and have a label `cp.ei.telekom.de/environment` indicating which environment it belongs to.
- The Zone name MUST match the pattern `^[a-z0-9]+(-?[a-z0-9]+)*$` (lowercase alphanumeric with hyphens).
- Each Zone creates its own dedicated namespace (stored in `status.namespace`) for managing related resources.
- Zones define gateway configuration, identity provider settings, and Redis connection details.
- The `visibility` field controls subscription behavior and can be either `World` or `Enterprise`.
- Zones can optionally define Team APIs through the `teamApis` field, which creates routes on the gateway.
- The Zone controller creates and manages related resources in its handlers.
- All managed resources are labeled with both `cp.ei.telekom.de/environment` and `cp.ei.telekom.de/zone` labels.

</details>
<br />

<details>
<summary>
<strong>RemoteOrganization</strong>
This CRD represents an external Control Plane instance for cross-platform integration.
</summary>  

- The RemoteOrganization CR MUST be created in a namespace and have a label `cp.ei.telekom.de/environment` indicating which environment it belongs to.
- Each RemoteOrganization creates its own dedicated namespace (stored in `status.namespace`) for managing related resources.
- The `spec.id` field uniquely identifies the remote organization.
- The `spec.zone` field references the Zone through which this remote organization is accessed.
- Contains OAuth2 client credentials (`clientId`, `clientSecret`) and issuer URL for authentication with the remote Control Plane.
- Used for federated operations and cross-platform API exposure/subscription.

</details>
<br />
