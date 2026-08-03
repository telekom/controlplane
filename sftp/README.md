<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# SFTP Operator

Kubernetes operator for the **SFTP Service** — manages SSH key-based SFTP access and user provisioning.

## API Group

`sftp.cp.ei.telekom.de/v1`

## Resources

| Kind                | Description                                                                             |
| ------------------- | --------------------------------------------------------------------------------------- |
| `Instance`          | Represents an SFTP service instance backed by an SFTP service configuration.            |
| `User`              | Represents an SFTP user and its SSH public keys. Name pattern: `<hub>--<team>--<name>`. |
| `SFTPServiceConfig` | Namespaced configuration for SFTP Tardis API access in a zone.                          |

## Architecture

The sftp-operator manages three main CRD types:

> [!NOTE]
> For a detailed architecture diagram, see [docs](./docs/sftp-domain-architecture.md).

### User

- Represents an SFTP user in the system
- Scoped to a specific Kubernetes namespace
- References its SFTP instance via `spec.instanceRef`
- Manages SSH public keys under `spec.sshPublicKeys[]`
- Public keys are synchronized by the User controller using a per-User client ID
- Status is set directly by User reconciliation after key synchronization

### Instance

- Represents an SFTP instance used by one or more users
- References its `SFTPServiceConfig` via `spec.sftpServiceConfigRef`
- Carries the service-instance description and readiness conditions

### SFTPServiceConfig

- Namespaced configuration resource
- Provides zone-specific SFTP Tardis API endpoint and OAuth2 client credentials
- Referenced by Instance resources

## Resource Relationships

```plain
SFTPServiceConfig (configuration namespace)
       │
       └── referenced by Instance.spec.sftpServiceConfigRef
                    │
                    └── referenced by User.spec.instanceRef
```

## Build & Test

```bash
# From this directory:
make build      # generate + build
make test       # envtest-based integration tests
make lint       # golangci-lint
make install    # install CRDs into current cluster
make deploy     # deploy controller
```

## CRD Generation

CRDs are generated from Go type annotations via `controller-gen`. Run `make manifests` to regenerate them into `config/crd/bases/`.

## Sample Manifests

See `config/samples/` for example resources:

-   `sftp_v1_instance.yaml` — Example Instance resource
-   `sftp_v1_user.yaml` — Example User resource
-   `sftp_v1_sftpserviceconfig.yaml` — Example SFTPServiceConfig resource
