## Context

The current Go control plane validates API categories during `ApiSpecification` admission, but `ApiExposure` and `ApiSubscription` handlers still need to enforce the live `ApiCategory.allowTeams.categories` policy at runtime. This leaves a gap where category policy can be bypassed after API registration and leads to inconsistent governance outcomes.

Both handlers already resolve the active `Api` and the related `Application`, which provides all data needed for policy evaluation:
- API category from `Api.Spec.Category`
- Team category from `Application -> Team.Spec.Category`

## Goals / Non-Goals

**Goals:**
- Enforce `ApiCategory.allowTeams.categories` policy in `ApiExposure` handling.
- Enforce `ApiCategory.allowTeams.categories` policy in `ApiSubscription` handling.
- Use `ApiCategory` resources as the policy source of truth for allowed team categories, including the unrestricted case when `allowTeams` is omitted.
- Surface clear blocked conditions/messages when policy denies the action.
- Keep behavior aligned across API registration and runtime API-domain resources.

**Non-Goals:**
- Changing `ApiCategory` CRD schema or policy semantics.
- Introducing new category mapping models.
- Refactoring unrelated exposure/subscription validation logic.

## Decisions

### 1. Evaluate category policy in both handlers before downstream provisioning
Both `ApiExposure` and `ApiSubscription` SHALL perform category policy checks early in reconciliation, after required objects (`Api`, `Application`/Team context) are resolved and before creating/updating gateway resources.

**Rationale:** prevents side effects for requests that should be denied by governance policy.

**Alternative considered:** checking only in admission/webhook.  
**Why not chosen:** runtime resources can still drift from admission-time assumptions; enforcement should be consistent where provisioning happens.

### 2. Reuse `ApiCategory` allow-teams-by-category policy (`IsAllowedForTeamCategory`)
The handlers SHALL fetch the referenced `ApiCategory` by label value (`Api.Spec.Category`) and evaluate `IsAllowedForTeamCategory(teamCategory)`.

**Rationale:** reuses existing policy contract and avoids parallel rule implementations.

**Alternative considered:** recreate legacy hard-coded valid category set.  
**Why not chosen:** static lists are harder to govern and diverge from current CRD-based policy.

### 3. Fail closed when policy information is unavailable or not allowed
If `ApiCategory` is missing/inactive/not allowed for the team category, handlers SHALL block the resource with explicit condition reason and message.

**Rationale:** governance should be deterministic and conservative.

**Alternative considered:** permissive fallback.  
**Why not chosen:** would reintroduce unpredictable access control behavior.

## Risks / Trade-offs

- **[Risk] Existing resources may start blocking after rollout** → Mitigation: provide clear condition reasons/messages and release notes for operators to align category policies.
- **[Risk] Additional reconciliation lookups for policy objects** → Mitigation: single object lookup per reconciliation path and only after prerequisite resources are present.
- **[Trade-off] Stricter enforcement may surface previously hidden policy misconfigurations** → Mitigation: expected and desired for governance correctness.

## Migration Plan

1. Implement validation utility and wire it into both handlers.
2. Add unit/integration tests for allowed and denied category combinations.
3. Deploy with release notes describing stricter runtime category enforcement.
4. Rollback strategy: revert handler-level category check commit if unexpected operational impact occurs.

## Open Questions

- Should we add a feature flag for gradual rollout, or enforce immediately as default behavior?
