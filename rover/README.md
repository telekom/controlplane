<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->


# rover
// TODO(user): Add simple overview of use/purpose

## Description
// TODO(user): An in-depth paragraph about your project and overview of use


## Features

* **Load Balancing**: As an API provider you can use the load balancing feature to have requests distributed between a list of upstreams.
* **Rate Limiting**: Configure rate limits for API traffic at the provider level and for consumers with customizable time windows.

### Load Balancing

As an API provider you can use the load balancing feature to have requests distributed between a list of upstreams. 
This list is limited to a maximum of 12 upstreams. You can also define weights to configure the request distribution 
ratio. This is optional and as a default, requests will be balanced equally between all upstreams.

> An important thing to notice is that when you define load balancing, either all upstreams must contain a weight or 
> no upstream must contain a weight. Mixing weights is not allowed and will result in an error when creating or 
> updating the Rover resource.

Example resource of kind Rover with load balancing:

```yaml
apiVersion: rover.cp.ei.telekom.de/v1
kind: Rover
metadata:
  name: provider-with-load-balancing
spec:
  zone: "aws"
  exposures:
    - api:
        basePath: "/some/basepath/v1"
        upstreams:
          - url: "https://my-first-upstream.telekom.de"
            weight: 1  # optional
          - url: "https://my-second-upstream.telekom.de"
            weight: 2 # optional
```

### Rate Limiting

Rate limiting allows you to control the number of requests that can be made to your API within specific time windows. This helps protect your API from abuse, ensures fair usage, and maintains service quality during high traffic periods.

#### Rate Limit Configuration

You can configure rate limits at two levels:
1. **Provider Rate Limits**: Applied to all traffic to your API
2. **Consumer Rate Limits**: Set default limits for all consumers and override limits for specific consumers

Rate limits can be defined for three time windows:
- **Second**: Maximum requests per second
- **Minute**: Maximum requests per minute
- **Hour**: Maximum requests per hour

Additionally, you can configure the following options for rate limits:
- **HideClientHeaders**: When set to `true`, rate limit headers will not be sent to clients
- **FaultTolerant**: When set to `true`, the system will not fail requests if the rate limiting service is unavailable

#### Consumer Identification

Consumer names in rate limit overrides refer to the client identifier that will be extracted from the request. This is typically:
- The API key identifier for API key-based authentication
- The client ID for OAuth2/OIDC-based authentication
- The consumer name configured in the subscription for the API

The system automatically matches the extracted consumer identifier with the configured overrides to apply the appropriate rate limits.

#### Validation Rules

The Rover admission webhook enforces the following validation rules for rate limits:
- At least one time window (second, minute, or hour) must be specified
- When multiple time windows are specified, they must follow the ordering: second < minute < hour
- These rules apply to both provider rate limits and consumer rate limits (default and overrides)

#### Example Configuration

For the definition and default values see [./api/v1/traffic_types.go](./api/v1/traffic_types.go)

```yaml
apiVersion: rover.cp.ei.telekom.de/v1
kind: Rover
metadata:
  name: example-api
spec:
  exposures:
  - api:
      basePath: "/rateLimiting/v1"
      traffic:
        rateLimit:
          # Provider rate limits (applied to all traffic)
          provider:
            limits:
              second: 100
              minute: 1000
              hour: 10000
            options:
              hideClientHeaders: false
              faultTolerant: true
          consumers:
            # Default limits for all consumers
            default:
              limits:
                second: 10
                minute: 100
                hour: 1000
              options: # values are optional. If not provided, default values will be applied
                hideClientHeaders: true
                faultTolerant: false
            # Override limits for specific consumers
            overrides:
            - consumer: alpha--premium-client  # Matches client ID <group--team>
              config:
                limits:
                  second: 50
                  minute: 500
                  hour: 5000
                options:
                  hideClientHeaders: false 
                  faultTolerant: true
            - consumer: myGroup--internal-service  # Matches client ID or API key identifier
              config:
                limits:
                  minute: 2000
                  hour: 20000
```

## Getting Started

### Prerequisites
- go version v1.21.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/rover:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/rover:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/rover:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/rover/<tag or branch>/dist/install.yaml
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
