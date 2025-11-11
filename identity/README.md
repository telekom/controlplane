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

## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](../CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/). Each file contains copyright and license information, and license texts can be found in the [./LICENSES](../LICENSES) folder. For more information visit https://reuse.software/.