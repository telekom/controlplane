<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Local Installation

This directory contains the Kubernetes manifests for installing the controlplane locally.
It includes a disposable RustFS instance and creates the `controlplane-files`
bucket used by File Manager automatically.

## Documentation

For complete installation instructions, please refer to [Installation Guide](https://telekom.github.io/controlplane/docs/admin-journey/installation).

## Directory Structure

- `kustomization.yaml`: Main entry point for applying all resources (local development overlay)
- `global-config.env`: Global configuration shared by local Control Plane workloads
- `secret-manager-config.yaml`: Local configuration for the secret-manager
- `file-manager-config.yaml`: Local configuration for the file-manager
- `resources/`: Sample resources to create after installation
  - `admin/`: Admin resources (zones, environments)
  - `org/`: Organization resources (teams, groups)
  - `rover/`: Rover resources (workloads)

## Important Configuration Notes

The local overlay includes the Eventing component. This atomically installs the Event and PubSub workloads and enables their required global feature flag; do not add that flag to `global-config.env` manually.

Workloads consume optional `controlplane-env` and their unique `<component>-env` ConfigMap before explicit container environment entries. Application code owns shipped defaults; local component values are created as top-level generators rather than merged into nested component generators.

Local structured configuration files are generated with `behavior: replace`. This works here because the overlay composes the bundle from local paths in the same Kustomize build.

Before installing, create local zone configuration files from the provided examples in `resources/admin/zones` and then replace all placeholders with your real values:

```bash
cp install/overlays/local/resources/admin/zones/dataplane1.example.yaml install/overlays/local/resources/admin/zones/dataplane1.yaml
cp install/overlays/local/resources/admin/zones/dataplane2.example.yaml install/overlays/local/resources/admin/zones/dataplane2.yaml
```

> The `.example.yaml` files are safe templates and are tracked in Git.
> The copied `dataplane1.yaml` and `dataplane2.yaml` files are intentionally ignored via `.gitignore` to reduce the risk of committing secrets.

After copying the files, update them with your identity provider and gateway configuration:

**Identity Provider Configuration (in your local `dataplane1.yaml` and `dataplane2.yaml`)**
```yaml
identityProvider:
  admin:
    clientId: admin-cli
    userName: admin
    password: somePassword
  url: https://my-idp.example.com/
```

**Gateway Configuration (in your local `dataplane1.yaml` and `dataplane2.yaml`)**
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
kubectl apply -k install/overlays/local

# Install sample resources
kubectl apply -k install/overlays/local/resources/admin
kubectl apply -k install/overlays/local/resources/org
kubectl apply -k install/overlays/local/resources/rover
```

Open the RustFS console after installation:

```bash
kubectl port-forward -n controlplane-system svc/rustfs 9001:9001
```

Then browse to [http://localhost:9001](http://localhost:9001). The bundled
login is `controlplane` / `controlplane-local-rustfs` and is for local
development only.

For detailed explanations, troubleshooting, and verification steps, please refer to the documentation site linked above.
