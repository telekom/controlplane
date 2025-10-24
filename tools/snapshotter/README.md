<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# Snapshotting Tool

This tool is intended to create snapshots of the current state of the system. Mainly for the gateway-domain.

## Installation

Build the binary using:

```bash
make build
```

This will create a binary in the `bin` directory.

## Usage

The snapshotter tool is a unified CLI with multiple subcommands:

```bash
snapshotter [global flags] command [command flags]
```

### Global Flags

- `--config string`: Path to the configuration file

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
# Take a snapshot of a route
snapshotter --config config.yaml snap --source my-source --route my-route

# Take a snapshot of a consumer
snapshotter --config config.yaml snap --source my-source --consumer my-consumer
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
- `--store string`: Path to the snapshot store (default "./snapshots")

Examples:
```bash
# Compare two snapshots
snapshotter cmp --a id1 --b id2 --store ./snapshots

# Compare two snapshots, failing if either doesn't exist
snapshotter cmp --a id1 --b id2 --must
```

#### serve

Start the HTTP API server for snapshot operations.

```bash
snapshotter serve [flags]
```

Flags:
- `--port int`: Port to listen on (default 8080)
- `--store string`: Path to the snapshot store (default "./snapshots")

Example:
```bash
# Start the API server
snapshotter --config config.yaml serve --port 9090
```

## Configuration

The tool uses a YAML configuration file. An example configuration can be found in `config/example.yaml`.