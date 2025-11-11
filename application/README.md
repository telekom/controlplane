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
  <a href="#about">About</a> •
  <a href="#workflow">Workflow</a> 
</p>

## About

The Application operator provides a Kubernetes-native way to manage Tardis applications. A Tardis application is an abstraction representing a users Rover file. Once this Rover file is applied, the created Application logically encapsulates all the exposures and subscriptions. The Application can also contain an Identity client, that can be used to access the subscriptions. The operator extends Kubernetes with custom resources to create and manage Applications in a declarative way.

This operator is part of the Deutsche Telekom ControlPlane (CP) platform.


### Workflow
The Application operator follows a hierarchical reconciliation pattern for Application resource management:

1. **Application Reconciliation**: The operator watches the Application resource and periodically adjust the cluster's configuration. This includes actions like managing the Identity client and Gateway consumer associated with the Application and rotating the secret. 

The controller implements a declarative approach, continuously reconciling the desired state (defined in the CRs) with the actual state in the cluster. It handles retries and error conditions to ensure eventual consistency.

## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

By participating in this project, you agree to abide by its [Code of Conduct](./CODE_OF_CONDUCT.md) at all times.

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/). You can find a guide for developers at https://telekom.github.io/reuse-template/.   
Each file contains copyright and license information, and license texts can be found in the [./LICENSES](./LICENSES) folder. For more information visit https://reuse.software/.