## Why

`ApiExposure` and `ApiSubscription` currently do not enforce team-category vs API-category compatibility in the Go control plane, while the legacy Stargate implementation performed category validation before provisioning. This gap creates governance drift and unpredictable access behavior across API domain resources.

## What Changes

- Add category-compatibility validation to `ApiExposure` reconciliation, blocking provisioning when the provider team category is not allowed for the API category.
- Add category-compatibility validation to `ApiSubscription` reconciliation, blocking provisioning when the subscriber team category is not allowed for the API category.
- Reuse the existing `ApiCategory` policy model (`allowTeams.categories`) as the source of truth for allowed team categories.
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
