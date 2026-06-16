## Why

`ApiExposure` and `ApiSubscription` currently do not enforce the live `ApiCategory.allowTeams.categories` policy in the Go control plane. This creates governance drift between declared category policy and runtime behavior.

## What Changes

- Add category-policy validation to `ApiExposure` reconciliation, blocking provisioning when the provider team category is not allowed by the referenced `ApiCategory`.
- Add category-policy validation to `ApiSubscription` reconciliation, blocking provisioning when the subscriber team category is not allowed by the referenced `ApiCategory`.
- Reuse the existing `ApiCategory.allowTeams.categories` policy as the source of truth for allowed team categories.
- Add explicit status conditions/messages to make category mismatches observable and actionable.
- Add tests covering allowed and denied combinations for both resources.

## Capabilities

### New Capabilities

- `api-team-category-alignment`: Enforce consistent team-category and API-category policy checks for API exposure and subscription flows.

### Modified Capabilities

- None.

## Impact

- Affected code: `api/internal/handler/apiexposure`, `api/internal/handler/apisubscription`, shared handler utilities, and corresponding tests.
- Behavioral impact: resources that previously progressed may now become blocked when team category is not permitted by API category policy.
- API/governance impact: aligns runtime enforcement with existing `ApiCategory` governance rules and expected domain behavior.
