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
   <a href="#getting-started">Getting Started</a>
</p>


## About

This repository contains the implementation of the Gateway domain, which is responsible for configuring the API Gateway at runtime.
The API is designed to be independent of the underlying API Gateway technology, allowing for flexibility in choosing the best solution for your needs.
However, at the moment, the implementation is tightly coupled with the [Kong Gateway](https://docs.konghq.com/gateway/latest/).

The following diagram illustrates the architecture of the Gateway domain:

<div align="center">
    <img width="800" height="700" src="docs/overview.drawio.svg" />
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

## Getting Started


... tbd