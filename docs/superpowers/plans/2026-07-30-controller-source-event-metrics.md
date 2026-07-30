# Controller Source Event Metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Emit a Prometheus counter attributing every controller event to the watch that produced it, labelled by controller, role (`for`/`owns`/`watches`), source Kind and verb.

**Architecture:** A single predicate constructor, `Count`, lives in the shared `common` module and wraps any inner predicates. Predicates are the only controller-runtime extension point invoked uniformly for `For()`, `Owns()` and `Watches()`. It counts every event it observes, then delegates filtering to the inner predicates unchanged. It registers against controller-runtime's own metrics registry, so every service exposes it with no per-service wiring.

**Tech Stack:** Go, controller-runtime v0.24.1, prometheus/client_golang v1.23.2, Ginkgo v2 + Gomega for tests.

Design spec: `docs/superpowers/specs/2026-07-30-controller-source-event-metrics-design.md`

## Global Constraints

- Every new `.go` file starts with the repo's SPDX header, exactly:
  ```go
  // Copyright 2025 Deutsche Telekom IT GmbH
  //
  // SPDX-License-Identifier: Apache-2.0
  ```
- Metric name is exactly `controlplane_controller_source_events_total`.
- Label order in `NewCounterVec` and every `WithLabelValues` call is exactly `controller`, `role`, `source`, `verb`, `result`.
- `result` values are exactly `passed` and `filtered`, exposed as constants `ResultPassed` and `ResultFiltered`. Counting happens *after* the inner predicates decide, so a watch guarded by `GenerationChangedPredicate` does not report discarded events as load.
- The `controller` label value is the primary Kind lowercased (`"rover"`, `"route"`), **never** the `GetEventRecorderFor` style (`"rover-controller"`). This matches controller-runtime's own derivation at `pkg/builder/controller.go:387` and is required for the metric to join against `workqueue_adds_total{name=...}`.
- `verb` values are exactly `create`, `update`, `delete`, `generic`.
- The `common` module is consumed by the service modules via `replace ... => ../common` directives already present in each service `go.mod`. No version bump or publish step is needed.
- Tests in `common/pkg/controller` run under the existing Ginkgo suite (`suite_test.go`), which boots envtest. Do not add a second `TestMain` or `RunSpecs`.
- Pre-commit hooks: the `reuse-lint-file` hook fails on `COMMIT_EDITMSG` in a git worktree. If a commit fails with `is not inside of '.'`, re-run it with `SKIP=reuse-lint-file` prefixed. Do not "fix" the file's licence header in response to that error.

---

### Task 1: The `Count` predicate in `common`

**Files:**
- Create: `common/pkg/controller/metrics.go`
- Create: `common/pkg/controller/metrics_test.go`
- Modify: `common/go.mod` (promote `github.com/prometheus/client_golang` from indirect to direct)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `controller.Count(controller, role string, inner ...predicate.Predicate) predicate.Predicate`
  - `controller.RoleFor`, `controller.RoleOwns`, `controller.RoleWatches` — untyped string constants `"for"`, `"owns"`, `"watches"`.
  - Package `github.com/telekom/controlplane/common/pkg/controller`, conventionally imported as `cc` in service controllers.

- [ ] **Step 1: Write the failing test**

Create `common/pkg/controller/metrics_test.go`:

