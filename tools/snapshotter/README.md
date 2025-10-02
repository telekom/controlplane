<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# Snapshotting Tool

This tool is intended to create snapshots of the current state of the system. Mainly for the gateway-domain.

## Usage

```bash
go build -o bin/snapshotter main.go
install -m 0755 bin/snapshotter /usr/local/bin/snapshotter
# or go run main.go --help
snapshotter --help
```

This tool can either be configured via environment variables or flags.

Using a `.env` file (see [example](.env.example))
```bash
GATEWAY_ENV="poc"
GATEWAY_ZONE="dataplane1"
GATEWAY_ROUTE="poc--my-route-v1"
GATEWAY_ADMIN_URL="https://<host>/admin-api>"
GATEWAY_ADMIN_CLIENT_ID="<client-id>"
GATEWAY_ADMIN_CLIENT_SECRET="<client-secret>"
GATEWAY_ADMIN_ISSUER="<issuer-url>"
```
Then run the tool with:
```bash
go run main.go --from-env
```

Or using the automatic setup via flags:
```bash
# If the secret-manager is used, then you need to configure this port-forwarding:
# kubectl -n controlplane-system port-forward svc/secret-manager 8443:443

go run main.go --env poc --zone dataplane1 --route poc--my-route-v1
```

This will automatically connect to the current active kubernetes context and retrieve all
necessary information from the custom resources.

> ℹ️ **Note:** If the secret-manager is used, this will connect to the secret-manager to retrieve the credentials for the admin API.

## Functionality

1. Per default this tool will output a snapshot in yaml format to `./snapshots/`. You can change the output directory by using the `--output-dir` flag.

2. If this snapshot-file does not exist, it will be created and nothing further will happen.

3. If the snapshot file already exists, it will compare the current state with the snapshot and output the differences to the console.