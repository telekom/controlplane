## 1. Shared category-policy validation foundation

- [ ] 1.1 Add or extend a shared handler utility that resolves `ApiCategory` from `Api.Spec.Category` and evaluates `IsAllowedForTeamCategory(teamCategory)`.
- [ ] 1.2 Define consistent blocked-condition reason/message mapping for allowlist denials and policy-resolution failures.

## 2. ApiExposure enforcement

- [ ] 2.1 Integrate `ApiCategory.allowTeams.categories` validation into `ApiExposure` reconciliation before route/proxy provisioning.
- [ ] 2.2 Add/adjust `ApiExposure` handler tests for allowed category, denied category, and unresolved/inactive `ApiCategory` policy paths.

## 3. ApiSubscription enforcement

- [ ] 3.1 Integrate `ApiCategory.allowTeams.categories` validation into `ApiSubscription` reconciliation before approval/consume-route provisioning.
- [ ] 3.2 Add/adjust `ApiSubscription` handler tests for allowed category, denied category, and unresolved/inactive `ApiCategory` policy paths.

## 4. Verification and documentation

- [ ] 4.1 Run module and/or repo make targets needed to verify build and test stability for changed components.
- [ ] 4.2 Update relevant documentation to describe runtime `ApiCategory.allowTeams.categories` enforcement for `ApiExposure` and `ApiSubscription`.
