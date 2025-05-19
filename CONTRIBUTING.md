<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0    
-->

# Development and Contribution Guidelines

## Table of contents

  - [Pre-commit Hooks](#pre-commit-hooks)

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