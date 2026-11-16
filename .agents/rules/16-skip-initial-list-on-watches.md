---
id: 16-skip-initial-list-on-watches
description: Watches whose map function lists from the cache to enqueue For objects must skip the informer's initial-list creates.
severity: HIGH
complexity: 1
scope: file
applies_to:
  - "**/internal/controller/**/*.go"
exclude:
  - "**/*_test.go"
  - "**/suite_test.go"
context_lines: 60
---

# Skip the startup replay on Watches

When a cache is first filled, the informer emits a synthetic `Create` event for
every existing object. Since controller-runtime v0.23 a source only counts as
synced once all of these events have passed through its predicates and event
handler, and that must finish within `CacheSyncTimeout`. A
`handler.EnqueueRequestsFromMapFunc` whose map function runs `r.List(...)` with
a label selector scans the whole cache per event, so startup costs
O(watched objects × primary objects). With enough objects the operator fails
with `timed out waiting for cache to be synced` and crash-loops.

That startup work is also redundant: the `For(...)` source already enqueues
every primary object from its own initial list. A `Watches(...)` whose map
function only enqueues objects of the controller's `For` type therefore gains
nothing from the replay and must add `cc.SkipInitialListPredicate{}` as an inner
predicate of `cc.Count(...)`, so skipped events are still counted as filtered.

The predicate is wrong on a `For(...)` source (the primaries would never be
reconciled at startup) and on a watch whose map function enqueues anything that
is not the controller's `For` type. `Owns(...)` does not run a map function and
is out of scope.

## Detect

```regex
(EnqueueRequestsFromMapFunc|SkipInitialListPredicate)
```

## Checks

- [missing-skip] when=`EnqueueRequestsFromMapFunc` Does a touched `Watches(...)` with `EnqueueRequestsFromMapFunc`, whose map function lists objects from the cache and only enqueues objects of the controller's `For` type, lack `SkipInitialListPredicate` among its predicates?
- [wrong-source] when=`SkipInitialListPredicate` Is `SkipInitialListPredicate` applied to a `For(...)` source, or to a watch whose map function enqueues objects that are not of the controller's `For` type?
- [outside-count] when=`SkipInitialListPredicate` Is `SkipInitialListPredicate` passed to `builder.WithPredicates` directly instead of as an inner predicate of `cc.Count(...)`?

## Examples

### Violation

```go source=api/internal/controller/apisubscription_controller.go:73@d82c1226
		Watches(&apiapi.Api{},
			handler.EnqueueRequestsFromMapFunc(r.MapApiToApiSubscription),
			builder.WithPredicates(cc.Count("apisubscription", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{})),
		).
```

### Allowed

```go source=api/internal/controller/apisubscription_controller.go:73@0b70b62d
		Watches(&apiapi.Api{},
			handler.EnqueueRequestsFromMapFunc(r.MapApiToApiSubscription),
			builder.WithPredicates(cc.Count("apisubscription", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{}, cc.SkipInitialListPredicate{})),
		).
```

```go source=gateway/internal/controller/route_controller.go:60@d82c1226
		Watches(&gatewayv1.ConsumeRoute{},
			handler.EnqueueRequestsFromMapFunc(r.mapConsumeRouteToRoute),
			builder.WithPredicates(cc.Count("route", cc.RoleWatches, predicate.GenerationChangedPredicate{}, cc.SkipInitialListPredicate{}))).
```
