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
