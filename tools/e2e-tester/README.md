<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# E2E-Tester Tool

## Introduction

E2E-Tester is a comprehensive end-to-end testing tool designed specifically for validating [rover-ctl](../../rover-ctl/README.md) commands and operations. It provides a structured and automated approach to ensure that your control plane functions correctly across different environments, versions, and configurations.

The tool executes [rover-ctl](../../rover-ctl/README.md) commands, captures their outputs, and compares them against expected results using a snapshot-based testing methodology. This enables regression detection, consistent behavior verification, and automated validation of complex workflows.

### Key Features

- **Command Execution**: Automatically runs rover-ctl commands with configurable parameters
- **Snapshot-Based Testing**: Captures and compares command outputs against expected values
- **Environment Support**: Tests across multiple environments with different credentials
- **Test Suites**: Organizes tests into logical suites for better management
- **Continuity Options**: Controls whether tests continue after failures
- **Selective Execution**: Filters for specific suites or environments
- **Integration with Snapshotter**: Uses snapshotter tool for state verification
- **Detailed Reporting**: Generates clear success/failure reports with diffs

### Use Cases

- Validating rover-ctl functionality across different environments
- Verifying system behavior after changes or updates
- Regression testing for critical workflows
- Automated validation in CI/CD pipelines
- Testing complex multi-step operations
- Comparing system state across deployments

## Installation and Setup

### Prerequisites

- Go 1.21 or later
- Access to rover-ctl binary
- Access credentials for test environments
- (Optional) Access to snapshotter binary or service for system state verification

### Building from Source

Clone the repository and build the binary:

```bash
git clone <repository-url>
cd e2e-tester
go build -o bin/e2e-tester .
```

Alternatively, use the provided Makefile:

```bash
make build
```

This will create a binary in the `bin` directory.

### Installing

To build and install the binary to your system:

```bash
make install
```

This will build the binary and install it to `/usr/local/bin/e2e-tester`.

### Quick Start

1. Create a basic configuration file (e.g., `config.yaml`):

```yaml
roverctl:
  binary: "roverctl"  # Path to your roverctl binary

environments:
  - name: "test-env"
    token: "env:TEST_TOKEN"  # Use environment variable TEST_TOKEN

suites:
  - name: "basic-suite"
    environments:
      - "test-env"
    cases:
      - name: "version-check"
        must_pass: true
        command: "--version"
        compare: true
```

2. Set up your environment variables:

```bash
export TEST_TOKEN="your-token-value"
```

3. Run the tests:

```bash
./bin/e2e-tester --config config.yaml
```

## Configuration

The E2E-Tester is configured through a YAML file that specifies all testing parameters, environments, and test cases.

### Configuration Structure

```yaml
# Snapshotter configuration (optional)
snapshotter:
  # Either specify a URL to a running snapshotter service
  url: "http://localhost:8080"
  # Or specify a path to the snapshotter binary
  binary: "snapshotter"

# RoverCtl configuration
roverctl:
  binary: "roverctl"  # Path to your roverctl binary
  run_as: "user"      # Optional: Run commands as this user

# Test environments
environments:
  - name: "team-a"    # Unique name for the environment
    token: "env:TEAM_A_TOKEN"  # Token for authentication, can use env variable with "env:" prefix
  - name: "team-b"
    token: "env:TEAM_B_TOKEN"

# Test suites
suites:
  - name: "basic-suite"  # Unique name for the test suite
    description: "Optional description of the test suite"
    environments:
      - "team-a"        # List of environments to run this suite in
    cases:
      # Test cases defined below
```

### Environment Configuration

Each environment represents a separate context with its own authentication:

```yaml
environments:
  - name: "production"    # Environment identifier
    token: "env:PROD_TOKEN"  # Reference to environment variable
  - name: "staging"
    token: "direct-token-value"  # Direct token value (not recommended for security)
```

### Test Suite Configuration

Test suites group related test cases together:

```yaml
suites:
  - name: "api-validation"
    description: "Validates API functionality"
    environments:
      - "production"     # Run this suite in these environments
      - "staging"
    cases:
      # Test cases defined below
```

You can also define environments at the case level, which overrides the suite-level setting:

```yaml
cases:
  - name: "sensitive-test"
    environment: "staging"  # Only run this case in staging
    command: "dangerous-operation"
```

### Test Case Configuration

Each test case defines a specific command to run and how to validate its output:

```yaml
cases:
  - name: "version-check"     # Name of the test case
    description: "Verify rover-ctl version"  # Optional description
    type: "roverctl"          # Type of command (default: "roverctl")
    must_pass: true           # Whether this test must pass for suite to succeed
    command: "--version"      # Command to execute
    compare: true             # Whether to compare with snapshot
    wait_before: 5s           # Optional: Wait before executing
    wait_after: 2s            # Optional: Wait after executing
    timeout: 30s              # Optional: Command timeout
    selector: "$.version"     # Optional: JSON path selector for partial output comparison
```

