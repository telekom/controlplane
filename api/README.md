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
  <a href="#features"> Features</a> •
  <a href="#dependencies">Dependencies</a> •
  <a href="#crds">CRDs</a>
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

## Dependencies
- [Common](../common)
- [Admin](../admin)
- [Application](../application)
- [Approval](../approval)
- [Gateway](../gateway)
- [Identity](../identity)
- [ControlplaneApi](../cpapi)

## CRDs
All CRDs can be found here: [CRDs](./config/crd/bases/).
<p>The Api domain defines the following Custom Resources (CRDs) APIs:</p>

<details>
<summary>
<strong>Api</strong>
This CRD represents a registered API, uniquely identified by its basePath.
</summary>  
Example resource of kind Api:

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

- There can be multiple API CRs in different namespaces with the same basePath, but only one of them can be `active` at a time.
- The API is considered a singleton based on its [normalized](../common/pkg/util/labelutil/labelutil.go#34) basePath.
  - This means that there can only be one `active` API CR per basePath in the entire cluster, regardless of the namespace and case sensitivity.
  - E. g. `/group/team/api/v1` and `/Group/Team/API/v1` would be considered the same basePath.
- The name of the API CR SHOULD be constructed by using the basePath, e.g. `group-team-api-v1` for basePath `/group/team/api/v1`.
- The API CR MUST be created in the namespace of the team that registered the API.
- The API CR MUST have a label `cp.ei.telekom.de/basepath` with the value of the [normalized](../common/pkg/util/labelutil/labelutil.go#34) basePath.

</details>
<br />

<details>
<summary>
<strong>ApiExposure</strong>
This CRD represents an API exposed on the Gateway. 
For a full description of allowed properties, see <a href="./api/v1/apicategory_types.go#L16">ApiCategory</a>.
</summary>  
Example resource of kind ApiExposure:

```yaml
apiVersion: api.cp.ei.telekom.de/v1
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

- There can be multiple ApiExposure CRs in different namespaces and applications for the same API basePath, but only one of them can be `active` at a time.
- The ApiExposure CR is considered a singleton based on its [normalized](../common/pkg/util/labelutil/labelutil.go#34) apiBasePath.
  - This means that there can only be one `active` ApiExposure CR per apiBasePath in the entire cluster, regardless of the namespace and case sensitivity.
  - E. g. `/group/team/api/v1` and `/Group/Team/API/v1` would be considered the same apiBasePath.
- The name of the ApiExposure CR SHOULD be constructed by using the application name and the apiBasePath, e.g. `application-name--group-team-api-v1`.
- The ApiExposure CR MUST be created in the namespace of the application that exposes the API.
- The ApiExposure CR MUST have a label `cp.ei.telekom.de/basepath` with the value of the [normalized](../common/pkg/util/labelutil/labelutil.go#34) apiBasePath.

</details>
<br />

<details>
<summary>
<strong>ApiSubscription</strong>
This CRD represents a subscription to an exposed API.
</summary>
Example resource of kind ApiSubscription: 

```yaml
apiVersion: api.cp.ei.telekom.de/v1
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

- There can be multiple ApiSubscription CRs in different namespaces and applications for the same API basePath. All of them can be `active` at the same time.
- The name of the ApiSubscription CR SHOULD be constructed by using the application name and the apiBasePath, e.g. `application-name--group-team-api-v1`.
- The ApiSubscription CR MUST be created in the namespace of the application that subscribes to the API.
- The ApiSubscription CR MUST have a label `cp.ei.telekom.de/basepath` with the value of the [normalized](../common/pkg/util/labelutil/labelutil.go#34) apiBasePath.

</details>
<br />

<details>
<summary>
<strong>ApiCategory</strong>
This CRD represents a category to classify APIs.
</summary>
Example resource of kind ApiCategory:

```yaml
apiVersion: api.cp.ei.telekom.de/v1
kind: ApiCategory
metadata:
  name: internal
  namespace: env
  labels:
    cp.ei.telekom.de/environment: env
spec:
  active: true # Whether this category is active and can be used
  description: APIs intended for internal use only.
  labelValue: Internal # This is the expected value in the info.x-api-category field of the OpenAPI spec
  allowTeams:
    names:
      - '*' # The name of the team allowed to register an API with this category. Use '*' to allow all teams.
    categories:
      - Infrastructure # These categories are defined in the organization domain and are just referenced here
```
</details>
<br />

