<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0    
-->

<p align="center">
  <img src="docs/pages/static/img/Open-Telekom-Integration-Platform_Visual.svg" alt="Open Telekom Integration Platform logo" width="200">
  <h1 align="center">Control Plane</h1>
</p>

<p align="center">
 A centralized management layer that maintains the desired state of your systems by orchestrating workloads, scheduling, and system operations through a set of core and custom controllers.
</p>

## About

As Part of Open Telekom Integration Platform, the Control Plane is the central management layer that governs the operation of your Kubernetes cluster. It maintains the desired state of the system, manages workloads, and provides interfaces for user interaction and automation.

The Control Plane components run on one or more nodes in the cluster and coordinate all cluster activities, including scheduling, monitoring, and responding to events.

## Documentation

For complete documentation, please visit the Control Plane documentation site:

- [Control Plane Documentation](https://telekom.github.io/controlplane/)

The documentation includes:

- [Overview and Architecture](https://telekom.github.io/controlplane/docs/Overview/controlplane)
- [Component Details](https://telekom.github.io/controlplane/docs/Overview/components)
- [Operators](https://telekom.github.io/controlplane/docs/Overview/operators)
- [Technology Overview](https://telekom.github.io/controlplane/docs/Technology/technology)
- [Installation Guide](https://telekom.github.io/controlplane/docs/Installation/installation)
- [Quickstart Guide](https://telekom.github.io/controlplane/docs/Installation/quickstart)

## Getting Started

To quickly get started with Control Plane:

```bash
# Clone the repository
git clone https://github.com/telekom/controlplane.git

# Navigate to the local installation directory
cd controlplane/install/local

# Install Control Plane components
kubectl apply -k .
```

For detailed installation instructions and configuration options, refer to the [Installation Guide](https://telekom.github.io/controlplane/docs/Installation/installation) and [Quickstart Guide](https://telekom.github.io/controlplane/docs/Installation/quickstart).

## Code of Conduct

This project has adopted the [Contributor Covenant](https://www.contributor-covenant.org/) in version 2.1 as our code of conduct. Please see the details in our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). All contributors must abide by the code of conduct.

## Licensing

This project follows the [REUSE standard for software licensing](https://reuse.software/).    
Each file contains copyright and license information, and license texts can be found in the [./LICENSES](./LICENSES) folder. For more information visit https://reuse.software/.    
You can find a guide for developers at https://telekom.github.io/reuse-template/.