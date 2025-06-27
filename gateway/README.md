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
  <a href="#crds">CRDs</a> •
   <a href="#getting-started">Getting Started</a>
</p>


## About

This repository contains the implementation of the Gateway domain, which is responsible for configuring the API Gateway at runtime.
The API is designed to be independent of the underlying API Gateway technology, allowing for flexibility in choosing the best solution for your needs.
However, at the moment, the implementation is tightly coupled with the [Kong Gateway](https://docs.konghq.com/gateway/latest/).

The following diagram illustrates the architecture of the Gateway domain:

<div align="center">
    <img src="docs/overview.drawio.svg" />
</div>

## Features

- **Route Management**: Manage routes and their configurations.
- **Consumer Management**: Manage consumers and their access to routes.
- **Realm Management**: Support for virtual environments to allow for virtualization of the API Gateway deployments.

Other - more advanced - features are planned for the future, such as:

- **Rate Limiting**: Control the rate of requests to your APIs.
- **Load Balancing**: Distribute incoming requests across multiple instances of your API.
- **External IDP Integration**: Integrate with external Identity Providers for authentication and authorization.
- **Scopes**: Define scopes for consumers to control access to specific resources.
- **Basic Authentication**: Support for basic authentication for consumers.
- ... and many more features to come!

## CRDs

The Gateway domain defines the following Custom Resource Definitions (CRDs) as an API:

<details>
<summary>
<strong>Gateway</strong>
This CRD represents a phyical API Gateway instance, which can be a Kong Gateway or any other API Gateway technology.
It acts as a container for the credentials and global configuration of the API Gateway.
</summary>  

```yaml
apiVersion: gateway.cp.ei.telekom.de/v1
kind: Gateway
metadata:
  labels:
    cp.ei.telekom.de/environment: default
  name: example-gateway
  namespace: zone-namespace
spec:
  admin:
    clientId: example-client-id
    clientSecret: example-client-secret
    issuerUrl: https://issuser.example.com # this is the issuer configured in Kong
    url: https://api.kong.example.com/admin # this is the admin-api of Kong
```

</details>
<br />

<details>
<summary>
<strong>Realm</strong>
This CRD represents a virtual instance of an API Gateway, allowing for the virtualization of the API Gateway deployments.
Each API Gateway must atleast have one Realm, which <strong>must match the virtual environment</strong>. Each `Route`, `Consumer` and `ConsumeRoute` must be assigned to a Realm.
</summary>  

```yaml
apiVersion: gateway.cp.ei.telekom.de/v1
kind: Realm
metadata:
  labels:
    cp.ei.telekom.de/environment: default
  name: default
  namespace: zone-namespace
spec:
  gateway:
    name: example-gateway
    namespace: zone-namespace
  url: https://default.api.example.com
  issuerUrl: https://issuer.example.com/auth/realms/default
```

</details>
<br />


<details>
<summary>
<strong>Route</strong>
This CRD is the primary resource of the Gateway domain, representing an API route that can be accessed by consumers.
The access is configured by the `Consumer` and `ConsumeRoute` CRDs.

</summary>  

```yaml
# Expose a Route on the defined Realm that points to the upstream API provider.
apiVersion: gateway.cp.ei.telekom.de/v1
kind: Route
metadata:
  labels:
    cp.ei.telekom.de/environment: default
  name: default--api
  namespace: zone-namespace
spec:
  realm:
    name: default
    namespace: zone-namespace
  upstreams: 
  - scheme: https
    host: provider.example.com
    port: 443
    path: /api/v1
  downstreams:
  - host: default.api.example.com
    port: 443
    path: /api
    issuerUrl: https://issuer.example.com/auth/realms/default
```

</details>
<br />


<details>
<summary>
<strong>Consumer</strong>
This CRD represents a `Consumer` of the API, which is in most cases an application.
By default a `Consumer` has no access to any route. The `Consumer` is granted access to routes by configuring the `ConsumeRoute` CRD for each `Route´.

</summary>  

```yaml
apiVersion: gateway.cp.ei.telekom.de/v1
kind: Consumer
metadata:
  labels:
    cp.ei.telekom.de/environment: default
  name: default
  namespace: zone-namespace
spec:
  realm:
    name: default
    namespace: zone-namespace
  name: example-consumer

```

</details>
<br />


<details>
<summary>
<strong>ConsumeRoute</strong>
This CRD represents the access of a `Consumer` to a `Route`.
It can be used to configure consumer specific settings for a route, such as rate limiting, authentication, and other features.
</summary>  

```yaml
# Grant access to defined Route for the Consumer
apiVersion: gateway.cp.ei.telekom.de/v1
kind: ConsumeRoute
metadata:
  labels:
    cp.ei.telekom.de/environment: default
  name: default
  namespace: zone-namespace
spec:
  route:
    name: default--api
    namespace: zone-namespace
  consumerName: example-consumer

```

</details>
<br />

## Internal Architecture

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


## Getting Started


... tbd