Special test case types:

```yaml
- name: "system-state-snapshot"
  type: "snapshot"             # Use the snapshotter tool
  command: "snap --source source-name --route route-name"
  compare: true
  selector: "$.b"              # Common selector for snapshotter output
```

## Command-Line Options

The E2E-Tester provides several command-line options to control its execution:

```bash
./e2e-tester [flags]
```

### Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Path to the configuration file | `e2e-test-config.yaml` |
| `--update` | Update snapshots instead of comparing | `false` |
| `--continue` | Continue execution even if tests fail | `false` |
| `--snapshots-dir` | Directory to store snapshots | `snapshots` |
| `--log-level` | Log level (debug, info, warn, error) | `info` |
| `--dev` | Enable development mode logging | `false` |
| `--verbose` | Show detailed output including complete diffs | `false` |
| `--suite` | Run only the specified test suite (by name) | all suites |
| `--env` | Run tests only in the specified environment (by name) | all environments |

### Example Usage

Run all tests:
```bash
./e2e-tester --config config.yaml
```

Update snapshots instead of comparing:
```bash
./e2e-tester --config config.yaml --update
```

Run only a specific test suite:
```bash
./e2e-tester --config config.yaml --suite basic-suite
```

Run only in a specific environment:
```bash
./e2e-tester --config config.yaml --env production
```

Run with verbose output:
```bash
./e2e-tester --config config.yaml --verbose
```

Continue after test failures:
```bash
./e2e-tester --config config.yaml --continue
```

Combine filters to run a specific suite in a specific environment:
```bash
./e2e-tester --config config.yaml --suite api-validation --env staging
```

## Test Case Definitions

Test cases are the building blocks of the E2E-Tester. They define individual commands to execute and validation criteria for those commands.

### Basic Test Case Structure

Each test case must have:
1. A unique name within its suite
2. A command to execute
3. Whether the command must pass for the test to succeed
4. Whether to compare output with a snapshot

```yaml
- name: "version-check"
  must_pass: true
  command: "--version"
  compare: true
```

### Test Case Types

The E2E-Tester supports multiple test case types:

1. **roverctl (default)**: Executes a rover-ctl command
   ```yaml
   - name: "apply-config"
     command: "apply -f ./examples/test-files"
   ```

2. **snapshot**: Executes a snapshotter command to capture system state
   ```yaml
   - name: "system-state-snapshot"
     type: "snapshot"
     command: "snap --source poc-dataplane1 --route my-route"
   ```

### Optional Test Case Parameters

You can enhance test cases with additional parameters:

```yaml
- name: "complex-test"
  description: "Tests complex API functionality"
  must_pass: true
  command: "apply -f ./config/complex-api.yaml"
  compare: true
  wait_before: 5s     # Wait before execution
  wait_after: 2s      # Wait after execution
  timeout: 30s        # Command timeout
  selector: "$.items" # JSON path selector for comparison
  environment: "production"  # Override suite-level environment
```

### Test Case Examples

#### Basic Version Check
```yaml
- name: "version-check"
  must_pass: true
  command: "--version"
  compare: true
```

#### Resource Creation and Verification
```yaml
- name: "create-resource"
  must_pass: true
  command: "apply -f ./examples/test-files/resource.yaml"
  compare: true
  wait_after: 2s

- name: "verify-resource"
  must_pass: true
  command: "get-info --name test-resource"
  compare: true
```

#### System Snapshot
```yaml
- name: "system-state-snapshot"
  type: "snapshot"
  must_pass: false
  command: "snap --source dataplane1 --route api-route-v1"
  compare: true
  selector: "$.b"
  wait_before: 5s
```

#### Cleanup
```yaml
- name: "delete-resources"
  must_pass: false  # Marking as non-critical for test success
  command: "delete -f ./examples/test-files"
  compare: true
```

#### Environment-Specific Test
```yaml
- name: "production-only-test"
  environment: "production"
  must_pass: true
  command: "special-command --production-flag"
  compare: true
```

## Snapshotter Integration

The E2E-Tester integrates with the Snapshotter tool to provide system state comparison capabilities, enabling powerful end-to-end testing scenarios.

### What is Snapshotter?

The Snapshotter is a companion tool that captures and compares the state of API gateway services and other control plane components. It provides:

- State capturing of routes, consumers, and other gateway resources
- Versioned snapshot storage
- Diff generation between snapshots
- Data obfuscation for sensitive information

For more details, see the [Snapshotter README](../snapshotter/README.md).

### Configuration

To use Snapshotter with E2E-Tester, configure it in your test configuration file:

