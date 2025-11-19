<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

<p align="center">
  <h1 align="center">Identity</h1>
</p>

<p align="center">
  The Identity domain is responsible for configuring the Identity Providers on the data planes. It provides an API to manage IdentyProviders, Realms and Clients.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#features">Features</a> •
  <a href="#keycloak-integration">Keycloak Integration</a> •
  <a href="#crds">CRDs</a>
</p>

## About

The Identity domain provides a Kubernetes-native way to manage identity and access management resources through Keycloak integration. It extends Kubernetes with custom resources to create and manage identity providers, realms, and clients in a declarative way.


The API is designed to be independent of the underlying IDP technology, allowing for flexibility in choosing the best solution for your needs.
However, at the moment, the implementation is tightly coupled with [Keycloak](https://www.keycloak.org/documentation).

The following diagram illustrates the architecture of the Identity domain:
<div align="center">
    <img width="800" height="700" src="./docs/identity_overview.drawio.svg" />
</div>

## Features

- **Client Management**: Manage clients and their configurations.
- **IDP Management**: Manage configured IDPs
- **Realm Management**: Manage separated IDP realms.
- **Secret Resolving**: Resolving Secret Manager references before creating clients in Keycloak

> ![IMPORTANT]
> Clients **block** if their referenced Realm is not ready:

## Keycloak Integration

### Client Type
All created clients are **service accounts** with these fixed settings:
- `serviceAccountsEnabled: true`
- `standardFlowEnabled: false` (no browser login)
- `fullScopeAllowed: false` (restricted scopes)

### Protocol Mappers
Automatically adds "Client ID" mapper to include client ID in tokens.

See [pkg/keycloak/mapper/mapper_client.go](pkg/keycloak/mapper/mapper_client.go)for implementation.

## CRDs
All CRDs can be found here: [CRDs](./config/crd/bases/).

<p>The Identity domain defines the following Custom Resources (CRDs):</p>

<details>
<summary>
<strong>IdentityProvider</strong>
This CRD represents a connection to an identity provider instance (e.g., Keycloak).
</summary>  

- The IdentityProvider CR defines connection details to the underlying identity provider.
- The IdentityProvider CR includes admin credentials for management operations.
- The IdentityProvider status tracks URLs for admin access, token endpoints, and console.

</details>
<br />

<details>
<summary>
<strong>Realm</strong>
This CRD represents a security realm within an identity provider.
</summary>  

- The Realm CR MUST reference an IdentityProvider where it will be created.
- The Realm status tracks URLs and admin credentials for the realm.
- Realms provide logical separation of identity resources.
- Other resources like Clients depend on Realms being ready.

</details>
<br />

<details>
<summary>
<strong>Client</strong>
This CRD represents a client application registered with an identity provider.
</summary>  

- The Client CR MUST reference a Realm where it will be created.
- The Client CR contains client ID and secret for authentication.
- All clients are created as service accounts (non-interactive).
- The Client status tracks the issuer URL for token validation.

</details>
<br />
