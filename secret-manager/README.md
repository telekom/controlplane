<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

<p align="center">
  <h1 align="center">Secret Manager</h1>
</p>

<p align="center">
  Secret Manager (SM) is a REST-ful API for managing secrets. It allows to store, retrieve, and delete secrets securely.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#backends">Backends</a> •
  <a href="#security">Security</a> •
  <a href="#getting-started">Getting Started</a>
</p>

## About

Secret Manager (SM) is a REST-ful API for managing secrets. It allows you to store, retrieve, and delete secrets securely. 

> **Note**: The main goal of this is to obfuscate the secrets in Custom-Resources (CR) with placeholders "secret references".

The following problems are solved:

1. **Secret obfuscation:** The SM is used to replace secrets in CRs with a placeholder and if they secret is needed, retrieve it (jit).
2. **Secret onboarding:** The SM provides a unified API for onboarding new entities and secrets.
3. **Secret storage:** The SM provides a unified API for storing secrets in different backends.
4. **Secret retrieval:** The SM provides a unified API for retrieving secrets using references (IDs) to secrets.
5. **Auditing:** As the SM is a single point of access for secrets, it can provide auditing capabilities for secret access.

The following diagram provides a high-level overview of how the SM is integrated into the Controlplane.

![Architecture Diagram](docs/overview.drawio.svg)


## Backends

The SM itself does not store anything. This is done using backend implementations.
Currently, the SM supports the following backends.

### Kubernetes Secrets

This backend uses [Kubernetes Secrets](https://kubernetes.io/docs/concepts/configuration/secret/) to store secrets. 
It will therefore work with any Kubernetes cluster. 

The **upside** of this backend is that it is easy to set up and does not require any additional components. It is also enables fast development and testing cycles.
The **downside** of this backend is that the secrets are stored in Kubernetes and therefore visible for anyone with access to the Kubernetes cluster.

For more information about the Kubernetes implementation, see the [Kubernetes Backend](./pkg/backend/kubernetes/README.md) documentation.

### Conjur

This backend uses [Conjur](https://www.conjur.org/) to store secrets. 
It is a more secure option than the Kubernetes backend, as it provides a more fine-grained access control and auditing capabilities.

For more information about the Conjur implementation, see the [Conjur Backend](./pkg/backend/conjur/README.md) documentation.

## Security

### Access Rights

We have implemented a simple access control mechanism that allows you to define which services are allowed to access the SM at different levels.

* `secrets_read`: Allows GET requests to the SM.
* `secrets_write`: Allows PUT/DELETE requests to the SM.
* `onboarding_write`: Allows access to `v1/onboarding` endpoints. This is used to onboard new teams, groups and environments.
  * For more information about team/groups and environments, see [Organization](../organization/README.md) and [Admin](../admin/README.md) domain respectively.

For more details on the configuration, see the [Server Configuration](#server-configuration) section.

## Getting Started

### Server Configuration
An example configuration can be found [./config/default/config.yaml](./config/default/config.yaml).

```yaml
backend:
  type: conjur

security:
  enabled: true  # 
  access_config:  # defines a list of services that are allowed to access the SM
  - service_account_name: default
    deployment_name: secret-client-shell
    namespace: secret-manager-client
    allowed_access: 
    - secrets_read
    - secrets_write
    - onboarding_write
```

#### Starting the Server
To start the server, you need to provide the configuration file as a command line argument.

Example for Kubernetes:

```bash
go run ./cmd/server/server.go -backend kubernetes -configfile ./config/default/config.yaml
```

Example for Conjur:

```bash
go run ./cmd/server/server.go -backend conjur -configfile ./config/default/config.yaml
```

For loading the Conjur configuration [github.com/cyberark/conjur-api-go](https://github.com/cyberark/conjur-api-go) is used. 
Configuration is done using environment variables. Please refer to their documentation for more information on how to set up the Conjur backend.


### Client Configuration

For developing and integration purposes, we have included a client that can be used to test the SM. 


### Code Integration
We've included an [OpenAPI spec](./api/openapi.yaml) that can be used to generate client code for the SM.

However, we also provide a basic go implementation that can be used to integrate the SM into your code.


