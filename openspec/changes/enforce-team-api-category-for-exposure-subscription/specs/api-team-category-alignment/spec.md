## ADDED Requirements

### Requirement: ApiExposure enforces API category policy for provider team category
The system SHALL validate the provider team's category against the referenced `ApiCategory.allowTeams.categories` policy before provisioning an `ApiExposure`.

#### Scenario: Exposure allowed when team category is permitted
- **WHEN** an `ApiExposure` references an API whose `ApiCategory` allows the provider team category
- **THEN** category validation passes and exposure reconciliation continues

#### Scenario: Exposure blocked when team category is not permitted
- **WHEN** an `ApiExposure` references an API whose `ApiCategory` does not allow the provider team category
- **THEN** the `ApiExposure` is marked blocked with a category-policy violation reason and no downstream provisioning is performed

#### Scenario: Exposure blocked when API category policy cannot be resolved
- **WHEN** an `ApiExposure` references an API category that cannot be resolved to an active `ApiCategory` policy
- **THEN** the `ApiExposure` is marked blocked with an explicit category-policy resolution reason

### Requirement: ApiSubscription enforces API category policy for subscriber team category
The system SHALL validate the subscriber team's category against the referenced `ApiCategory.allowTeams.categories` policy before provisioning an `ApiSubscription`.

#### Scenario: Subscription allowed when team category is permitted
- **WHEN** an `ApiSubscription` references an API whose `ApiCategory` allows the subscriber team category
- **THEN** category validation passes and subscription reconciliation continues

#### Scenario: Subscription blocked when team category is not permitted
- **WHEN** an `ApiSubscription` references an API whose `ApiCategory` does not allow the subscriber team category
- **THEN** the `ApiSubscription` is marked blocked with a category-policy violation reason and no downstream provisioning is performed

#### Scenario: Subscription blocked when API category policy cannot be resolved
- **WHEN** an `ApiSubscription` references an API category that cannot be resolved to an active `ApiCategory` policy
- **THEN** the `ApiSubscription` is marked blocked with an explicit category-policy resolution reason

### Requirement: Exposure and subscription category enforcement remains semantically consistent
The system SHALL apply the same `ApiCategory.allowTeams.categories` policy semantics in both `ApiExposure` and `ApiSubscription` flows to avoid governance divergence.

#### Scenario: Same policy semantics for both resources
- **WHEN** two resources (`ApiExposure` and `ApiSubscription`) target the same API category with the same team category
- **THEN** both resources produce equivalent allow/deny outcomes under the same `ApiCategory` policy configuration
