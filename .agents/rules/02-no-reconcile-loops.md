---
id: 02-no-reconcile-loops
description: Reconcilers must not trigger themselves or each other in a cycle.
severity: CRITICAL
tier: 1
scope: file
applies_to:
  - "**/internal/controller/**/*.go"
  - "**/internal/handler/**/*.go"
  - "**/pkg/controller/**/*.go"
exclude:
  - "**/*_test.go"
  - "**/suite_test.go"
context_lines: 60
---

# No reconcile loops

Operators watch the resources they own. Any write to a watched resource
re-enters the queue, so a write that is not guarded by a convergence condition
turns reconciliation into a busy loop: constant API server load, rate limiting
and a cluster that never settles.

The detector narrows the audit to changed reconciler and handler files that
actually write to the API server or mutate status. Judge the change in that
file only — the checks below are the full set of questions.

A write is safe when repeating it with the same input produces no further
event: the value is unchanged, the write is guarded by `ObservedGeneration`,
or the data is derived deterministically from the spec.

## Detect

The rule-level detector must stay at least as broad as the union of the
`when=` patterns below, or a check could never be asked.

```regex
(Status\(\)\.(Update|Patch)|\.(Update|Patch|Create)\(ctx|SetCondition|ObservedGeneration|RequeueAfter|\.List\(|\.Items\b)
```

## Checks

Each check declares the signal that makes it worth asking. A check whose
pattern is absent from the candidate window is answered `PASS` by the engine,
with the reason recorded — asking a model to confirm that a string is missing
is pure cost.

- [self-trigger] when=`(Status\(\)\.(Update|Patch)|\.(Update|Patch|Create)\(ctx)` Does this change write to a resource the same controller watches, without a guard that makes the write stop once the desired state is reached?
- [observed-generation] when=`(SetCondition|SetStatusCondition|ObservedGeneration)` If a status condition is written, is `ObservedGeneration` stamped so an unchanged generation does not re-trigger work?
- [unstable-ordering] when=`(\.List\(|\.Items\b)` Is any collection derived from a `List` call stored in a spec or status without being sorted first, so cached ordering can flip between reconciles?
- [fixed-requeue] when=`(RequeueAfter|Requeue:)` Does the change requeue on a fixed interval instead of a jittered one?
- [cross-resource-cycle] when=`(Status\(\)\.(Update|Patch)|\.(Update|Patch|Create)\(ctx)` Does the change introduce a write chain across resources (A updates B, B updates A) that can cycle?

## Examples

### Violation

```go synthetic="a committed reconcile loop would be a production incident, so no instance exists to cite; this is the shape being prevented"
// Writes status on every pass: the watch fires again, forever.
obj.Status.LastSyncedAt = metav1.Now()
if err := r.Status().Update(ctx, obj); err != nil {
    return ctrl.Result{}, err
}
```

```go synthetic="same reason; shows the unstable-ordering variant"
// List order from the cache is not stable, so the stored slice keeps changing.
obj.Status.Zones = zonesFrom(zoneList.Items)
return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, obj)
```

### Allowed

```go source=identity/internal/handler/client/handler.go:123@b53a2d9c
// Guarded: once the generation is observed, the work is skipped entirely.
cond := meta.FindStatusCondition(client.GetConditions(), identityv1.SecretRotationConditionType)
return cond != nil && cond.ObservedGeneration == client.Generation
```

```go source=projector/internal/runtime/reconciler.go:180@b53a2d9c
// Jittered requeue spreads resync load instead of synchronising it.
jitter := 0.8 + r.randFloat64()*0.4
return ctrl.Result{RequeueAfter: time.Duration(float64(resync) * jitter)}
```
