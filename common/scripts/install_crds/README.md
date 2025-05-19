<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# Install CRDs

This script can be used to install all CRDs that a project requires to the current kubeconfig context.

- It does so by reading the `go.mod` file and looking for all modules matching `cp.ei.telekom.de/.*/api`.
- It then leverages the convention that CRDs are stored in a `crds` directory in the root of the module.
- It installs all CRDs found in the `crds` directory by running `kubectl apply -f <path>`.