```go
// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	cc "github.com/telekom/controlplane/common/pkg/controller"
)

// rejectAll is a predicate that filters out every event.
type rejectAll struct{}

func (rejectAll) Create(event.CreateEvent) bool   { return false }
func (rejectAll) Delete(event.DeleteEvent) bool   { return false }
func (rejectAll) Update(event.UpdateEvent) bool   { return false }
func (rejectAll) Generic(event.GenericEvent) bool { return false }

var _ = Describe("Count predicate", func() {

	// readCounter gathers the metric from the controller-runtime registry and
	// returns the value for the given label set, or 0 if the series is absent.
	readCounter := func(controller, role, source, verb string) float64 {
		families, err := metrics.Registry.Gather()
		Expect(err).ToNot(HaveOccurred())
		for _, f := range families {
			if f.GetName() != "controlplane_controller_source_events_total" {
				continue
			}
			for _, m := range f.GetMetric() {
				want := map[string]string{
					"controller": controller,
					"role":       role,
					"source":     source,
					"verb":       verb,
				}
				match := true
				for _, l := range m.GetLabel() {
					if want[l.GetName()] != l.GetValue() {
						match = false
						break
					}
				}
				if match {
					return m.GetCounter().GetValue()
				}
			}
		}
		return 0
	}

	obj := &corev1.ConfigMap{}

	It("counts each verb once and passes events through with no inner predicate", func() {
		p := cc.Count("testctl", cc.RoleWatches)

		before := map[string]float64{}
		for _, verb := range []string{"create", "update", "delete", "generic"} {
			before[verb] = readCounter("testctl", cc.RoleWatches, "ConfigMap", verb)
		}

		Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
		Expect(p.Update(event.UpdateEvent{ObjectOld: obj, ObjectNew: obj})).To(BeTrue())
		Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
		Expect(p.Generic(event.GenericEvent{Object: obj})).To(BeTrue())

		for _, verb := range []string{"create", "update", "delete", "generic"} {
			Expect(readCounter("testctl", cc.RoleWatches, "ConfigMap", verb)).
				To(Equal(before[verb]+1), "verb %s", verb)
		}
	})

	It("counts events that the inner predicate rejects, and still rejects them", func() {
		p := cc.Count("filterctl", cc.RoleOwns, rejectAll{})

		before := readCounter("filterctl", cc.RoleOwns, "ConfigMap", "create")

		Expect(p.Create(event.CreateEvent{Object: obj})).To(BeFalse())

		Expect(readCounter("filterctl", cc.RoleOwns, "ConfigMap", "create")).
			To(Equal(before + 1))
	})

	It("derives the source label from the object type", func() {
		p := cc.Count("srcctl", cc.RoleFor)

		before := readCounter("srcctl", cc.RoleFor, "Secret", "create")
		Expect(p.Create(event.CreateEvent{Object: &corev1.Secret{}})).To(BeTrue())
		Expect(readCounter("srcctl", cc.RoleFor, "Secret", "create")).To(Equal(before + 1))
	})

	It("ANDs multiple inner predicates", func() {
		p := cc.Count("andctl", cc.RoleWatches, predicate.NewPredicateFuncs(func(client.Object) bool {
			return true
		}), rejectAll{})

		Expect(p.Create(event.CreateEvent{Object: obj})).To(BeFalse())
	})

	It("tolerates a nil object without panicking", func() {
		p := cc.Count("nilctl", cc.RoleWatches)
		Expect(func() { p.Create(event.CreateEvent{}) }).ToNot(Panic())
	})
})
```

The `testutil` import is used only by `readCounter`'s sibling assertions; if your
linter flags it as unused, drop it — `metrics.Registry.Gather()` is the only
gathering call this file needs.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd common && go test ./pkg/controller/ -run TestControllers 2>&1 | head -30`

Expected: compile failure, `undefined: cc.Count`, `undefined: cc.RoleWatches`.

- [ ] **Step 3: Write the implementation**

Create `common/pkg/controller/metrics.go`:

```go
// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Roles describe which builder call a watch originates from.
const (
	RoleFor     = "for"
	RoleOwns    = "owns"
	RoleWatches = "watches"
)

var eventsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "controlplane_controller_source_events_total",
	Help: "Events observed per controller source, before predicate filtering and queue deduplication.",
}, []string{"controller", "role", "source", "verb"})

func init() {
	metrics.Registry.MustRegister(eventsTotal)
}

var _ predicate.Predicate = &countingPredicate{}

// Count returns a predicate that counts every event it observes and then
// delegates the filtering decision to inner. With no inner predicates it always
// returns true.
//
// Counting happens before inner runs, so events that inner rejects are still
// counted. Always pass filtering predicates to Count rather than listing them
// alongside it in builder.WithPredicates: that call ANDs its predicates and
// short-circuits on the first false, which would silently drop counts.
//
// The controller argument must be the primary Kind lowercased, e.g. "rover" and
// not "rover-controller". That is how controller-runtime labels its own metrics,
// and matching it is what allows this metric to be joined against
// workqueue_adds_total and controller_runtime_reconcile_total.
func Count(controller, role string, inner ...predicate.Predicate) predicate.Predicate {
	return &countingPredicate{
		controller: controller,
		role:       role,
		inner:      inner,
	}
}

