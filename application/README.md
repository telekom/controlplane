<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

<p align="center">
  <h1 align="center">Application</h1>
</p>

<p align="center">
  Kubernetes operator for managing Tardis applications.
</p>

<p align="center">
  <a href="#reconciliation-Flow"> Reconciliation Flow</a> •
  <a href="#dependencies">Dependencies</a> •
  <a href="#model">Model</a> •
  <a href="#code-of-conduct">Code of conduct</a> • 
  <a href="#licensing">Licensing</a> 
</p>

## About

The Application operator provides a Kubernetes-native way to manage Tardis applications. A Tardis application is an abstraction representing a users Rover file. Once this Rover file is applied, the created Application logically encapsulates all the exposures and subscriptions. The Application can also contain an Identity client, that can be used to access the subscriptions. The operator extends Kubernetes with custom resources to create and manage Applications in a declarative way.

This operator is part of the Deutsche Telekom Control Plane (CP) platform.


## Reconciliation Flow
The diagram below shows the general Reconciliation flow.
# ![Flow](./docs/identity_overview.drawio.svg)


### Workflow
The Application operator follows a hierarchical reconciliation pattern for Application resource management:

1. **Application Reconciliation**: The operator watches the Application resource and periodically adjust the cluster's configuration. This includes actions like managing the Identity client and Gateway consumer associated with the Application and rotating the secret. 

The controller implements a declarative approach, continuously reconciling the desired state (defined in the CRs) with the actual state in the cluster. It handles retries and error conditions to ensure eventual consistency.

## Dependencies
- [Controller-Runtime](https://github.com/kubernetes-sigs/controller-runtime) - Library for building Kubernetes operators
- [Common](../common/) - Deutsche Telekom Control Plane common library

## Model
The Application operator provides a single Custom Resource Definition (CRD) that represent the Tardis Application. 

Structure: 
- **needsClient**: boolean value that tells Tardis if an Identity client is required for this Application; true if a subscription is present, otherwise false
- **needsConsumer**: boolean value that tells Tardis if a Gateway consumer is required for this Application; true if a subscription is present, otherwise false
- **secret**: holds the secret that is used to get an access token to access the subscriptions; can be a direct value (not recommended) or a secret manager reference
- **team**: identifies the team that this Application belongs to
- **teamEmail**: contact information of the team that this Application belongs to
- **zone**: a reference to the Zone where this Application resides; it is the same Zone that the Rover file specified

Each resource includes status conditions that reflect the state of reconciliation.

You can find the custom resource definitions in the [config/crd directory](./config/crd/).

A simple example Application would look like this:

<details>
  <summary>Example Application</summary>

  ```yaml
    apiVersion: application.cp.ei.telekom.de/v1
    kind: Application
    metadata:
      labels:
        cp.ei.telekom.de/application: sample-application
        cp.ei.telekom.de/environment: sample-env
        cp.ei.telekom.de/zone: sample-zone
      name: sample-application
      namespace: sample-env--sample-team--sample-application
    spec:
      needsClient: true
      needsConsumer: true
      secret: sample-secret
      team: sample-group--sample-team
      teamEmail: sample-team@example.com
      zone:
        name: sample-zone
        namespace: sample-env
  ```
</details><br />

## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

By participating in this project, you agree to abide by its [Code of Conduct](./CODE_OF_CONDUCT.md) at all times.

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/). You can find a guide for developers at https://telekom.github.io/reuse-template/.   
Each file contains copyright and license information, and license texts can be found in the [./LICENSES](./LICENSES) folder. For more information visit https://reuse.software/.