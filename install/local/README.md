<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Local Installation

This directory contains the Kubernetes manifests for installing the controlplane locally.

## Documentation

For complete installation instructions, please refer to:
- [Quickstart Guide](https://telekom.github.io/controlplane/docs/Installation/quickstart)
- [Detailed Installation Guide](https://telekom.github.io/controlplane/docs/Installation/installation)

## Directory Structure

- `kustomization.yaml`: Main entry point for applying all resources
- `resources/`: Sample resources to create after installation
  - `admin/`: Admin resources (zones, environments)
  - `org/`: Organization resources (teams, groups)
  - `rover/`: Rover resources (workloads)

## Important Configuration Notes

Before installing, you may need to update the zone configuration files in `resources/admin/zones` with your identity provider and gateway configuration:

**Identity Provider Configuration (in dataplane1.yaml and dataplane2.yaml)**
```yaml
identityProvider:
  admin:
    clientId: admin-cli
    userName: admin
    password: somePassword
  url: https://my-idp.example.com/
```

**Gateway Configuration (in dataplane1.yaml and dataplane2.yaml)**
```yaml
gateway:
  admin:
    clientSecret: someSecret
    url: https://my-gateway-admin.example.com/admin-api
  url: https://my-gateway.example.com/
```

## Basic Installation Commands

For quick reference, the basic installation sequence is:

```bash
# Install controlplane components
kubectl apply -k .

# Install sample resources
kubectl apply -k resources/admin
kubectl apply -k resources/org
kubectl apply -k resources/rover
```

For detailed explanations, troubleshooting, and verification steps, please refer to the documentation site linked above.