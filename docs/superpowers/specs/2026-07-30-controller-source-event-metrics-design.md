# Controller Source Event Metrics

## Problem

We cannot tell which watch is generating controller load. `For()`, `Owns()` and
every `Watches()` feed the same workqueue, so all built-in metrics are
aggregated per controller with no source attribution. When a controller is hot,
there is no way to know whether the cause is its own CRs, an owned resource
churning, or a mapfunc watch fanning out.

## What controller-runtime already provides

Registered automatically by `sigs.k8s.io/controller-runtime` v0.24.1:

| Metric | Labels | Meaning |
| --- | --- | --- |
| `workqueue_adds_total` | `name` | Items added to the queue: post-predicate, post-mapfunc, deduplicated by key |
| `workqueue_depth`, `workqueue_queue_duration_seconds`, `workqueue_retries_total` | `name` | Queue pressure and backlog |
| `controller_runtime_reconcile_total` | `controller`, `result` | Reconciles executed |
| `controller_runtime_reconcile_time_seconds` | `controller` | Reconcile duration |
| `controller_runtime_active_workers`, `controller_runtime_max_concurrent_reconciles` | `controller` | Saturation |

None carry a source dimension. Neither `pkg/source` nor `pkg/handler` exposes a
metric hook. Per-source attribution does not exist upstream and must be built.

## Hook selection

Three extension points were evaluated.

**Predicates (chosen).** Invoked once per event per source, before
deduplication, at `pkg/internal/source/event_handler.go:77,110,158`. Accepted by
`For()`, `Owns()` and `Watches()` alike. Uniform coverage of all three roles.

**Event handler decorator (rejected).** `handler.TypedEventHandler` is exported,
is a required positional argument to `Watches()`, and receives the workqueue —
so it could count both events in and requests out, giving a fan-out
amplification factor. Rejected because `Owns()` accepts only `OwnsOption` and
constructs `EnqueueRequestForOwner` internally
(`pkg/builder/controller.go:123`). Covering `Owns` would require mixing hooks,
producing two different counting semantics (pre-filter vs post-filter) in one
metric.

**Builder wrapper (rejected).** A wrapper around `ctrl.NewControllerManagedBy`
injecting the predicate automatically would make the metric impossible to
forget. Rejected because it requires shadowing the entire fluent `builder.Builder`
API (`For`, `Owns`, `Watches`, `WatchesRawSource`, `WithOptions`,
`WithEventFilter`, `Named`, `Complete`, `Build`) as passthroughs returning the
wrapper type. That API has churned across c-r 0.19 to 0.24; owning a shadow of
it for a metric is a poor trade.

## Design

New file `common/pkg/controller/metrics.go`, alongside the existing
`predicates.go`.

```go
const (
    RoleFor     = "for"
    RoleOwns    = "owns"
    RoleWatches = "watches"
)

// Results record what the source's predicates did with an observed event.
const (
    ResultPassed   = "passed"   // admitted by this source's predicates
    ResultFiltered = "filtered" // rejected by an inner predicate
)

var eventsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
    Name: "controlplane_controller_source_events_total",
    Help: "Events observed per controller source, labelled by whether the source's predicates admitted the event.",
}, []string{"controller", "role", "source", "verb", "result"})

func init() { metrics.Registry.MustRegister(eventsTotal) }

// Count returns a predicate that delegates the filtering decision to inner and
// records every event it observes, labelled with that decision. Counting happens
// after inner runs: the event is recorded either way, with the filtering outcome
// in the result label (ResultPassed or ResultFiltered).
func Count(controller, role string, inner ...predicate.Predicate) predicate.Predicate
```

Registered against `sigs.k8s.io/controller-runtime/pkg/metrics.Registry`, so it
is served by the existing metrics endpoint with no wiring per service.

### Labels

`controller` — explicit string argument. The predicate sees only the watched
object and cannot derive the primary type, so this cannot be inferred. It must
match controller-runtime's own derivation, `strings.ToLower(gvk.Kind)`
(`pkg/builder/controller.go:387`), to remain joinable with
`workqueue_adds_total{name=...}` and `controller_runtime_reconcile_total{controller=...}`.
This means `"rover"`, not `"rover-controller"` as used by
`mgr.GetEventRecorderFor`. The doc comment must call out this trap.

`role` — `for`, `owns` or `watches`. Explicit, for the same reason as
`controller`.

`source` — derived per event by reflection: `reflect.TypeOf(obj).Elem().Name()`,
yielding `"Team"`. `obj.GetObjectKind().GroupVersionKind()` is unusable because
`TypeMeta` is empty on typed objects delivered by informers.
`apiutil.GVKForObject` would give the full group-qualified GVK but requires a
scheme and an error path; the repo has no duplicate Kind names across groups, so
the Kind alone is unambiguous.

