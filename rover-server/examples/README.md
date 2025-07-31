<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# Rover Examples

This directory contains example Rover definitions in JSON format that demonstrate various configurations and capabilities of the Rover API.

## Authentication Examples

### Basic Auth

- **File**: [basic-auth.yaml](basic-auth.yaml)
- **Description**: Demonstrates how to configure basic authentication for both exposures and subscriptions.

### OAuth2 Examples

- **File**: [oauth2-with-scopes.yaml](oauth2-with-scopes.yaml)
- **Description**: Shows how to configure OAuth2 with scopes for both exposures and subscriptions.

- **File**: [oauth2-external-idp.yaml](oauth2-external-idp.yaml)
- **Description**: Demonstrates how to configure OAuth2 with an external identity provider.

- **File**: [oauth2-client-key.yaml](oauth2-client-key.yaml)
- **Description**: Shows how to configure OAuth2 with a client key for JWT authentication.

- **File**: [oauth2-client-creds.yaml](oauth2-client-creds.yaml)
- **Description**: Demonstrates how to configure OAuth2 with client credentials flow.

- **File**: [oauth2-password.yaml](oauth2-password.yaml)
- **Description**: Shows how to configure OAuth2 with password grant type.

## Traffic Management Examples

- **File**: [load-balancing.yaml](load-balancing.yaml)
- **Description**: Demonstrates how to configure load balancing between multiple backend servers with different weights.

- **File**: [failover.yaml](failover.yaml)
- **Description**: Shows how to configure failover to different zones for both exposures and subscriptions.

- **File**: [rate-limit.yaml](rate-limit.yaml)
- **Description**: Demonstrates how to configure rate limiting at the provider level and for specific consumers.

## Request Modification Examples

- **File**: [remove-headers.yaml](remove-headers.yaml)
- **Description**: Shows how to configure header removal for API exposures.

## Event Handling Examples

- **File**: [event-exposure.yaml](event-exposure.yaml)
- **Description**: Demonstrates how to configure an event exposure with custom scopes and triggers.

- **File**: [event-subscription.yaml](event-subscription.yaml)
- **Description**: Shows how to configure an event subscription with various options including callback URL, filters, and delivery settings.

## Usage

These examples are provided in JSON format for easy consumption by the Rover API. You can use these as starting points for your own Rover configurations.

To use these examples:

1. Copy the desired example
2. Modify the values to match your requirements
3. Submit the JSON to the Rover API endpoint

Note: The examples use placeholder values that should be replaced with actual values in a real deployment.