---
sidebar_position: 3
---

# JSON Schemas

The Control Plane publishes JSON Schema files for all of its Custom Resource Definitions (CRDs). These schemas provide autocompletion, inline validation, and hover documentation when editing Control Plane YAML manifests — directly in your editor, without any plugins.

:::warning Version drift
The schemas published here reflect the CRD definitions at the time this documentation was built. If you are running a different version of the Control Plane, fields may have been added, removed, or changed since then. Always verify that the schema version matches the version deployed in your cluster.
:::

## Usage

Add a `yaml-language-server` comment as the **first line** of your YAML file. This tells the editor which schema to use for validation and autocompletion:

```yaml title="my-rover.yaml"
# yaml-language-server: $schema=https://telekom.github.io/controlplane/schemas/rover.cp.ei.telekom.de/rover_v1.json
apiVersion: rover.cp.ei.telekom.de/v1
kind: Rover
metadata:
  name: my-rover
  namespace: default
spec:
  zone: aws
  exposures:
    - ...
```

Once the comment is in place, your editor provides:

- **Autocompletion** for all fields, including nested objects and enum values
- **Inline validation** that flags missing required fields, incorrect types, and constraint violations
- **Hover documentation** showing the description of each field

This works out of the box in VS Code, Neovim, JetBrains IDEs, and any other editor that supports the [YAML Language Server](https://github.com/redhat-developer/yaml-language-server).

### Schema URL Pattern

All schemas follow a consistent URL pattern:

```
https://telekom.github.io/controlplane/schemas/{group}/{kind}_{version}.json
```

### Workspace-Wide Configuration

Instead of adding a comment to every file, you can configure schema associations for your entire workspace. In VS Code, create or edit `.vscode/settings.json`:

```json title=".vscode/settings.json"
{
  "yaml.schemas": {
    "https://telekom.github.io/controlplane/schemas/rover.cp.ei.telekom.de/rover_v1.json": [
      "**/rovers/**/*.yaml",
      "**/rover*.yaml"
    ],
    "https://telekom.github.io/controlplane/schemas/admin.cp.ei.telekom.de/environment_v1.json": [
      "**/environments/**/*.yaml"
    ]
  }
}
```

Other editors offer similar configuration. Refer to the [YAML Language Server documentation](https://github.com/redhat-developer/yaml-language-server#language-server-settings) for the corresponding settings in your editor.

## Available Schemas

The following tables list all available schemas, grouped by domain. Click a schema link to view or download the file.

<CRDSchemaList />

---

### SFTP

| Kind | Schema |
| ---- | ------ |
| **Instance** | [`instance_v1.json`](pathname:///schemas/sftp.cp.ei.telekom.de/instance_v1.json) |
| **User** | [`user_v1.json`](pathname:///schemas/sftp.cp.ei.telekom.de/user_v1.json) |
| **SFTPServiceConfig** | [`sftpserviceconfig_v1.json`](pathname:///schemas/sftp.cp.ei.telekom.de/sftpserviceconfig_v1.json) |

---

## Schema Index

A machine-readable index of all available schemas is published at [`schemas/index.json`](pathname:///schemas/index.json). This can be used by tooling to discover schemas programmatically.

## What the Schemas Include

The schemas are generated from the same CRD definitions that the Kubernetes API server uses for validation. They include the following enrichments:

- **Pinned `apiVersion` and `kind`** — constrained to their exact values, so typos in these fields are flagged immediately.
- **Metadata autocompletion** — the `metadata` section provides suggestions for `name`, `namespace`, `labels`, and `annotations`.
- **Full field validation** — enums, patterns, min/max constraints, and format annotations, matching what the API server enforces.

:::note
[CEL validation rules](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#validation-rules) (cross-field validation expressions) are present in the schemas as `x-kubernetes-validations` annotations but cannot be enforced by the YAML language server. These rules are only evaluated server-side by the Kubernetes API server.
:::
