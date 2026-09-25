<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0    
-->

# Development and Contribution Guidelines

## Table of contents

  - [Verification Workflow](#verification-workflow)
  - [Pre-commit Hooks](#pre-commit-hooks)
  - [GraphQL Naming](#graphql-naming)
  - [Kubebuilder](#kubebuilder)

## Verification workflow

Checks are split into two tiers to keep commits fast while catching everything before code reaches the remote:

| When                         | What                                                             | Why                                                 |
|------------------------------|------------------------------------------------------------------|-----------------------------------------------------|
| **Pre-commit**               | REUSE license headers, gitleaks, conventional commit format      | Fast (<2s), runs on every commit                    |
| **Pre-push / `make verify`** | `go mod tidy`, `make build`, `golangci-lint` per module | Thorough but slow — only needed before sharing code |

Always use Makefile targets instead of running Go tooling directly. The per-module Makefiles ensure correct flags, code generation, envtest setup, and coverage configuration.

```bash
# From a module directory
make build          # build (includes generate, fmt, vet)
make test           # test (includes envtest + coverage)
make lint           # golangci-lint

# From the repo root
make verify         # all pre-push checks on every module with a Makefile
make verify MODULES="gateway identity"  # check specific modules only
make build-all      # build every module
make test-all       # test every module
```

The `make verify` target delegates to `hack/verify.sh`, which is the single source of truth for pre-push validation. It can be called from CI, git hooks, or AI agent hooks without any tool-specific setup.

## Pre-commit hooks

We use [pre-commit](https://pre-commit.com/) to ensure that our code meets various standards and best-practices.
Pre-commit is a tool that will run checks against a codebase **on every commit**.
If any of these checks fail, you have two options:

1. Manually resolve the issues: Review the error messages provided by the hooks and fix the problems in your codebase.
2. Let the hooks fix the issues: In some cases, the hooks will automatically update your code to meet the required standards. If this happens, simply stage the changes and attempt the commit again.

This ensures that your code adheres to the project's conventions before being committed.

Our repository has a **configuration file** called `.pre-commit-config.yaml`, located in the root directory of the repository.
This contains all of the instructions and extensions to use with pre-commit.

To get started with `pre-commit`, follow these steps:

- **Install `pre-commit`**: 

  You can install it using `pip` with the command `pip install pre-commit`. For more details, refer to [the `pre-commit` installation instructions](https://pre-commit.com/#install).

- **Activate `pre-commit` in the repository**: 

  To activate pre-commit, run the following command:

  ```bash
  pre-commit install
  ```

  This will check the `.pre-commit-config.yaml` file and install the needed dependencies for this repository.

With this setup, `pre-commit` will now automatically run checks on every commit.

You may also manually run it with the following command:

```bash
pre-commit run
```

### Running pre-commit on all files

By default, pre-commit will only run on the **changed files** in a commit.
To run it for **all files at once**, use the following command:

```bash
pre-commit run --all-files
```

## GraphQL naming

Follow the [GraphQL naming conventions](https://graphql.org/learn/naming-design/#apply-field-and-type-naming-patterns) with Go-style initialisms:

- Use `camelCase` for fields, arguments, input fields, and directives.
- Use `PascalCase` for types, interfaces, unions, and enums.
- Keep initialisms uppercase, such as `clientID`, `gatewayURL`, and `externalIDP`.
- Use uppercase enum values. Add underscores where they are part of the established domain value.
- Apply the same initialisms to Ent schema types and edges, for example `APIExposure` and `exposed_APIs`.
- When a name change would change a database identifier, pin the old name with `entsql.Table` or `StorageKey`. Ent converts `APIs` to `ap_is` and `IDs` to `i_ds`.
- The test in `controlplane-api/internal/database/schema_snapshot_test.go` compares all database identifiers with `testdata/schema.golden`. For an intended schema change, run `UPDATE_SCHEMA_SNAPSHOT=1 make test` in `controlplane-api` and review the snapshot diff.

Examples of valid enum values include `CLIENT_SECRET_BASIC`, `SEMIGRANTED`, and `TELECONTEXTMCP`.

## Kubebuilder

We use Kubebuilder to scaffold and manage Kubernetes APIs/controllers. 
For local development and code generation, we require a pinned version:

- **Version**: 4.9.0
- **Release notes**: https://github.com/kubernetes-sigs/kubebuilder/releases/tag/v4.9.0

### Install Kubebuilder 4.9.0


For detailed information, please take a look at [Kubebuilder's installation instructions](https://book.kubebuilder.io/quick-start#installation) to get the installation guide for your platform.
For reference, to validate your Kubebuilder version, type the following command:

```console
kubebuilder version
```

The output should look like something like this:

```
❯  kubebuilder version
Version: cmd.version{KubeBuilderVersion:"4.9.0", KubernetesVendor:"1.34.0", GitCommit:"5e331e74c7a25c8e8fc0d9d5c33c319b7268f395", BuildDate:"2025-09-22T10:53:21Z", GoOs:"linux", GoArch:"amd64"}
```