// countingPredicate holds no per-event state, so one instance may be shared
// across several watches of the same controller and role.
type countingPredicate struct {
	controller string
	role       string
	inner      []predicate.Predicate
}

// observe records one event of the given verb for the object's type.
func (p *countingPredicate) observe(verb string, obj client.Object) {
	eventsTotal.WithLabelValues(p.controller, p.role, sourceOf(obj), verb).Inc()
}

// sourceOf returns the Kind of obj, e.g. "ConfigMap".
//
// It uses reflection rather than obj.GetObjectKind(): TypeMeta is empty on typed
// objects delivered by an informer, so the GVK there is blank. apiutil.GVKForObject
// would give the group-qualified GVK but needs a scheme and an error path, and no
// two watched Kinds in this repo share a name across groups.
func sourceOf(obj client.Object) string {
	if obj == nil {
		return "unknown"
	}
	t := reflect.TypeOf(obj)
	if t == nil {
		return "unknown"
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Name() == "" {
		return "unknown"
	}
	return t.Name()
}

func (p *countingPredicate) Create(e event.CreateEvent) bool {
	p.observe("create", e.Object)
	for _, i := range p.inner {
		if !i.Create(e) {
			return false
		}
	}
	return true
}

func (p *countingPredicate) Update(e event.UpdateEvent) bool {
	p.observe("update", e.ObjectNew)
	for _, i := range p.inner {
		if !i.Update(e) {
			return false
		}
	}
	return true
}

func (p *countingPredicate) Delete(e event.DeleteEvent) bool {
	p.observe("delete", e.Object)
	for _, i := range p.inner {
		if !i.Delete(e) {
			return false
		}
	}
	return true
}

func (p *countingPredicate) Generic(e event.GenericEvent) bool {
	p.observe("generic", e.Object)
	for _, i := range p.inner {
		if !i.Generic(e) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd common && go mod tidy && go test ./pkg/controller/ 2>&1 | tail -20`

Expected: PASS. `go mod tidy` moves `github.com/prometheus/client_golang` out of the indirect block.

- [ ] **Step 5: Verify the metric name and labels are exactly as specified**

Run: `cd common && grep -n "controlplane_controller_source_events_total" pkg/controller/metrics.go`

Expected: one match, in the `CounterOpts`. Confirm the label slice reads `[]string{"controller", "role", "source", "verb"}` in that order.

- [ ] **Step 6: Commit**

```bash
git add common/pkg/controller/metrics.go common/pkg/controller/metrics_test.go common/go.mod common/go.sum
SKIP=reuse-lint-file git commit -m "feat(common): add Count predicate for per-source controller event metrics"
```

---

### Task 2: Instrument the controllers that have `Owns` or `Watches`

These three carry the attribution question, so they land first and prove the metric end to end.

**Files:**
- Modify: `rover/internal/controller/rover_controller.go:74-96`
- Modify: `gateway/internal/controller/route_controller.go:49-53`
- Modify: `pubsub/internal/controller/subscriber_controller.go:50-56`

**Interfaces:**
- Consumes: `cc.Count`, `cc.RoleFor`, `cc.RoleOwns`, `cc.RoleWatches` from Task 1.
- Produces: nothing consumed by later tasks.

All three files already import the `common/pkg/controller` package aliased as `cc` (it is the source of `cc.NewController` and `cc.NewRateLimiter`). No new import is needed. `builder` is already imported in `rover` and `gateway`; `pubsub/subscriber` imports it too.

- [ ] **Step 1: Instrument `route_controller.go`**

Replace lines 49-53:

```go
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.Route{}).
		Watches(&gatewayv1.ConsumeRoute{},
			handler.EnqueueRequestsFromMapFunc(r.mapConsumeRouteToRoute),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
```

with:

```go
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.Route{}, builder.WithPredicates(cc.Count("route", cc.RoleFor))).
		Watches(&gatewayv1.ConsumeRoute{},
			handler.EnqueueRequestsFromMapFunc(r.mapConsumeRouteToRoute),
			builder.WithPredicates(cc.Count("route", cc.RoleWatches, predicate.GenerationChangedPredicate{}))).
```

Note the `GenerationChangedPredicate` moved *inside* `Count` rather than sitting beside it.

- [ ] **Step 2: Build and commit gateway**

Run: `cd gateway && go build ./... && go vet ./internal/controller/`
Expected: no output.

```bash
git add gateway/internal/controller/route_controller.go gateway/go.mod gateway/go.sum
SKIP=reuse-lint-file git commit -m "feat(gateway): count route controller source events"
```

- [ ] **Step 3: Instrument `subscriber_controller.go`**

Replace lines 50-56:

```go
	return ctrl.NewControllerManagedBy(mgr).
		For(&pubsubv1.Subscriber{}).
		Watches(&pubsubv1.Publisher{},
			handler.EnqueueRequestsFromMapFunc(r.MapPublisherToSubscriber),
			builder.WithPredicates(),
		).
```

with:

```go
	return ctrl.NewControllerManagedBy(mgr).
		For(&pubsubv1.Subscriber{}, builder.WithPredicates(cc.Count("subscriber", cc.RoleFor))).
		Watches(&pubsubv1.Publisher{},
			handler.EnqueueRequestsFromMapFunc(r.MapPublisherToSubscriber),
			builder.WithPredicates(cc.Count("subscriber", cc.RoleWatches)),
		).
```

The existing `builder.WithPredicates()` here is empty, so there is no inner predicate to carry over.

- [ ] **Step 4: Build and commit pubsub**

Run: `cd pubsub && go build ./... && go vet ./internal/controller/`
Expected: no output.

```bash
git add pubsub/internal/controller/subscriber_controller.go pubsub/go.mod pubsub/go.sum
SKIP=reuse-lint-file git commit -m "feat(pubsub): count subscriber controller source events"
```

- [ ] **Step 5: Instrument `rover_controller.go`**

This one builds conditionally across lines 73-96. Replace:

```go
	b := ctrl.NewControllerManagedBy(mgr).
		For(&rover.Rover{}).
		Owns(&apiapi.ApiSubscription{}).
		Owns(&apiapi.ApiExposure{}).
		Owns(&application.Application{})

	if cconfig.FeaturePubSub.IsEnabled() {
		b = b.Owns(&eventv1.EventExposure{}).
			Owns(&eventv1.EventSubscription{})
	}

	if cconfig.FeaturePermission.IsEnabled() {
		b = b.Owns(&permissionv1.PermissionSet{})
	}

	if cconfig.FeatureAiGateway.IsEnabled() {
		b = b.Owns(&agenticv1.AgenticExposure{}).
			Owns(&agenticv1.AgenticSubscription{})
	}

	b = b.Watches(&organizationv1.Team{},
		handler.EnqueueRequestsFromMapFunc(r.MapTeamToRovers),
		builder.WithPredicates(predicate.GenerationChangedPredicate{}),
	)
```

with:

```go
	owns := builder.WithPredicates(cc.Count("rover", cc.RoleOwns))

	b := ctrl.NewControllerManagedBy(mgr).
		For(&rover.Rover{}, builder.WithPredicates(cc.Count("rover", cc.RoleFor))).
		Owns(&apiapi.ApiSubscription{}, owns).
		Owns(&apiapi.ApiExposure{}, owns).
		Owns(&application.Application{}, owns)

	if cconfig.FeaturePubSub.IsEnabled() {
		b = b.Owns(&eventv1.EventExposure{}, owns).
			Owns(&eventv1.EventSubscription{}, owns)
	}

	if cconfig.FeaturePermission.IsEnabled() {
		b = b.Owns(&permissionv1.PermissionSet{}, owns)
	}

	if cconfig.FeatureAiGateway.IsEnabled() {
		b = b.Owns(&agenticv1.AgenticExposure{}, owns).
			Owns(&agenticv1.AgenticSubscription{}, owns)
	}

	b = b.Watches(&organizationv1.Team{},
		handler.EnqueueRequestsFromMapFunc(r.MapTeamToRovers),
		builder.WithPredicates(cc.Count("rover", cc.RoleWatches, predicate.GenerationChangedPredicate{})),
	)
```

Reusing a single `owns` option across all eight `Owns` calls is safe: the predicate holds no per-watch state, and the `source` label is derived from each event's own object.

- [ ] **Step 6: Build and commit rover**

Run: `cd rover && go build ./... && go vet ./internal/controller/`
Expected: no output.

```bash
git add rover/internal/controller/rover_controller.go rover/go.mod rover/go.sum
SKIP=reuse-lint-file git commit -m "feat(rover): count rover controller source events"
```

- [ ] **Step 7: Verify the metric appears at runtime**

Run the rover controller's existing envtest suite, which starts a manager and a metrics server:

Run: `cd rover && make test 2>&1 | tail -20`
Expected: PASS. This confirms `MustRegister` does not panic on duplicate registration when several controllers register the same collector — it is a package-level `init`, so it runs once.

---

### Task 3: Instrument the remaining controllers with `RoleFor`

Thirty-eight controllers have only a `For()` call. Each gets one `builder.WithPredicates(cc.Count("<kind>", cc.RoleFor))` argument. This is mechanical; the value is that the roles then sum to the controller total.

**Files:** Modify the `For(` line in each of:

```
admin/internal/controller/environment_controller.go:49          environment
admin/internal/controller/remoteorganization_controller.go:49   remoteorganization
admin/internal/controller/zone_controller.go:62                 zone
agentic/internal/controller/agenticexposure_controller.go:60    agenticexposure
agentic/internal/controller/agenticsubscription_controller.go:64 agenticsubscription
agentic/internal/controller/mcpserver_controller.go:51          mcpserver
api/internal/controller/api_controller.go:51                    api
api/internal/controller/apicategory_controller.go:46            apicategory
api/internal/controller/apiexposure_controller.go:61            apiexposure
api/internal/controller/apisubscription_controller.go:68        apisubscription
api/internal/controller/remoteapisubscription_controller.go:54  remoteapisubscription
application/internal/controller/application_controller.go:79    application
approval/internal/controller/approval_controller.go:51          approval
approval/internal/controller/approvalexpiration_controller.go:50 approvalexpiration
approval/internal/controller/approvalrequest_controller.go:48   approvalrequest
event/internal/controller/eventconfig_controller.go:61          eventconfig
event/internal/controller/eventexposure_controller.go:65        eventexposure
event/internal/controller/eventsubscription_controller.go:64    eventsubscription
event/internal/controller/eventtype_controller.go:46            eventtype
gateway/internal/controller/consumer_controller.go:46           consumer
gateway/internal/controller/consumeroute_controller.go:50       consumeroute
gateway/internal/controller/gateway_controller.go:46            gateway
identity/internal/controller/client_controller.go:61            client
identity/internal/controller/identityprovider_controller.go:48  identityprovider
identity/internal/controller/realm_controller.go:61             realm
notification/internal/controller/notification_controller.go:90  notification
notification/internal/controller/notificationchannel_controller.go:50 notificationchannel
notification/internal/controller/notificationtemplate_controller.go:52 notificationtemplate
organization/internal/controller/group_controller.go:45         group
organization/internal/controller/team_controller.go:62          team
permission/internal/controller/permissionset_controller.go:52   permissionset
pubsub/internal/controller/eventstore_controller.go:46          eventstore
pubsub/internal/controller/publisher_controller.go:51           publisher
rover/internal/controller/apichangelog_controller.go:45         apichangelog
rover/internal/controller/apispecification_controller.go:53     apispecification
rover/internal/controller/eventspecification_controller.go:50   eventspecification
rover/internal/controller/mcpspecification_controller.go:47     mcpspecification
rover/internal/controller/roadmap_controller.go:48              roadmap
```

The second column is the exact `controller` label value — the Kind lowercased, matching controller-runtime's `strings.ToLower(gvk.Kind)`.

**Interfaces:**
- Consumes: `cc.Count`, `cc.RoleFor` from Task 1.
- Produces: nothing.

- [ ] **Step 1: Apply the edit, one module at a time**

For a controller with a bare `For`, e.g. `admin/internal/controller/zone_controller.go:62`:

```go
		For(&adminv1.Zone{}).
```

becomes:

```go
		For(&adminv1.Zone{}, builder.WithPredicates(cc.Count("zone", cc.RoleFor))).
```

For the one controller that already has a predicate, `api/internal/controller/apisubscription_controller.go:68`:

```go
		For(&apiapi.ApiSubscription{}, builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
```

becomes:

```go
		For(&apiapi.ApiSubscription{}, builder.WithPredicates(cc.Count("apisubscription", cc.RoleFor, predicate.ResourceVersionChangedPredicate{}))).
```

Most of these files do not yet import `builder`. Add to the import block:

```go
	"sigs.k8s.io/controller-runtime/pkg/builder"
```

Every one of these files already imports the common controller package as `cc`.

- [ ] **Step 2: Build each module**

Run, for each of `admin agentic api application approval event gateway identity notification organization permission pubsub rover`:

```bash
cd <module> && go build ./... && go vet ./internal/controller/
```

Expected: no output.

- [ ] **Step 3: Verify no controller was missed**

Run:

```bash
grep -rn "\.For(&\|\.Owns(&\|\.Watches(&" --include="*.go" */internal/controller/*.go \
  | grep -v _test | grep -v "cc.Count"
```

Expected: no output. Any line printed is an uninstrumented watch.

- [ ] **Step 4: Verify label values match the Kind**

Run:

```bash
grep -rn "cc.Count(" --include="*.go" */internal/controller/*.go | grep -i "controller\"" 
```

Expected: no output. A match means someone used the `"zone-controller"` recorder-style name, which breaks the join with the built-in metrics.

- [ ] **Step 5: Commit, one commit per module**

```bash
git add <module>/internal/controller/
SKIP=reuse-lint-file git commit -m "feat(<module>): count controller source events"
```

---

### Task 4: Document the metric

**Files:**
- Modify: `common/README.md`

**Interfaces:**
- Consumes: `cc.Count` from Task 1.
- Produces: nothing.

- [ ] **Step 1: Append a section to `common/README.md`**

```markdown
## Controller source event metrics

`controlplane_controller_source_events_total{controller,role,source,verb,result}`
counts every event delivered to a controller, attributed to the watch that
produced it. `role` is `for`, `owns` or `watches`; `source` is the Kind of the
watched object; `verb` is `create`, `update`, `delete` or `generic`; `result` is
`passed` or `filtered` depending on whether that watch's own predicates admitted
the event.

Wire it into a watch with `controller.Count`, passing any filtering predicates as
trailing arguments rather than listing them alongside it:

    For(&gatewayv1.Route{}, builder.WithPredicates(cc.Count("route", cc.RoleFor))).
    Watches(&gatewayv1.ConsumeRoute{},
        handler.EnqueueRequestsFromMapFunc(r.mapConsumeRouteToRoute),
        builder.WithPredicates(cc.Count("route", cc.RoleWatches, predicate.GenerationChangedPredicate{})))

Passing predicates to `Count` rather than beside it is what makes `result`
meaningful: `Count` sees the filtering decision and records it, instead of
counting events the controller then discards.

The `controller` argument must be the primary Kind lowercased ("route", not
"route-controller"), matching how controller-runtime labels its own metrics.

Query `result="passed"` for the traffic that actually drives reconciles, and the
`filtered` share to see how much noise a watch's predicates are absorbing.
```

- [ ] **Step 2: Commit**

```bash
git add common/README.md
SKIP=reuse-lint-file git commit -m "docs(common): document controller source event metrics"
```

---

## Self-Review

**Spec coverage:** hook choice (Task 1), all four labels with the stated derivations (Task 1), wrapping rather than composing (Task 1 impl + Task 2/3 call sites), pre-filter semantics (Task 1 test asserts rejected events are still counted), the three-stage rollout order (Tasks 2 and 3 mirror the spec's rollout section), the `client_golang` promotion (Task 1 Step 4), the test plan (Task 1 Step 1). Out-of-scope items are correctly absent.

**Type consistency:** `Count(controller, role string, inner ...predicate.Predicate) predicate.Predicate` and the three `Role*` constants are used identically in Tasks 1, 2, 3 and 4.
