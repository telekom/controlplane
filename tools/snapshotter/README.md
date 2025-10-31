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

- Go 1.21 or later
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
    tokenUrl: https://auth.example.com/token  # Authentication token URL
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

- `storePath`: Directory where snapshots will be stored
- `decoders`: Global decoders applied to all sources
- `obfuscators`: Global obfuscators applied to all sources

#### Source Configuration

Each source under `sources` represents a gateway instance to snapshot from:

- `environment`: Environment identifier (e.g., "production", "staging")
- `zone`: Zone or region identifier (e.g., "aws", "azure")
- `url`: Gateway admin API URL
- `tokenUrl`: OAuth token URL for authentication
- `clientId`: OAuth client ID
- `clientSecret`: OAuth client secret
- `scopes` (optional): OAuth scopes for authentication
- `tags` (optional): Tags for filtering gateway resources
- `obfuscators` (optional): Source-specific obfuscators
- `decoders` (optional): Source-specific decoders

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
- `--store string`: Path to the snapshot store (default "./snapshots")

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

# Capture all routes from a source (unlimited)
snapshotter --config config.yaml snap --source my-source

# Capture with limit
snapshotter --config config.yaml snap --source my-source --limit 10
```

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
- `POST /snapshots`: Take a new snapshot
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