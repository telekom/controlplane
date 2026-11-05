<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# Snapshotter Tool

A powerful tool for creating, comparing, and managing snapshots of system state. The snapshotter is designed to capture the configuration and status of various resources, particularly for API gateway services, enabling state tracking, comparison, and monitoring over time.

## Overview

The snapshotter tool provides the following key capabilities:

- **State Capturing**: Create snapshots of routes, consumers, and other gateway resources
- **Versioned Storage**: Store multiple versions of snapshots for historical comparison
- **Diff Generation**: Compare snapshots to identify changes between states
- **Data Transformations**: Apply decoders and obfuscators to handle encoded content and sensitive data
- **HTTP API Server**: Expose snapshot functionality through a web service interface
- **Multiple Source Support**: Connect to and snapshot from multiple gateway instances

## Installation

### Prerequisites

- Go 1.26.9 or later
- Access credentials for gateway admin API

### Build from Source

Build the binary using:

```bash
make build
```

This will create a binary in the `bin` directory.

### Installing

To build and install the binary to your system:

```bash
make install
```

This will build the binary and install it to `/usr/local/bin/snapshotter`.

For custom installation locations, you can use:

```bash
make DESTDIR=/custom/path install
```

## Configuration

The snapshotter tool is configured through a YAML file that specifies sources, data transformations, and storage options.

### Configuration File Structure

```yaml
# Path where snapshots will be stored
storePath: "/path/to/snapshots"

# Global decoders - applied to all sources
decoders:
  - type: base64                              # Decoder type (base64, json, etc.)
    pattern: pattern_name:([A-Za-z0-9=]+)    # Regex pattern with capture group

# Global obfuscators - applied to all sources
obfuscators:
  - pattern: "[sensitive-data-pattern]"       # Regex pattern to match sensitive data
    replace: "redacted-value"                 # Replacement string

# Source configurations
sources:
  source-name-1:                              # Unique name for the source
    environment: production                   # Environment identifier
    zone: aws                                 # Zone/region identifier
    url: https://gateway.example.com/admin    # Gateway admin API URL
    tokenUrl: https://auth.example.com/auth/realms/rover # Issuer base (see below)
    insecureSkipVerify: false                 # Default: verify token and API TLS
    clientId: client-id                       # OAuth client ID
    clientSecret: client-secret               # OAuth client secret
    scopes: [optional-scope1, optional-scope2]# Optional OAuth scopes
    tags: [tag1, tag2]                        # Optional tags for filtering

  source-name-2:
    # Another source configuration
    # ...
```

### Configuration Options

#### Global Settings

- `storePath`: Directory where snapshots will be stored (can be overridden with `snap --store`)
- `decoders`: Global decoders applied to all sources
- `obfuscators`: Global obfuscators applied to all sources

#### Source Configuration

Each source under `sources` represents a gateway instance to snapshot from:

- `environment`: Environment identifier (e.g., "production", "staging")
- `zone`: Zone or region identifier (e.g., "aws", "azure")
- `url`: Gateway admin API URL
- `tokenUrl`: Issuer base URL for authentication. Despite the field name, the Kong client appends `/protocol/openid-connect/token`; supply the base without that suffix or a trailing slash.
- `clientId`: OAuth client ID
- `clientSecret`: OAuth client secret
- `scopes` (optional): OAuth scopes for authentication
- `tags` (optional): Tags for filtering gateway resources
- `obfuscators` (optional): Source-specific obfuscators
- `decoders` (optional): Source-specific decoders
- `insecureSkipVerify` (optional): Defaults to `false`. Explicitly setting `true` disables TLS certificate-chain and hostname verification for **both the authentication token endpoint and the gateway admin API** for this source.

#### TLS verification and test-only opt-out

Leave `insecureSkipVerify` omitted or `false` for normal use. HTTPS endpoints must present certificates trusted by the snapshotter process's system trust store and valid for their hostnames. For private CAs, configure that trust store rather than disabling verification.

Only for an isolated test environment with disposable credentials, opt out explicitly on the source:

```yaml
sources:
  local-test:
    environment: test
    zone: local
    url: https://gateway.test.example.com/admin
    tokenUrl: https://idp.test.example.com/auth/realms/rover
    clientId: snapshotter-test
    clientSecret: <disposable-test-secret>
    insecureSkipVerify: true # TEST ONLY: affects both token and API requests
```

