---
id: 12-webhook-validation-prefer-cel
description: Admission webhooks must not check spec constraints that a CEL XValidation rule or schema marker can express.
severity: HIGH
complexity: 1
scope: file
applies_to:
  - "**/internal/webhook/**/*.go"
exclude:
  - "**/*_test.go"
  - "**/suite_test.go"
context_lines: 80
---

# Prefer CEL to webhook validation

A constraint in the CRD schema is enforced by the API server on every write, is
visible in `kubectl explain` and in the generated reference, and needs no running
webhook. A constraint in Go code is invisible to users and agents until a write
is rejected. So whenever the CRD schema can express a constraint, it belongs
there: an ordinary marker (`Enum`, `MinItems`, `MaxItems`, `Pattern`,
`+listType=map`) for a single field, a `+kubebuilder:validation:XValidation`
rule for the rest.

CEL can express a constraint when it reads only the object being admitted:

- its own spec fields, including several fields together
- the previous version of the object (`oldSelf`), so immutability and allowed
  transitions
- fixed values such as reserved names

Webhook validation is the right place for everything CEL cannot see: other
objects read through a client (`Get`, `List`), uniqueness across the cluster,
external systems, and the admission request itself, such as the user or the
operation. Logging, defaulting and wiring code are not validation.

## Detect

```regex
field\.(Invalid|Forbidden|Required|Duplicate|NotSupported|TooMany|TooLong)\(|\.Add\w*Error\(|\bold\w*\.Spec\b|return (nil, )?(fmt\.Errorf|errors\.New|apierrors\.NewInvalid)\(
```

## Checks

- [cel-expressible] Does a marked line reject the object based only on the object's own fields, or on old versus new values of them, in a way an `XValidation` rule or schema marker could express?

## Examples

### Violation

```go source=admin/internal/webhook/v1/environment_webhook.go:53@522cdd64
	// RealmName is immutable once set — changing it after resources have been
	// provisioned in the realm (gateway routes, identity clients, etc.) would
	// leave orphaned objects in the old realm that controllers cannot clean up.
	if oldEnvironment.Spec.RealmName != "" && oldEnvironment.Spec.RealmName != newEnvironment.Spec.RealmName {
		return nil, fmt.Errorf("spec.realmName is immutable once set (was %q)", oldEnvironment.Spec.RealmName)
	}
```

Immutability once set reads only the old and new object, which a transition
rule over `oldSelf` expresses.

```go source=admin/internal/webhook/v1/zone_validator.go:79@522cdd64
	if zone.Spec.ManagedRoutes != nil {
		for i, route := range zone.Spec.ManagedRoutes.Routes {
			if route.Name == adminv1.ZoneHealthRouteName {
				errs = append(errs, field.Forbidden(specPath.Child("managedRoutes", "routes").Index(i).Child("name"),
					"zone-health is reserved for the platform health probe"))
			}
		}
	}
```

A reserved name is a fixed value checked against the object's own list, which
an `XValidation` rule on the route name expresses.

### Allowed

```go source=admin/internal/webhook/v1/environment_webhook.go:73@522cdd64
func (v *EnvironmentCustomValidator) validateRealmNameUniqueness(ctx context.Context, environment *adminv1.Environment) error {
	realmName := environment.GetRealmName()

	var envList adminv1.EnvironmentList
	if err := v.Client.List(ctx, &envList,
		client.MatchingFields{IndexFieldRealmName: realmName},
	); err != nil {
		return fmt.Errorf("listing environments: %w", err)
	}

	for i := range envList.Items {
		other := &envList.Items[i]
		if other.UID == environment.UID {
			continue
		}
		return fmt.Errorf("realm name %q is already used by Environment %s/%s", realmName, other.Namespace, other.Name)
	}

	return nil
}
```

The same field, in the same webhook, but uniqueness across Environments needs a
`List` over the cluster, which CEL cannot do.

```go source=rover/internal/webhook/v1/rover_webhook.go:685@522cdd64
func (r *RoverValidator) GetZone(ctx context.Context, valErr *cerrors.ValidationError, zoneRef client.ObjectKey) (*adminv1.Zone, error) {
	zone := &adminv1.Zone{}
	err := r.client.Get(ctx, zoneRef, zone)
	if err != nil {
		if apierrors.IsNotFound(err) {
			valErr.AddInvalidError(
				field.NewPath("spec").Child("zone"),
				zoneRef.Name,
				fmt.Sprintf("zone %q not found", zoneRef.Name),
			)
			return nil, nil
		}
		return nil, apierrors.NewInternalError(err)
	}
	return zone, nil
}
```

Rejects `spec.zone`, but only after a `Get` shows the referenced Zone is missing.
