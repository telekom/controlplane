<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# Install CRDs

This script can be used to install all CRDs that a project requires to the current kubeconfig context.

- It does so by reading the `go.mod` file and looking for all modules matching `^github\.com/telekom/controlplane/.*$`.
- It then leverages the convention that CRDs are stored in a `config/crd/bases` directory in the root of the module.
- If the module is replaced by a local path, it will use that path to find the CRDs.
- It installs all CRDs found in the `config/crd/bases` directory by running `kubectl apply -f <path>`.