**Risk:** With verification disabled, an attacker can impersonate either server, steal client credentials or access tokens, and read or alter admin API traffic and snapshots. Encryption alone no longer authenticates the server. Do not use this option in production or with real credentials. Remove it or set it back to `false` after testing. This option is specific to the snapshotter; it is not available on Zone, Gateway, or IdentityProvider resources.

#### Decoders

Decoders transform encoded data in snapshots:

- `type`: Decoder type (e.g., "base64", "json")
- `pattern`: Regex pattern with capture group for the encoded content

#### Obfuscators

Obfuscators hide sensitive data in snapshots:

- `pattern`: Regex pattern matching sensitive data
- `replace`: String to replace sensitive data with

## Snapshot Store

The snapshot store manages versioned snapshots on disk or in memory.

### Store Structure

Snapshots are organized by snapshot ID, which is formed by joining the environment, zone, and resource ID with hyphens:

```
<snapshot-id> = <environment>-<zone>-<resource-id>
```

For example: `default-aws-eni-basicauth-demo-v1`

The snapshots are stored under the store path with each ID having its own directory containing versioned snapshot files:

```
<storePath>/<snapshot-id>/<version>.snap.yaml
```

For example:
```
/path/to/snapshots/default-aws-eni-basicauth-demo-v1/1.snap.yaml
```

### Versioning

- Each new snapshot increments the version number
- Default store keeps only the most recent version
- Maximum versions can be configured

### File Format

Snapshots are stored as YAML files with the `.snap.yaml` extension, containing:

```yaml
environment: production
zone: aws
id: resource-id
state:
  # Resource-specific state data
  # ...
```

## Usage

The snapshotter tool is a unified CLI with multiple subcommands:

```bash
snapshotter [global flags] command [command flags]
```

### Global Flags

- `--config string`: Path to the configuration file
- `--format string`: Output format (text, yaml, json) (default "text")

### Commands

#### snap

Take snapshots of routes or consumers from configured sources.

```bash
snapshotter snap [flags]
```

Flags:
- `--source string`: Source to snapshot from (only required if multiple sources are configured)
- `--route string`: ID of the route to snapshot
- `--consumer string`: ID of the consumer to snapshot
- `--store string`: Directory to store snapshots in (overrides `storePath` from the config file)
- `--limit int`: Maximum number of resources for a global snapshot (default `100`, `0` = no limit). Only valid without `--route`/`--consumer`.
  `--limit` takes the first N resources in the gateway's listing order (stable while the gateway content doesn't change);
  the selected resources are processed in sorted order.
- `--raw`: Store the unmodified gateway state (no obfuscators/decoders); required for `apply`. Only allowed together with `--route`.

> **Warning:** Raw snapshots are not obfuscated and may contain sensitive data (credentials, keys, inline
> configuration). They are written with file mode `0600` (directories `0700`). The file store keeps only
> one version per snapshot ID, so a later `snap` of the same route into the same store overwrites the
> rollback point (with an obfuscated, non-applicable snapshot if `--raw` is omitted). Use a dedicated
> `--store` directory for rollback snapshots.

Examples:
```bash
# Take a snapshot of a route with default text output format
snapshotter --config config.yaml snap --source my-source --route my-route

# Take a snapshot of a consumer
snapshotter --config config.yaml snap --source my-source --consumer my-consumer

# Take a snapshot with JSON output format
snapshotter --config config.yaml --format json snap --source my-source --route my-route

# Take a snapshot with YAML output format
snapshotter --config config.yaml --format yaml snap --source my-source --consumer my-consumer

# Capture routes from a source (default limit 100)
snapshotter --config config.yaml snap --source my-source

# Capture all routes from a source
snapshotter --config config.yaml snap --source my-source --limit 0

# Capture with limit
snapshotter --config config.yaml snap --source my-source --limit 10
```

#### apply

Restore a route 1:1 on the same gateway from a raw snapshot. Restores exactly what a route snapshot
contains: service, route, route plugins, upstream and upstream targets. Values are copied as-is.

```bash
snapshotter --config cfg.yaml apply --route <route-id-or-name> [--source <key>] [--store <dir>] [--apply] [--prune]
snapshotter --config cfg.yaml apply --file <path/to/N.snap.yaml> [--source <key>] [--store <dir>] [--apply] [--prune]
```

Flags:
- `--route string`: Load the latest snapshot `<environment>-<zone>-<route>` from the store (environment/zone of the selected source)
- `--file string`: Apply the given snapshot file (mutually exclusive with `--route`)
- `--source string`: Target gateway (only required if multiple sources are configured)
- `--store string`: Snapshot store directory (overrides `storePath` from the config file)
- `--apply`: Execute the plan. Without it, `apply` is a dry run.
- `--prune`: Delete route plugins, upstream targets and the upstream (named like the route) that exist on the gateway but not in the snapshot. Without it they are kept and listed as "would be kept".

