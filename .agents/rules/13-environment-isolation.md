---
id: 13-environment-isolation
description: Raw-client reads and writes outside the ScopedClient must stay within one environment.
severity: CRITICAL
complexity: 1
scope: line
applies_to:
  - "**/internal/**/*.go"
exclude:
  - "**/*_test.go"
  - "**/suite_test.go"
  - "**/testutil/**"
context_lines: 30
---

# Environment isolation

Environments (tenants) share one cluster and are isolated only virtually:
every CustomResource carries `cconfig.EnvironmentLabelKey`, and every read and
write must be restricted to a single environment. A list that leaks across
environments enqueues, reads or mutates another tenant's objects — nothing
fails, the data is just wrong, so this is caught at review time.

`cclient.NewScopedClient` enforces the label automatically. Code that uses the
raw manager client instead — `r.List`, `r.Get`, `r.Client.*`,
`mgr.GetClient()`, typically in `Watches` map functions and webhooks — must
scope itself, in one of these ways:

- `client.MatchingLabels{cconfig.EnvironmentLabelKey: <source>.Labels[cconfig.EnvironmentLabelKey]}`
  (or an equivalent `env` list option) on the `List`
- wrapping the raw client with `cclient.NewScopedClient(r.Client, <env>)`
- a `Get` by an exact key whose namespace or name is derived from an object
  already in the same environment
- `client.InNamespace(<source>.Namespace)`, because every environment owns its
  own namespace — **only** when a comment at the call says namespace isolation
  is intended

A genuinely cluster-wide read (e.g. listing `Environment` objects themselves)
is allowed when a comment at the call explains why it is not
environment-scoped. Undocumented exceptions are violations.

## Detect

```regex
(\b(r|v)\.((Client|client)\.)?(List|Get|Create|Update|Patch|Delete)\(ctx|mgr\.GetClient\(\))
```

## Checks

- [unscoped-list] when=`\.List\(ctx` Does the marked `List` run on the raw client without an environment label selector, a `NewScopedClient` wrapper, or a namespace restriction?
- [undocumented-namespace] when=`InNamespace` Does the marked call rely on `client.InNamespace` for isolation without a nearby comment stating that the namespace isolates the environment?
- [unscoped-get] when=`\.Get\(ctx` Does the marked `Get` use a key that is not derived from an object in the same environment (e.g. a fixed or user-supplied namespace)?
- [unlabelled-write] when=`\.(Create|Update|Patch)\(ctx` Does the marked write through the raw client create or modify an object without setting the environment label from the source object?
- [unscoped-reader] when=`mgr\.GetClient\(\)` Is the raw manager client handed to a webhook or helper that then reads without environment scoping?
- [unscoped-delete] when=`\.Delete\(ctx` Does the marked deletion use a raw client without verifying that the target belongs to the source environment or wrapping the client in a `NewScopedClient`?

## Examples

### Violation

```go synthetic="no unscoped raw-client list exists in the repository; this is the cross-environment leak being prevented"
func (r *ClientReconciler) mapRealmObjToIdentityClient(ctx context.Context, obj client.Object) []reconcile.Request {
	list := &identityv1.ClientList{}
	// Lists Clients of every environment that reference a realm with this name.
	if err := r.List(ctx, list, client.MatchingFields{IndexFieldSpecRealm: obj.GetName()}); err != nil {
		return nil
	}
```

```go synthetic="same reason; shows the undocumented namespace variant"
	var publishers pubsubv1.PublisherList
	err := r.List(ctx, &publishers, client.InNamespace(eventStore.Namespace))
```

```go synthetic="a raw-client deletion must verify the target environment before deleting"
	if err := r.Delete(ctx, target); err != nil {
		return err
	}
```

### Allowed

```go source=identity/internal/controller/client_controller.go:87@4fbb53d2
	listOpts := []client.ListOption{
		client.MatchingFields{
			IndexFieldSpecRealm: types.ObjectRefFromObject(realm).String(),
		},
		client.MatchingLabels{
			cconfig.EnvironmentLabelKey: realm.Labels[cconfig.EnvironmentLabelKey],
		},
```

```go source=file/internal/controller/filetype_controller.go:77@4fbb53d2
	ctx = cclient.WithClient(ctx, cclient.NewJanitorClient(cclient.NewScopedClient(r.Client, exposure.Labels[cconfig.EnvironmentLabelKey])))
```
