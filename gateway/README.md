<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: CC0-1.0    
-->

<p align="center">
  <h1 align="center">Gateway</h1>
</p>

<p align="center">
  The Gateway domain is responsible for configuring the API Gateway at runtime. 
  It provides an API to manage Routes and their Consumers.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#features">Features</a> •
  <a href="#Architecture">Architecture</a> •
  <a href="#crds">CRDs</a>
</p>


## About

This repository contains the implementation of the Gateway domain, which is responsible for configuring the API Gateway at runtime.
The API is designed to be independent of the underlying API Gateway technology, allowing for flexibility in choosing the best solution for your needs.
However, at the moment, the implementation is tightly coupled with the [Kong Gateway](https://docs.konghq.com/gateway/latest/).

<div align="center">
    <img src="docs/overview.drawio.svg" />
</div>

## Features

- **Route Management**: Manage routes and their configurations
- **Consumer Management**: Manage consumers and their access to routes
- **Realm Management**: Support for virtual environments to allow for virtualization of the API Gateway deployments
- **Rate Limiting**: Control the rate of requests to your APIs (configured via Rover domain)
- **Load Balancing**: Distribute incoming requests across multiple upstream instances (configured via Rover domain)
- **JWT Authentication**: OAuth2/OIDC authentication with Keycloak integration
- **...**

> [!Note]
> For a full list of gateway features, see the contents of [./internal/features/feature](./internal/features/feature).

## Architecture

### Features

Features are the core components of the Gateway domain, providing the functionality to configure the actual API Gateway.

Each `Feature` needs to implement the [`Feature` interface](internal/features/builder.go).

The so-called `FeaturesTypes` that are implemented are listed in [api/v1/features.go](api/v1/features.go) as constants.
These are used to identify the features during the configuration process.

> [!NOTE]
> New features should have unique `FeatureType` constants in [api/v1/features.go](api/v1/features.go) and referenced when using the [Feature-Builder](#feature-builder).


#### Feature Priorities

The priority values of the features are used to determine the order in which the features are applied to the API Gateway configuration.
Some features are mandatory by others and must be applied first.

> [!NOTE]
> Priorities can also be relative to each other to fine-tune the order of application.

The following table lists a **subset** of features and their priorities.


| Feature          | Priority                                   |
|------------------|--------------------------------------------|
| PassThrough      | 0                                          |
| AccessControl    | 10                                         |
| RateLimit        | 10                                         |
| ExternalIDP      | InstanceCustomScopesFeature - 1 (98)       |
| CustomScopes     | InstanceLastMileSecurityFeature - 1 (99)   |
| LastMileSecurity | 100                                        |

### Feature-Builder

The `FeatureBuilder` is responsible for building the configuration of the API Gateway based on the defined features.
Currently, it is tightly coupled with [our Kong Gateway](https://github.com/telekom/Open-Telekom-Integration-Platform/blob/main/docs/repository_overview.md#gateway) extension.
However, extending it to support other API Gateway technologies is possible.

The `FeatureBuilder` also constructs the required [plugins](pkg/kong/client/plugin) and provides access to them via its interface.
So `Feature` developers do not need to worry about that and can expect that the plugins are available.

For information about the process flow of the `FeatureBuilder`, see the [internal/features/builder.go](internal/features/builder.go).

>[!WARNING]
> If one feature fails to be applied, the builder will stop processing and the `v1.Route` resource will result in an error in the operator.

The `FeatureBuilder` uses `context.Context` to create clients, loggers or access the run-time components such as the environment.
Otherwise, when referring to builder-context, it is the variables of the current `FeatureBuilder` object.

### Api-Client

### Plugins
Kong Gateway can be configured with various plugins in mind. Here is a list of plugins that are currently supported:

* [ACL](https://docs.konghq.com/hub/kong-inc/acl/)
* [JWT](https://docs.konghq.com/hub/kong-inc/jwt/)
* [Rate Limiting](https://docs.konghq.com/hub/kong-inc/rate-limiting/)
* [Request Transformer](https://docs.konghq.com/hub/kong-inc/request-transformer/)
* [Jumper](https://github.com/telekom/gateway-jumper)

Various `Features` require different plugin configurations, which are applied by the `FeatureBuilder`.
Hence, the operator expects, that the plugins are available in the Kong Gateway instance.

The implementation of the plugins is done in the [pkg/kong/client/plugin](pkg/kong/client/plugin) package.
Since the Kong Client works on JSON objects, the plugins should use the `CustomPlugin` [types.go](pkg/kong/client/types.go) interface to define their 
configuration. This make handling plugins much more easier, and developers do not have to consider mutating JSON objects directly.
>[!NOTE]
> If you implement new Plugins, the need to be registered in the FeatureBuilder to be able to work with as well.


## CRDs
All CRDs can be found here: [CRDs](./config/crd/bases/).

<p>The Gateway domain defines the following Custom Resources (CRDs):</p>

<details>
<summary>
<strong>Gateway</strong>
This CRD represents a gateway instance that serves as the entry point for API traffic.
</summary>  

- The Gateway CR defines the connection details to the underlying API Gateway technology (currently Kong).
- The Gateway CR contains Redis configuration for distributed caching and rate limiting.
- The Gateway CR includes admin credentials for gateway management.
- Supports feature flags to enable/disable specific gateway capabilities.

</details>
<br />

<details>
<summary>
<strong>Realm</strong>
This CRD represents a virtual environment within a gateway for multi-tenancy.
</summary>  

- The Realm status tracks URLs for issuer, certs, and discovery endpoints based on the Gateway CR.

</details>
<br />

<details>
<summary>
<strong>Route</strong>
This CRD represents an API route exposed on the gateway.
</summary>  

- The Route CR MUST reference a Realm where it will be exposed.
- The Route CR defines upstream and downstream configurations.
- Routes support failover configuration for high availability.
- Routes can be configured with traffic management.
- Security options include pass-through mode, M2M authentication, and external IDP integration.
- Transformation options allow request/response modifications.

</details>
<br />

<details>
<summary>
<strong>Consumer</strong>
This CRD represents a client that can access routes on the gateway.
</summary>  

- The Consumer CR MUST reference a Realm where it will be used.
- The Consumer CR contains a unique name within the realm.
- Consumers can have IP restrictions for additional security.
- The Consumer status tracks the underlying gateway consumer ID.

</details>
<br />

<details>
<summary>
<strong>ConsumeRoute</strong>
This CRD represents the relationship between a Consumer and a Route.
</summary>  

- The ConsumeRoute CR links a Consumer to a Route, granting access permissions.
- The ConsumeRoute CR can override security settings for a specific consumer-route pair.
- The ConsumeRoute CR can specify consumer-specific rate limits.
- Supports M2M authentication with client credentials or basic auth.

</details>
<br />