`verb` — `create`, `update`, `delete`, `generic`. Update events increment once
despite carrying two objects.

`result` — `passed` or `filtered`, recording what the source's own predicates did
with the event. Without it the metric is misleading: a watch guarded by
`GenerationChangedPredicate` would report every status-only update as load, when
the controller discards them. `result="passed"` is the reconcile-driving traffic;
`result="filtered"` is traffic the informer and predicates still pay for but the
workqueue never sees. Summing over `result` gives raw informer delivery.

Cardinality: roughly 40 controllers and 42 distinct watched Kinds, times 4 verbs,
times 2 results — low hundreds of series.

### Wrapping rather than composing

`builder.WithPredicates` ANDs its predicates and short-circuits on the first
`false`. A bare counter listed after a filtering predicate would miss every
event that predicate rejects. `rover_controller.go:95` is exactly this case, a
`Watches` guarded by `predicate.GenerationChangedPredicate{}`.

`Count` therefore takes the inner predicates as arguments rather than sitting
beside them, which makes the ordering impossible to get wrong. It evaluates
`inner`, records the outcome in the `result` label, and returns that same
decision, leaving filtering behaviour unchanged. With no inner predicates every
event passes.

### Semantics

Counting happens after the source's own predicates decide and before queue
deduplication, with the decision recorded in `result`. Real reconcile-driving
load per source is `result="passed"`. The gap between that and
`workqueue_adds_total` is queue deduplication alone; the `filtered` series is
what the predicates absorbed.

Useful queries:

```promql
# Which watch actually drives reconciles
sum by (controller, role, source) (
  rate(controlplane_controller_source_events_total{result="passed"}[5m]))

# Predicate effectiveness per source: how much noise is being absorbed
sum by (source) (rate(controlplane_controller_source_events_total{result="filtered"}[5m]))
  / sum by (source) (rate(controlplane_controller_source_events_total[5m]))
```

## Usage

```go
b := ctrl.NewControllerManagedBy(mgr).
    For(&rover.Rover{}, builder.WithPredicates(cc.Count("rover", cc.RoleFor))).
    Owns(&apiapi.ApiSubscription{}, builder.WithPredicates(cc.Count("rover", cc.RoleOwns)))

b = b.Watches(&organizationv1.Team{},
    handler.EnqueueRequestsFromMapFunc(r.MapTeamToRovers),
    builder.WithPredicates(cc.Count("rover", cc.RoleWatches, predicate.GenerationChangedPredicate{})),
)
```

Applied to every `For` across all ~40 controllers, so every controller reports
its primary source. Including `For` is partly redundant with
`workqueue_adds_total`, but it makes the primary stream directly comparable to
the non-primary ones.

`Owns`/`Watches` coverage is scoped: instrumented in `rover` (rover_controller
only), `gateway` (route, consumeroute), `pubsub` (subscriber only), `application`
and `api`. Roughly 47 `Owns()`/`Watches()` registrations elsewhere are not yet
instrumented: event (20), agentic (13), organization (3), rover `*Specification`
controllers (3), notification (2), identity (2), admin (2), pubsub publisher (1),
approval (1). Completing them is a planned follow-up.

Until then the roles do **not** sum to the controller total for those modules. An
absent `role="owns"`/`role="watches"` series means "not yet instrumented", not
"no traffic" — do not read a missing series as an idle watch.

## Rollout

1. Add `common/pkg/controller/metrics.go` and its test.
2. Apply to controllers with `Owns` or `Watches` first, since those carry the
   attribution question: `rover`, `pubsub/subscriber`, `gateway/route`.
3. Apply `RoleFor` to the remaining controllers.

`github.com/prometheus/client_golang` is already in `common/go.mod` as an
indirect dependency and is promoted to direct. No new module.

## Testing

One test file, `common/pkg/controller/metrics_test.go`, following the existing
Ginkgo suite in the package. It fires all four event types through a `Count`
wrapping a rejecting predicate and asserts, by gathering from
controller-runtime's own `metrics.Registry` via `metrics.Registry.Gather()`, that
counters increment for filtered events too and that the return value still
reflects the inner predicate. Gathering from the real registry rather than a
`testutil` helper also proves the metric is actually registered where the metrics
endpoint will serve it.
A second case asserts the `source` label is derived correctly from the object
type.

## Out of scope

Event-to-request fan-out amplification, which would need the handler hook that
`Owns()` cannot accept. Revisit if a mapfunc watch proves to be the dominant
load and the fan-out factor is needed to size it.

Automatic injection via a builder wrapper. Revisit if watches added without the
predicate become a recurring miss in review.
