<!--
SPDX-FileCopyrightText: 2024 Deutsche Telekom AG

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
  <a href="#features"> Features</a> •
  <a href="#dependencies">Dependencies</a> •
  <a href="#crds">CRDs</a> •
  <a href="#getting-started"> Getting started</a> •
</p>

## About
This repository contains the implementation of the Api domain, which is responsible for managing the whole ifecycle of an API. 

The following diagram illustrates the architecture of the Gateway domain:
<div align="center">
    <img src="docs/img/api_overview.drawio.svg.drawio" />
</div>

## Features

- **Api Management**: Manage the whole API lifecycle, including registering, exposing and subscribing.

## Dependencies
- [Common](../common)
- [Admin](../admin)
- [Application](../application)
- [Approval](../approval)
- [Gateway](../gateway)
- [Identity](../identity)
- [ControlplaneApi](../cpapi)

## CRDs
All CRDs can be found here: [CRDs](./config/crd/bases/)
The Api domain defines the following Custom Resources (CRDs) APIs:

<details>
<summary>
<strong>Api</strong>
This CRD represents a registered API, uniquely identified by its basePath.
Example Api resource:
</summary>  

```yaml
apiVersion: api.cp.ei.telekom.de/v1
kind: Api
metadata:
  labels:
    cp.ei.telekom.de/environment: default
  name: group-team-api-v1
  namespace: zone-namespace
spec:
  basePath: /group/team/api/v1
  category: other
  name: group-team-api-v1
  version: 1.0.0
  xVendor: false
```
</details>
<br />

<details>
<summary>
<strong>ApiExposure</strong>
This CRD represents an API exposed on the Gateway.
Example ApiExposure resource:
</summary>  

```yaml
apiVersion: stargate.cp.ei.telekom.de/v1
kind: ApiExposure
metadata:
  labels:
    cp.ei.telekom.de/application: applicationName
    cp.ei.telekom.de/basepath: group-team-api-v1
    cp.ei.telekom.de/environment: env
    cp.ei.telekom.de/zone: zoneName
  name: applicationName--group-team-api-v1
  namespace: env--group--team
spec:
  apiBasePath: /group/team/api/v1
  approval: Simple
  upstreams:
    - url: https://my-upstream-url
      weight: 100
  visibility: World
  zone:
    name: zoneName
    namespace: env
```
</details>
<br />

<details>
<summary>
<strong>ApiSubscription</strong>
This CRD represents a subscription to an exposed API.
Example ApiSubscription resource:
</summary>  

```yaml
apiVersion: stargate.cp.ei.telekom.de/v1
kind: ApiSubscription
metadata:
  labels:
    cp.ei.telekom.de/application: subscribing-application
    cp.ei.telekom.de/basepath: group-team-api-v1
    cp.ei.telekom.de/environment: env
    cp.ei.telekom.de/zone: zoneName
  name: subscribing-application--group-team-api-v1
  namespace: env--group--team
spec:
  apiBasePath: /group/team/api/v1
  requestor:
    application:
      name: subscribing-application
      namespace: env--group--team
  security: {}
  zone:
    name: zoneName
    namespace: env
```
</details>
<br />

## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

By participating in this project, you agree to abide by its [Code of Conduct](./CODE_OF_CONDUCT.md) at all times.

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/). You can find a guide for developers at https://telekom.github.io/reuse-template/.   
Each file contains copyright and license information, and license texts can be found in the [./LICENSES](./LICENSES) folder. For more information visit https://reuse.software/.