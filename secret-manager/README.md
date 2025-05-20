<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# Secret Manager

Secret Manager (SM) is a REST-ful API for managing secrets. It allows you to store, retrieve, and delete secrets securely. 

> The main goal of this is to obfuscate the secrets in Custom-Resources (CR) with placeholders "secret references".

The following problems are solved:

0. **Secret obfuscation:** The SM is used to replace secrets in CRs with a placeholder and if they secret is needed, retrieve it (jit).
1. **Secret onboarding:** The SM provides a unified API for onboarding new entities and secrets.
2. **Secret storage:** The SM provides a unified API for storing secrets in different backends.
3. **Secret retrieval:** The SM provides a unified API for retrieving secrets using references (IDs) to secrets.
4. **Auditing:** As the SM is a single point of access for secrets, it can provide auditing capabilities for secret access.

The following diagram provides a high-level overview of how the SM is integrated into the Controlplane.

![Architecture Diagram](docs/overview.drawio.svg)



## Backends

The SM itself does not store anything. This is done using backend implementations.
Currently the SM supports the following backends:

### Kubernetes Secrets

This backend uses Kubernetes Secrets to store secrets. It will therefore work with any Kubernetes cluster. 

### Conjur

## Security

## Usage

