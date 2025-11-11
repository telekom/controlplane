<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0    
-->

<p align="center">
  <h1 align="center">API</h1>
</p>

<p align="center">
  The Api domain is responsible for all Api-related resources: the Api itself as well as its Exposure and Subscription.
</p>

<p align="center">
  <a href="#about"> About</a> •
  <a href="#features"> Features</a>
</p>

## About
This repository contains the implementation of the Api domain, which is responsible for managing the whole lifecycle of an API. 

The following diagram illustrates the architecture of the Api domain:
<div align="center">
    <img src="docs/img/api_overview.drawio.svg" />
</div>

## Features

- **Api Management**: Manage the whole API lifecycle, including registering, exposing and subscribing.
- **Approval Handling**: Require approval when subscribing to APIs using the integration with the [Approval Domain](../approval).
- **Api Categories**: Classify APIs into categories and customize their behavior based on these categories.

### Api Categories

You may create categories to classify your APIs. Each category can have specific properties that define its behavior.
The check for allowed categories is done at the earliest point in the [Rover Domain](../rover).

## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/).    
Each file contains copyright and license information, and license texts can be found in the [./LICENSES](./LICENSES) folder. For more information visit https://reuse.software/.    
You can find a guide for developers at https://telekom.github.io/reuse-template/.