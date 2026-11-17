---
id: 13-environment-isolation
description: Raw-client and common-server store reads and writes outside the ScopedClient must stay within one environment.
severity: CRITICAL
complexity: 1
scope: file
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

Review all detected calls in the file together. Each check reports one finding
per file per pass; after a fix, rerun the audit to expose any remaining calls.
Findings outside the affected change window remain advisory.
Discovery and check triggers are independent of context argument names and
argument layout, and include `Status().Update` and `Status().Patch` writes.

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

### Store reads in REST servers

REST servers built on common-server (e.g. discovery-server, rover-server) do
not use the manager client. They read a `store.ObjectStore`, an in-memory
cache keyed `<namespace>/<name>`, where every namespace starts with
`<environment>--`. The label options above do not exist there; a store call is
environment-scoped when one of these holds:

- a `Get` whose namespace is built from the authenticated environment —
  `mapper.ParseApplicationId` and `mapper.ParseResourceId` prepend
  `BusinessContext.Environment`, as does `bCtx.Environment + "--" + ...`
- a `Get` by a namespace/name reference read from an object already loaded in
  the same environment (e.g. `in.Spec.Requestor.Application`, `in.Spec.Zone`)
- a `List` whose `ListOpts.Prefix` is set by `store.EnforcePrefix` from
  `security.PrefixFromContext(ctx)` or to `<environment>--`, or replaced by a
  namespace derived as above followed by `/`
- a `List` with an exact-match `store.Filter` pinning a namespace field to a
  namespace derived as above
- a generic helper (e.g. `pagination.FetchAll`) that forwards caller-supplied
  `ListOpts` without changing `Prefix` or `Filters`; scoping is then judged
  at its callers

A store `List` with an empty `Prefix` and no such filter, or a `Get` whose
namespace comes from user input without the environment, crosses environments.

## Detect

```regex
(\b[A-Za-z_]\w*(?:\.[A-Za-z_]\w*)*(?:\.Status\(\))?\.(List|Get|Create|Update|Patch|Delete)\s*\(|mgr\.GetClient\(\))
```

## Checks

- [unscoped-list] when=`\.List\s*\(` Does the marked `List` read without environment scoping — on a raw client no environment label selector, `NewScopedClient` wrapper or namespace restriction; on a common-server store no environment-bound `ListOpts.Prefix` or namespace filter, nor forwarding of caller-supplied `ListOpts`?
- [undocumented-namespace] when=`InNamespace` Does the marked call rely on `client.InNamespace` for isolation without a nearby comment stating that the namespace isolates the environment?
- [unscoped-get] when=`\.Get\s*\(` Does the marked `Get` use a key derived neither from the authenticated environment nor from an object in the same environment (e.g. a fixed or user-supplied namespace)?
- [unlabelled-write] when=`\.(Create|Update|Patch)\s*\(` Does the marked raw-client write either create or modify an object without preserving the source environment label, or write a status subresource without verifying the target belongs to that environment?
- [unscoped-reader] when=`mgr\.GetClient\(\)` Is the raw manager client handed to a webhook or helper that then reads without environment scoping?
- [unscoped-delete] when=`\.Delete\s*\(` Does the marked deletion use a raw client without verifying that the target belongs to the source environment or wrapping the client in a `NewScopedClient`?

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

```go synthetic="every store list in the repository sets a prefix or filter; this is the cross-environment store read being prevented"
	listOpts := store.NewListOpts()
	res, err := c.stores.EventTypeStore.List(ctx, listOpts)
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

```go source=discovery-server/internal/controller/application.go:58@79aab2db
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return api.ApplicationResponse{}, err
	}

	app, err := c.stores.ApplicationStore.Get(ctx, appInfo.Namespace, appInfo.AppName)
```

```go source=discovery-server/internal/mapper/apisubscription/out.go:156@79aab2db
	appRef := in.Spec.Requestor.Application
	app, err := appStore.Get(ctx, appRef.Namespace, appRef.Name)
```

```go source=discovery-server/internal/controller/application.go:34@79aab2db
	listOpts := store.NewListOpts()
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	items, err := pagination.FetchAll(ctx, c.stores.ApplicationStore, listOpts)
```

```go source=rover-server/internal/controller/apispecification.go:123@79aab2db
	listOpts := store.NewListOpts()
	listOpts.Cursor = params.Cursor
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	objList, err := a.Store.List(ctx, listOpts)
```
