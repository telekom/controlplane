<!--
Copyright 2026 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

<p align="center">
  <h1 align="center">Permission Domain</h1>
</p>

<p align="center">
    Permission Domain is an optional feature of the Control Plane that enables fine-grained authorization control for API exposures.
    It translates declarative authorization rules from Rover configurations into permission sets consumed by external permission services.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#features">Features</a> •
  <a href="#usage">Usage</a> •
  <a href="#crds">CRDs</a> •
  <a href="#references">References</a>
</p>

## About

The Permission Domain bridges the gap between the Rover Domain (user-facing API configuration) and external permission/authorization services. When users define authorization rules in their Rover files, this domain processes them and creates the appropriate resources that permission services can consume.

The core functions of the Permission Domain include:
- **Authorization Translation**: Converting Rover authorization specifications into standardized permission sets
- **Cross-Namespace Management**: Creating external PermissionSet resources in zone-specific namespaces
- **Lifecycle Management**: Automatically cleaning up permission sets when Rover resources are deleted
- **Format Flexibility**: Supporting multiple authorization declaration formats (flat, resource-oriented, role-oriented)

The Permission Domain consists of two main operators:
1. **Rover Handler**: Processes authorization fields from Rover resources and creates internal PermissionSet CRs
2. **Permission Operator**: Manages internal PermissionSets and creates corresponding external PermissionSets consumed by permission services

> [!NOTE]
> The permission domain requires the `FEATURE_PERMISSION_ENABLED=true` flag to be set on both the rover operator and rover-server.

## Features

- **Multiple Authorization Formats**: Support for three different ways to express authorization rules
  - **Flat Format**: Direct role + resource + actions specification
  - **Resource-Oriented**: Group permissions by resource with multiple roles
  - **Role-Oriented**: Group permissions by role with multiple resources

- **Zone-Aware**: Automatically routes permission sets to the correct zone namespace based on Rover configuration

- **Automatic Cleanup**: Removes external permission sets when the owning Rover is deleted, preventing orphaned resources

- **Cross-Namespace Management**: Handles permission sets across different namespaces while maintaining proper lifecycle management

## Usage

As this is an optional feature, it must be explicitly enabled before use.

### Enabling the Permission Domain

1. **Deploy the Permission Operator** to your Control Plane cluster with the CRDs and RBAC configured

2. **Enable the Feature Flag** on both the rover operator and rover-server:
   ```yaml
   env:
     - name: FEATURE_PERMISSION_ENABLED
       value: "true"
   ```

3. **Configure your Zone** to support permissions (zones must have a target namespace configured)

### Defining Permissions in Rover

Once enabled, you can define authorization rules directly in your Rover configuration using any of the three supported formats:

#### Flat Format
```yaml
apiVersion: rover.cp.ei.telekom.de/v1
kind: Rover
metadata:
  name: my-api
  namespace: my-team
spec:
  zone: production
  exposures:
    - basePath: /api/v1
      upstream: https://backend.example.com
  authorization:
    - role: admin
      resource: stargate:my-api:v1
      actions:
        - read
        - write
        - delete
    - role: viewer
      resource: stargate:my-api:v1
      actions:
        - read
```

#### Resource-Oriented Format
```yaml
authorization:
  - resource: stargate:my-api:v1
    permissions:
      - role: admin
        actions: [read, write, delete]
      - role: viewer
        actions: [read]
```

#### Role-Oriented Format
```yaml
authorization:
  - role: admin
    permissions:
      - resource: stargate:my-api:v1
        actions: [read, write, delete]
      - resource: stargate:my-api:v2
        actions: [read, write]
```

All three formats are equivalent and will produce the same normalized permission set.

## Architecture

```
┌─────────────────┐
│  Rover (User)   │
│  Configuration  │
└────────┬────────┘
         │
         │ Rover Operator (Handler)
         ▼
┌─────────────────┐
│  PermissionSet  │  Internal CR
│  (Internal)     │  Namespace: my-team
└────────┬────────┘
         │
         │ Permission Operator
         ▼
┌─────────────────┐
│  PermissionSet  │  External CR
│  (External)     │  Namespace: production-zone-ns
└────────┬────────┘
         │
         │
         ▼
┌─────────────────┐
│  Permission     │  External Service
│  Service        │  (e.g., Chevron/PCP)
└─────────────────┘
```

### Resource Flow

1. **User creates Rover** with `authorization` field defined
2. **Rover operator** processes the authorization specification and creates an internal `PermissionSet` in the same namespace
3. **Permission operator** watches internal PermissionSets and:
   - Looks up the Zone configuration to determine the target namespace
   - Creates an external `PermissionSet` (pcp.ei.telekom.de) in the zone's namespace
   - Maintains status references and environment labels for proper lifecycle tracking
4. **External permission service** watches external PermissionSets and enforces authorization policies

### Cleanup Flow

When a Rover is deleted:
1. Kubernetes garbage collection automatically deletes the internal PermissionSet (due to owner reference)
2. Permission operator's Delete handler is triggered
3. Delete handler looks up the external PermissionSet using the status reference
4. External PermissionSet is explicitly deleted from the zone namespace
5. External permission service sees the deletion and removes associated policies

## CRDs
All CRDs can be found here: [CRDs](./config/crd/bases/).

<p>The Permission domain defines the following Custom Resources (CRDs):</p>

<details>
<summary>
<strong>PermissionSet (Internal)</strong>
This CRD represents the normalized authorization rules within the Control Plane.
</summary>

- The internal PermissionSet CR is created in the same namespace as the owning Rover
- The name matches the Rover name
- Contains normalized permissions in flat format (role + resource + actions)
- Has owner reference to the Rover for automatic garbage collection
- Includes zone label for routing to the correct zone namespace
- Status tracks reference to the external PermissionSet
- Conditions track provisioning status and readiness

</details>
<br />

<details>
<summary>
<strong>PermissionSet (External - pcp.ei.telekom.de)</strong>
This CRD represents permissions consumed by external permission services.
</summary>

- The external PermissionSet CR is created in the zone's namespace (not the Rover namespace)
- Has the same name as the internal PermissionSet
- Contains identical permission structure (role + resource + actions)
- Includes environment labels for cross-namespace lifecycle tracking
- Includes owner UID label for janitor cleanup of orphaned resources
- No status subresource (read-only from permission operator's perspective)
- Consumed by external permission enforcement services

</details>
<br />

## References

- Rover Domain: [Rover Documentation](../rover/README.md)
- Rover Server: [Rover Server Documentation](../rover-server/README.md)