```yaml
# Snapshotter configuration (either URL or binary)
snapshotter:
  # Use a running snapshotter service
  url: "http://localhost:8080"
  # OR
  # Use the snapshotter binary directly
  binary: "/path/to/snapshotter"
```

The E2E-Tester will automatically create a snapshotter configuration file (`snapshotter-config.yaml`) or use an existing one if present.

### Creating Snapshot Test Cases

To create a snapshot test case, use the `snapshot` type:

```yaml
- name: "api-route-snapshot"
  type: "snapshot"  # Indicates this is a snapshotter operation
  must_pass: true
  command: "snap --source production-gateway --route api-route-v1"
  compare: true
  selector: "$.b"  # Snapshotter puts the new snapshot in $.b
```

### Snapshot Comparison

When using `--update` mode, the snapshotter will create new reference snapshots. In normal mode, it will compare the current state against the stored snapshots.

The comparison results include:
- Whether changes were detected
- Number of changes
- Full diff output (visible with `--verbose` flag)

### Snapshot Storage

Snapshots are stored in the directory specified by `--snapshots-dir` (default: `snapshots`) in the following structure:

```
snapshots/
  <suite-name>_<environment>_<case-index>_<case-name>/
    1.snap.yaml
```

For example:
```
snapshots/
  basic-suite_team-a_0_version-check/
    1.snap.yaml
  basic-suite_team-a_1_apply-config/
    1.snap.yaml
```

### Advanced Snapshot Features

The E2E-Tester leverages several advanced features of the Snapshotter:

1. **Decoders**: Automatically decodes base64-encoded content in snapshots
2. **Obfuscators**: Masks sensitive information like UUIDs and timestamps
3. **Selectors**: Uses JSONPath selectors for focused comparisons
4. **Multiple Sources**: Can snapshot from different gateway sources

## Sample Workflows

This section provides complete examples of common workflows using the E2E-Tester.

### Basic API Validation Workflow

This workflow validates basic rover-ctl functionality:

```yaml
suites:
  - name: "basic-api-validation"
    environments:
      - "team-a"
    cases:
      - name: "version-check"
        must_pass: true
        command: "--version"
        compare: true

      - name: "apply-config"
        must_pass: true
        command: "apply -f ./examples/test-files"
        compare: true
        wait_after: 2s

      - name: "get-info"
        must_pass: true
        command: "get-info --name test-rover"
        compare: true

      - name: "delete"
        must_pass: true
        command: "delete -f ./examples/test-files"
        compare: true
```

### Multi-Environment Testing

This workflow tests the same operations across multiple environments:

```yaml
environments:
  - name: "development"
    token: "env:DEV_TOKEN"
  - name: "staging"
    token: "env:STAGING_TOKEN"

suites:
  - name: "cross-environment-validation"
    environments:
      - "development"
      - "staging"
    cases:
      - name: "apply-config"
        must_pass: true
        command: "apply -f ./examples/test-files"
        compare: true
        wait_after: 2s

      - name: "get-info"
        must_pass: true
        command: "get-info --name test-rover"
        compare: true

      - name: "delete"
        must_pass: true
        command: "delete -f ./examples/test-files"
        compare: true
```

### API Gateway State Validation

This workflow tests API gateway configuration and state:

```yaml
suites:
  - name: "gateway-validation"
    environments:
      - "production"
    cases:
      - name: "apply-route"
        must_pass: true
        command: "apply -f ./examples/gateway/route.yaml"
        compare: true
        wait_after: 5s

      - name: "snapshot-route-state"
        type: "snapshot"
        must_pass: true
        command: "snap --source prod-gateway --route my-api-route"
        compare: true
        selector: "$.b"
        wait_before: 2s

      - name: "verify-route-exists"
        must_pass: true
        command: "get-info --name my-api-route"
        compare: true

      - name: "delete-route"
        must_pass: true
        command: "delete -f ./examples/gateway/route.yaml"
        compare: true

      - name: "verify-route-deleted"
        type: "snapshot"
        must_pass: true
        command: "snap --source prod-gateway --route my-api-route"
        compare: true
        selector: "$.b"
        wait_before: 2s
```

### CI/CD Pipeline Integration

Example of using E2E-Tester in a CI/CD pipeline:

```bash
#!/bin/bash
# CI/CD script for E2E testing

# Export required tokens
export TEAM_A_TOKEN="your-token-here"
export TEAM_B_TOKEN="your-token-here"

# Step 1: Create baseline snapshots before deployment (update mode)
./e2e-tester --config config.yaml --snapshots-dir baseline --update

# Step 2: Deploy new version
# ... deployment commands ...

# Step 3: Run tests against the new deployment
./e2e-tester --config config.yaml --snapshots-dir current

# Step 4: Compare baseline with current (if needed)
# ... comparison commands ...

# Exit with the test status
exit $?
```