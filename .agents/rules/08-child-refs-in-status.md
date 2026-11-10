---
id: 08-child-refs-in-status
description: Every sub-resource a handler creates must be referenced in its parent's status.
severity: HIGH
tier: 1
scope: file
applies_to:
  - "**/internal/handler/**/*.go"
  - "**/pkg/handler/**/*.go"
exclude:
  - "**/*_test.go"
  - "**/suite_test.go"
context_lines: 80
---

# Sub-resources belong in the parent's status

A handler that creates a child resource is the only thing that knows the child
exists. The parent's status is where that knowledge is written down, and it is
load-bearing twice over:

- **Cleanup.** `Delete` deletes what the status lists. A handler that instead
  re-derives the child's name reimplements the naming scheme a second time, and
  the day the two disagree — a renamed zone, a changed hash, a spec field that
  moved into the name — the child is orphaned with no record that it ever
  existed.
- **Observability.** The UI, the janitor and anyone debugging a tenant start at
  the parent and follow its status downwards. A child missing from the status is
  invisible: nothing fails, it simply cannot be found.

So the contract is: if the handler called `CreateOrUpdate` on a child, a
reference to that child is in the parent's status before the status update, and
it is cleared when the child goes away, so that `Delete` can read it back
instead of rebuilding the child's name.

Use `types.ObjectRefFromObject` for a single child and a sorted
`[]types.ObjectRef` for a collection — unsorted lists from a `List` call cause
reconcile churn (see `01-prefer-slices-sort` and the Reconciliation Flow section
of `AGENTS.md`).

The tell for a violation is a helper that creates a child and returns only
`error`, or a caller that discards the created object with `_, err :=`. Neither
leaves anywhere for the reference to come from.

Resources that are not children do not count: reading a `Zone`, patching an
object this handler does not own, or writing to an external system through an
API client are all outside the rule.

## Detect

```regex
\.CreateOrUpdate\(ctx,
```

## Checks

- [status-ref] Is a child resource created or updated here without a reference to it being recorded in the parent's status?

## Examples

### Violation

```go source=file/internal/handler/zoneserviceconfig/handler.go:205@b53a2d9c
	mutator := func() error {
		locErr := controllerutil.SetControllerReference(obj, apiClient, cc.Scheme())
		if locErr != nil {
			return fmt.Errorf("failed to set controller reference for apiClient: %w", locErr)
		}

		apiClient.Labels = util.DomainLabel()

		apiClient.Spec.ClientId = apiClient.Name
		apiClient.Spec.ClientSecret = ref
		apiClient.Spec.Realm = zone.Status.InternalIdentityRealm

		return nil
	}

	if _, err := cc.CreateOrUpdate(ctx, apiClient, mutator); err != nil {
		return fmt.Errorf("failed to create or update identity Client %q: %w", apiClient.Name, err)
	}

	return nil
```

The owning `ZoneServiceConfig` controls this identity `Client`, but
`ZoneServiceConfigStatus` carries only `SFTPServiceConfigRef`. The function
returns `error`, so the object is gone the moment it is created.

```go source=event/internal/handler/eventconfig/handler.go:390@b53a2d9c
	if _, err := c.CreateOrUpdate(ctx, consumer, mutator); err != nil {
		return errors.Wrapf(err, "failed to create or update gateway Consumer %s", clientId)
	}
	return nil
}
```

`EventConfigStatus` records eight other children — stores, clients, routes — and
not this consumer. A status that is almost complete is the harder bug: a reader
reasonably assumes the list is the whole inventory.

### Allowed

```go source=api/internal/handler/remoteapisubscription/provider_scenario.go:96@b53a2d9c
	_, err = c.CreateOrUpdate(ctx, application, mutator)
	if err != nil {
		return errors.Wrapf(err, "failed to create application %s", application.Name)
	}
	obj.Status.Application = types.ObjectRefFromObject(application)
```

The same `_, err =` discard of the operation result as the violations — the
difference is the next line. The result of `CreateOrUpdate` is genuinely
uninteresting; the object is not.

```go source=organization/internal/handler/team/handler/gateway_consumer/gateway_consumer.go:58@b53a2d9c
	if _, err = k8sClient.CreateOrUpdate(ctx, gatewayConsumerObj, mutate); err != nil {
		return err
	}

	owner.Status.GatewayConsumerRef = types.ObjectRefFromObject(gatewayConsumerObj)
	return nil
}

func (g GatewayConsumerHandler) Delete(ctx context.Context, owner *organisationv1.Team) error {
	var err error
	k8sClient := cclient.ClientFromContextOrDie(ctx)
	if owner.Status.GatewayConsumerRef != nil {
		err = k8sClient.Delete(ctx, &gatewayv1.Consumer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      owner.Status.GatewayConsumerRef.GetName(),
				Namespace: owner.Status.GatewayConsumerRef.GetNamespace(),
```

The full loop: the reference is written on create and is what `Delete` reads
back. No naming scheme is reimplemented.

```go source=application/internal/handler/application/handler.go:569@b53a2d9c
		if _, err := c.CreateOrUpdate(ctx, consumer, mutator); err != nil {
			return errors.Wrapf(err, "failed to create or update Gateway Consumer %s", resourceName)
		}

		owner.Status.Consumers = append(owner.Status.Consumers, *types.ObjectRefFromObject(consumer))
	}
	slices.SortFunc(owner.Status.Consumers, func(a, b types.ObjectRef) int {
```

A collection of children, appended as it is created and sorted before the status
update so the list order is stable across reconciles.
