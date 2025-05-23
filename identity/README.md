<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

<p align="center">
  <h1 align="center">Identity</h1>
</p>

<p align="center">
  Kubernetes operator for managing identity and access management resources with Keycloak
</p>

<p align="center">
  <a href="#reconciliation"> Reconciliation Flow</a> •
  <a href="#dependencies">Dependencies</a> •
  <a href="#model">Model</a> •
  <a href="#crds">CRDs</a>
</p>

## About

The Identity operator provides a Kubernetes-native way to manage identity and access management resources through Keycloak integration. It extends Kubernetes with custom resources to create and manage identity providers, realms, and clients in a declarative way.

This operator is part of the Deutsche Telekom Control Plane (CP) platform, enabling seamless IAM integration for cloud-native applications. It abstracts the complexity of Keycloak administration, allowing platform administrators and application teams to define and manage identity resources using Kubernetes manifests.


## Reconciliation Flow
The diagram below shows the general Reconciliation flow.
# ![Flow](./docs/identity_overview.drawio.svg)


### Workflow
The Identity operator follows a hierarchical reconciliation pattern for IAM resource management:

1. **IdentityProvider Reconciliation**: The operator connects to the configured Keycloak instance using the admin credentials provided in the IdentityProvider CR. It validates the connection and updates the status with configuration details.

2. **Realm Reconciliation**: When a Realm CR is created that references an IdentityProvider, the operator creates or updates the corresponding realm in Keycloak. It obtains the admin credentials from the referenced IdentityProvider resource.

3. **Client Reconciliation**: Client resources reference a Realm and define OAuth/OIDC client configurations. The operator manages the corresponding Keycloak client, creating or updating it based on the provided specifications.

The controller implements a declarative approach, continuously reconciling the desired state (defined in the CRs) with the actual state in Keycloak. It handles retries and error conditions to ensure eventual consistency.

## Dependencies
- [Keycloak](https://www.keycloak.org/) - Open source identity and access management server
- [Controller-Runtime](https://github.com/kubernetes-sigs/controller-runtime) - Library for building Kubernetes operators
- [Common](../common/) - Deutsche Telekom Control Plane common library
## Model
The Identity operator provides a set of Custom Resource Definitions (CRDs) that represent identity and access management resources. These resources form a hierarchical relationship, with each tier referencing the resource above it.

Core models in this project are:
- **IdentityProvider**: Represents a Keycloak instance with administrative credentials for managing realms and clients.
- **Realm**: Represents a security domain within Keycloak that contains a set of users, clients, and configurations.
- **Client**: Represents an OIDC/OAuth client application that can authenticate users and obtain token-based authentication.

Each resource includes status conditions that reflect the state of reconciliation with the actual Keycloak instance.

## CRDs
The Identity operator defines three main Custom Resource Definitions (CRDs) that represent the identity and access management resources in Kubernetes:

1. **IdentityProvider** - A connection to a Keycloak instance with admin credentials
2. **Realm** - A security domain within Keycloak
3. **Client** - An OAuth/OIDC client application

You can find the custom resource definitions in the [config/crd directory](./config/crd/).

### IdentityProvider
The IdentityProvider CRD represents a connection to a Keycloak instance. It contains the necessary information to authenticate with Keycloak as an administrator, allowing the operator to manage realms and clients.

A simple example IdentityProvider would look like this:

<details>
  <summary>Example IdentityProvider</summary>

  ```yaml
  apiVersion: identity.cp.ei.telekom.de/v1
  kind: IdentityProvider
  metadata:
    name: idp-germany
    namespace: default
    labels:
      app.kubernetes.io/name: idp-germany
      cp.ei.telekom.de/zone: dataplane1
      cp.ei.telekom.de/environment: poc
  spec:
    adminUrl: "https://keycloak.example.com/auth/admin/realms/"
    adminClientId: "admin-cli"
    adminUserName: "admin"
    adminPassword: "password"
  ```
</details><br />

### Realm
The Realm CRD represents a security domain within Keycloak. It references an IdentityProvider resource and is used to create and manage a realm in the referenced Keycloak instance.

A simple example Realm would look like this:

<details>
  <summary>Example Realm</summary>

  ```yaml
  apiVersion: identity.cp.ei.telekom.de/v1
  kind: Realm
  metadata:
    name: realm-germany
    namespace: default
    labels:
      app.kubernetes.io/name: realm-germany
      cp.ei.telekom.de/zone: dataplane1
      cp.ei.telekom.de/environment: poc
  spec:
    identityProvider:
      name: idp-germany
      namespace: default
  ```
</details><br />

### Client
The Client CRD represents an OAuth/OIDC client application within a realm. It references a Realm resource and is used to create and manage a client in the referenced realm.

A simple example Client would look like this:

<details>
  <summary>Example Client</summary>

  ```yaml
  apiVersion: identity.cp.ei.telekom.de/v1
  kind: Client
  metadata:
    name: client-germany
    namespace: default
    labels:
      app.kubernetes.io/name: client-germany
      cp.ei.telekom.de/zone: dataplane1
      cp.ei.telekom.de/environment: poc
  spec:
    realm:
      name: realm-germany
      namespace: default
    clientId: "client-germany"
    clientSecret: "topsecret"
  ```
</details><br />

## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

By participating in this project, you agree to abide by its [Code of Conduct](./CODE_OF_CONDUCT.md) at all times.

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/). You can find a guide for developers at https://telekom.github.io/reuse-template/.   
Each file contains copyright and license information, and license texts can be found in the [./LICENSES](./LICENSES) folder. For more information visit https://reuse.software/.