Behaviour:
- The snapshot must be raw (`raw: true`), its environment/zone must match the selected source and it must contain a route and a service with IDs. Otherwise nothing is written.
- The current gateway state is read and compared with the snapshot (ignoring `created_at`/`updated_at`). The diff (respecting `--format`) and the planned operations are printed.
- With `--apply`, a backup of the current gateway state is written to `<storePath>/.backups/<snapshot-id>/<unix-timestamp>.snap.yaml` (raw, mode `0600`) before the first write. A store path is required for `--apply`.
- Operations run in the order upstream, targets, service, route, plugins, then prune deletes. Entities are upserted (`PUT`) by the IDs in the snapshot; entities that are already equal are skipped.
- The first failing operation stops the run; the error lists the operations that already succeeded. There is no automatic rollback — re-apply the backup file instead.
- After applying, the gateway state is read again and verified against the snapshot.
- A backup taken while the route did not exist contains no route and cannot be re-applied; it only documents the previous state.

#### cmp

Compare two snapshots from the snapshot store.

```bash
snapshotter cmp [flags]
```

Flags:
- `--a string`: ID of the first snapshot to compare (required)
- `--b string`: ID of the second snapshot to compare (required)
- `--must`: If set, both snapshots must exist

Examples:
```bash
# Compare two snapshots with default text output format
snapshotter cmp --a id1 --b id2

# Compare two snapshots, failing if either doesn't exist
snapshotter cmp --a id1 --b id2 --must

# Compare two snapshots with JSON output format
snapshotter --format json cmp --a id1 --b id2

# Compare two snapshots with YAML output format
snapshotter --format yaml cmp --a id1 --b id2
```

#### serve

Start the HTTP API server for snapshot operations.

```bash
snapshotter serve [flags]
```

Flags:
- `--port int`: Port to listen on (default 8080)

Example:
```bash
# Start the API server
snapshotter --config config.yaml serve --port 9090
```

### API Endpoints

When running in server mode, the following HTTP endpoints are available:

- `GET /snapshots`: List all snapshots
- `GET /snapshots/{id}`: Get a specific snapshot
- `POST /snapshots?instance={source}&resourceType={route|consumer}&limit={n}`: Take a new global snapshot;
  `limit` defaults to `0` (no limit), negative values are rejected with `400`
- `GET /compare?a={id1}&b={id2}`: Compare two snapshots

## Example Workflows

### Continuous Integration

```bash
# Take a snapshot before deployment
snapshotter --config config.yaml snap --source prod --route api-route

# Deploy changes
# ...

# Take a snapshot after deployment
snapshotter --config config.yaml snap --source prod --route api-route

# Compare the before and after states
snapshotter cmp --a "production-aws-api-route" --b "production-aws-api-route"
```

### Route Rollback

```bash
# Before a change: store a raw rollback point in a dedicated store
snapshotter --config cfg.yaml snap --source prod --route my-route --raw --store /secure/rollback

# ... change or break the route ...

# Show what would be restored (dry run)
snapshotter --config cfg.yaml apply --source prod --route my-route --store /secure/rollback

# Restore it, deleting plugins/targets added since the snapshot
snapshotter --config cfg.yaml apply --source prod --route my-route --store /secure/rollback --apply --prune

# Undo the rollback using the backup written by apply
snapshotter --config cfg.yaml apply --source prod --store /secure/rollback --apply \
  --file /secure/rollback/.backups/production-aws-my-route/1767225600.snap.yaml
```

### Monitoring

```bash
# Start the API server
snapshotter --config config.yaml serve --port 8080

# Use monitoring tools to periodically call the API and check for changes
curl http://localhost:8080/snapshots/production-aws-api-route
```

## Advanced Features

### Snapshot Decoders

Decoders transform encoded content within snapshots. For example, if your gateway stores base64-encoded configuration, the decoder can automatically decode it:

```yaml
decoders:
  - type: base64
    pattern: config:([A-Za-z0-9+/=]+)
```

### Data Obfuscation

Obfuscators replace sensitive data with placeholder values to protect sensitive information:

```yaml
obfuscators:
  - pattern: "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"
    replace: "00000000-0000-0000-0000-000000000000"
```

This example replaces UUIDs with a placeholder UUID.
