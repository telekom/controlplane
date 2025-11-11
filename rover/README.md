<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

<p align="center">
  <h1 align="center">Rover</h1>
</p>

<p align="center">
  The Rover domain provides the user-facing API for defining and managing API exposures and subscriptions through declarative Rover files.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#features">Features</a> •
</p>

## About

The Rover domain is the primary entry point for users to interact with the Control Plane. Users define their API exposures and subscriptions in a Rover file, which the operator translates into the appropriate resources across other domains (API, Application, Gateway, Identity).

A Rover resource represents a complete application configuration, including:
- API exposures with upstream configurations
- API subscriptions to consume other APIs
- Traffic management (rate limiting, load balancing)
- Approval requirements for subscriptions


## Features

- **Declarative API Management**: Define API exposures and subscriptions in a single Rover file
- **Load Balancing**: Distribute requests across multiple upstream services with configurable weights
- **Rate Limiting**: Configure rate limits at provider and consumer levels with flexible time windows
- **Approval Workflows**: Integrate with the approval domain for subscription requests
- **Trusted Teams**: Automatically approve subscriptions from designated trusted teams
- **Traffic Management**: Configure circuit breakers, timeouts, and retry policies

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

#### Validation Rules

The Rover admission webhook enforces the following validation rules for rate limits:
- At least one time window (second, minute, or hour) must be specified
- When multiple time windows are specified, they must follow the ordering: second < minute < hour
- These rules apply to both provider rate limits and consumer rate limits (default and overrides)

#### Consumer Identification

Consumer names in `ratelimit.consumers.overrides` refer to the client identifier that will be extracted from the request.
The system automatically matches the extracted consumer identifier with the configured overrides to apply the appropriate rate limits.

The consumer ID format follows the pattern `{team}--{applicationName}` where:
- `team` is the team name from the Application's spec
- `applicationName` is the name of the Application resource

When configuring consumer-specific rate limits, you must use this exact format to ensure proper matching.

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
            # Override limits for specific consumers
            overrides:
            - consumer: alpha--premium-client 
              limits:
                second: 50
                minute: 500
                hour: 5000
            - consumer: myGroup--internal-service 
              limits:
                minute: 2000
                hour: 20000
```

