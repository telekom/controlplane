---
sidebar_position: 2
---

# API Categories

API Categories let you classify the APIs on your platform into logical groups — such as "internal", "partner", or "public" — and enforce rules for each group. When a team registers an API, the platform checks its category and validates it against the rules you have defined.

With API Categories, you can:

- **Organize** APIs by purpose or audience
- **Restrict** which teams are allowed to register APIs under a given category
- **Enforce naming conventions** by requiring the team's group name in the API base path
- **Enable linting** to validate OpenAPI specifications against a ruleset

## How it works

Every OpenAPI specification submitted to the platform includes a custom field called `x-api-category` in its `info` section. This field tells the platform which category the API belongs to:

```yaml
openapi: "3.0.0"
info:
  title: "My API"
  version: "1.0.0"
  x-api-category: "internal"
```

When the specification is submitted, the platform looks up the matching `ApiCategory` resource and validates the request against its rules. If the category does not exist, is inactive, or the team is not allowed to use it, the request is rejected.

:::info
If no `ApiCategory` resources exist in the environment, the platform skips category validation entirely and accepts any value. This means you can adopt API Categories gradually — they only take effect once you create your first one.
:::

:::caution
If a team's OpenAPI specification does not include an `x-api-category` field, the category defaults to `"other"`. Once you start using API Categories, make sure to create a category with `labelValue: "other"` if you want to accept APIs that do not specify a category explicitly.
:::

## Creating an API Category

An `ApiCategory` is a Kubernetes resource that you apply to the environment namespace. Here is a minimal example:

```yaml
apiVersion: api.cp.ei.telekom.de/v1
kind: ApiCategory
metadata:
  name: internal
  namespace: controlplane
spec:
  labelValue: "internal"
  active: true
  description: "APIs intended for internal use within the organization."
```

The key fields are:

- **`labelValue`** — The value that teams must use in the `x-api-category` field of their OpenAPI specification. It must be unique within the environment and can be between 1 and 20 characters long. Matching is **case-insensitive** — a specification with `x-api-category: "Internal"` will match a category with `labelValue: "internal"`.
- **`active`** — Must be set to `true` for the category to accept new APIs. If omitted, it defaults to `false` (inactive).
- **`description`** — An optional human-readable description (maximum 256 characters).

Apply the resource:

```bash
kubectl apply -f apicategory-internal.yaml
```

Verify:

```bash
kubectl get apicategories -n controlplane
```

## Configuration options

The following sections describe all available configuration options for an `ApiCategory`.

### Team restrictions

By default, any team can register APIs under any category. The `allowTeams` field lets you restrict access based on **team names** and **team categories**. When `allowTeams` is set, the platform checks both dimensions — a team must be allowed by **both** `names` and `categories` to use the category.

:::caution
When you set `allowTeams`, any dimension you leave empty will deny all teams for that dimension. Always use `"*"` as a wildcard for any dimension you want to leave unrestricted.
:::

**By team name** — only specific teams can use the category, regardless of their team category:

```yaml
spec:
  labelValue: "partner"
  active: true
  allowTeams:
    names:
      - "phoenix--firebirds"
      - "phoenix--thunderbolts"
    categories:
      - "*"
```

**By team category** — only teams belonging to certain categories, regardless of their name:

```yaml
spec:
  labelValue: "partner"
  active: true
  allowTeams:
    categories:
      - "premium"
    names:
      - "*"
```

**Combining both** — restrict by name and category at the same time:

```yaml
spec:
  labelValue: "partner"
  active: true
  allowTeams:
    categories:
      - "premium"
    names:
      - "phoenix--firebirds"
```

In this example, a team must belong to the `premium` category **and** be named `phoenix--firebirds`.

### Group prefix enforcement

By default, the platform requires that every API base path starts with the team's group name. For example, a team in the group `phoenix` must use a base path like `/phoenix/my-api/v1`.

This behavior is controlled per category with the `mustHaveGroupPrefix` field:

```yaml
spec:
  labelValue: "shared"
  active: true
  mustHaveGroupPrefix: false
```

When set to `false`, teams in this category can use any base path structure. The default is `true`.

### API linting

API Categories can enforce linting on submitted OpenAPI specifications. When linting is enabled, every specification registered under this category is validated against the specified ruleset.

```yaml
spec:
  labelValue: "public"
  active: true
  linting:
    enabled: true
    ruleset: "strict"
```

| Field | Description |
| ----- | ----------- |
| `enabled` | Turns linting on or off for this category. |
| `ruleset` | The name of the ruleset to validate against. |

### Activating and deactivating categories

The `active` field controls whether a category accepts new API registrations:

- **`active: true`** — Teams can register new APIs under this category.
- **`active: false`** — New registrations are rejected. Existing APIs already using this category are not affected.

This is useful when you want to retire a category without disrupting the APIs that already use it.

## Full example

The following example creates an API Category for public-facing APIs with strict governance:

```yaml
apiVersion: api.cp.ei.telekom.de/v1
kind: ApiCategory
metadata:
  name: public
  namespace: controlplane
spec:
  labelValue: "public"
  active: true
  description: "Public-facing APIs available to external consumers."
  mustHaveGroupPrefix: true
  allowTeams:
    categories:
      - "premium"
    names:
      - "phoenix--firebirds"
  linting:
    enabled: true
    ruleset: "strict"
```

This category:
- Requires the `x-api-category: "public"` field in the OpenAPI specification
- Only allows teams that belong to the `premium` category **and** are named `phoenix--firebirds`
- Enforces group prefix in the API base path
- Validates specifications against the `strict` linting ruleset

## Verifying your setup

After creating your API Categories, use the following commands to inspect them:

```bash
# List all API categories
kubectl get apicategories -n controlplane

# View details of a specific category
kubectl describe apicategory public -n controlplane
```

To test that validation is working, try submitting an `ApiSpecification` with an invalid or restricted category and verify that the request is rejected.

## Next Steps

- [Environments & Zones](../environments-and-zones.md) — Configure the infrastructure that hosts your APIs
- [Organizations & Teams](../organizations-and-teams.md) — Set up the teams that will register APIs
- [API Domain Architecture](../../architecture/api.mdx) — Explore the full ApiCategory specification and related resources
