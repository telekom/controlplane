---
sidebar_position: 1
---

# Contributing

Welcome to the Control Plane project. We appreciate your interest in contributing. This guide walks you through everything you need to know to get started, from setting up your environment to submitting your first pull request.

## Code of Conduct

This project follows the [Contributor Covenant](https://www.contributor-covenant.org/) v2.1. We are committed to providing a welcoming and inclusive experience for everyone. Please review the full [Code of Conduct](https://github.com/telekom/controlplane/blob/main/CODE_OF_CONDUCT.md) before contributing.

## Licensing

The project follows the [REUSE standard](https://reuse.software/) for software licensing. This means that every file in the repository must contain an SPDX license header with copyright and license information. The header typically looks like this:

```
// SPDX-FileCopyrightText: 2025 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0
```

The full license texts are stored in the [`LICENSES/`](https://github.com/telekom/controlplane/tree/main/LICENSES) folder at the root of the repository. For more details on how to apply REUSE headers, visit [reuse.software](https://reuse.software/) or the [developer guide](https://telekom.github.io/reuse-template/).

## Getting Started

Follow these steps to start contributing:

1. **Fork the repository** — Create a personal fork of the [Control Plane repository](https://github.com/telekom/controlplane) on GitHub.
2. **Clone your fork** — Clone the forked repository to your local machine.
3. **Create a branch** — Create a new branch for your changes. Use a descriptive name, for example `feat/add-widget-support` or `fix/reconciler-nil-pointer`.
4. **Make your changes** — Implement your changes, write tests, and make sure all existing tests pass.
5. **Commit your changes** — Write clear commit messages following the [Conventional Commits](#commit-messages) format.
6. **Push and open a pull request** — Push your branch to your fork and open a pull request against the main repository.

A maintainer will review your pull request and provide feedback. Once approved, your changes will be merged.

## Commit Messages

The project uses [Conventional Commits](https://www.conventionalcommits.org/) to keep the commit history clean and to enable automated releases. This convention is enforced by a pre-commit hook, so your commit will be rejected if the message does not follow the format.

The general format is:

```
type(scope): description
```

Where **type** describes the kind of change:

| Type | Purpose |
|----------|----------------------------------------|
| `feat` | A new feature |
| `fix` | A bug fix |
| `docs` | Documentation changes |
| `chore` | Maintenance tasks, dependency updates |
| `refactor`| Code changes that neither fix a bug nor add a feature |
| `test` | Adding or updating tests |
| `ci` | Changes to CI configuration or scripts |
| `style` | Formatting changes (no code logic change) |
| `perf` | Performance improvements |

The **scope** is optional and indicates the area of the codebase affected (for example, `gateway`, `common`, `ci`). The **description** should be a short, lowercase summary of the change.

**Examples:**

```
feat(gateway): add rate limiting to API endpoints
fix(common): handle nil pointer in reconciler loop
docs: update contributing guidelines
chore: bump Go version to 1.22
```

## Pre-commit Hooks

The project uses [pre-commit](https://pre-commit.com/) to run automated checks on every commit. Three hooks are configured:

| Hook | Version | Purpose |
|------|---------|---------|
| `conventional-pre-commit` | v4.2.0 | Enforces the Conventional Commits format for commit messages |
| `reuse-lint-file` | v5.0.2 | Checks that every file has the required SPDX license headers |
| `gitleaks` | v8.25.1 | Scans for accidentally committed secrets and credentials |

### Setup

Install pre-commit and activate the hooks in your local clone:

```bash
# Install pre-commit
pip install pre-commit

# Activate hooks in the repository
pre-commit install

# Optionally, run hooks on all files to check the current state
pre-commit run --all-files
```

Once installed, the hooks run automatically on every commit. If a hook fails, review the error output, fix the issue, stage the corrected files, and commit again. In some cases, hooks will auto-fix the issue for you — simply stage the updated files and retry the commit.

## Code Quality

The project uses [golangci-lint](https://golangci-lint.run/) for static analysis and linting of Go code. A shared configuration file (`.golangci.yml`) at the root of the repository defines the set of enabled linters and their settings.

Each domain in the repository provides a `make lint` target to run the linter locally:

```bash
make lint
```

Run this before pushing your changes to catch issues early.

## CI Pipeline

Every pull request triggers a CI pipeline that validates your changes. The pipeline runs the following checks for each module in parallel:

- **Static checks** — Linting and code style validation
- **Tests with coverage** — Unit and integration tests with code coverage reporting
- **Vulnerability scan** — [govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) scans dependencies for known vulnerabilities
- **CodeQL analysis** — Automated security analysis to detect common vulnerability patterns
- **Image build and scan** — Container images are built and scanned for security issues

All modules are checked independently and run in parallel, so you will get fast feedback on your changes.

## Release Process

The project uses [Semantic Release](https://semantic-release.gitbook.io/) with the Angular preset to automate versioning and publishing. Releases are triggered automatically based on Conventional Commit messages — for example, a `feat` commit produces a minor version bump, while a `fix` commit produces a patch version bump.

Key details:

- **Release branches:** `master`, `main`, `next`, `next-major`
- **Tag format:** `v${version}` (for example, `v1.2.3`)
- **Container images:** Built with [ko](https://ko.build/) and pushed to `ghcr.io/telekom/controlplane/<name>`

You do not need to manually bump versions or create tags. The release automation handles everything when changes are merged into a release branch.

## Next Steps

- [Local Development](./local-development.md) — Set up your local development environment and learn how to run operators locally.
- [Creating an Operator](./creating-an-operator.md) — Learn how to build a custom operator that extends the Control Plane.
