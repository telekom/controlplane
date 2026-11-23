// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package changeset_test

import (
	"context"
	"errors"
	"flag"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/telekom/controlplane/common/pkg/changeset"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// testObject is a minimal CRD-like object for Run.
type testObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            testStatus `json:"status,omitempty"`
}

type testStatus struct {
	Result    string
	Migration *types.MigrationStatus
	Applied   *types.AppliedChanges
}

func (o *testObject) DeepCopyObject() runtime.Object { return o.DeepCopy() }

func (o *testObject) DeepCopy() *testObject {
	out := new(testObject)
	o.DeepCopyInto(out)
	return out
}

func (o *testObject) DeepCopyInto(out *testObject) {
	*out = *o
	o.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Status.Migration = o.Status.Migration.DeepCopy()
	out.Status.Applied = o.Status.Applied.DeepCopy()
}

func (o *testObject) SetMigrationStatus(status *types.MigrationStatus) { o.Status.Migration = status }

func (o *testObject) SetLastAppliedChanges(applied *types.AppliedChanges) { o.Status.Applied = applied }

type request struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Password string `json:"password,omitempty"`
}

type response struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// stubEntity is a backend entity held in memory.
type stubEntity struct {
	current *response
	writes  int
}

func (e *stubEntity) Kind() string { return "route" }

func (e *stubEntity) Get(context.Context) (*response, bool, error) {
	return e.current, e.current != nil, nil
}

func (e *stubEntity) Project(current *response) (request, error) {
	return request{Name: current.Name, Value: current.Value}, nil
}

func (e *stubEntity) Write(_ context.Context, desired *request) (*response, error) {
	e.writes++
	e.current = &response{ID: "kong-id", Name: desired.Name, Value: desired.Value}
	return e.current, nil
}

// failingEntity fails to read or to project.
type failingEntity struct {
	*stubEntity
	getErr, projectErr error
}

func (e failingEntity) Get(ctx context.Context) (*response, bool, error) {
	if e.getErr != nil {
		return nil, false, e.getErr
	}
	return e.stubEntity.Get(ctx)
}

func (e failingEntity) Project(current *response) (request, error) {
	if e.projectErr != nil {
		return request{}, e.projectErr
	}
	return e.stubEntity.Project(current)
}

// secretEntity ignores the password in comparisons and masks it in records.
type secretEntity struct{ *stubEntity }

func (e secretEntity) Comparable(desired, current request) (request, request) {
	current.Password = desired.Password
	return desired, current
}

func (e secretEntity) Masker() changeset.Masker { return changeset.MaskPathsOrDie("$.password") }

func (e secretEntity) NameOf(desired request) string { return "named-" + desired.Name }

var _ = Describe("Masking", func() {
	It("masks the values at paths on both sides, so a changed secret stays a diff", func() {
		masker := changeset.MaskPathsOrDie("$.config.*.password")
		diffs, err := changeset.Compare(
			map[string]any{"config": map[string]any{"basic": map[string]any{"password": "new", "user": "u"}}},
			map[string]any{"config": map[string]any{"basic": map[string]any{"password": "old", "user": "u"}}},
			masker,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(diffs).To(HaveLen(1))
		Expect(diffs[0].Path).To(Equal("$.config.basic.password"))
		Expect(diffs[0].Desired).To(HavePrefix("sha256:"))
		Expect(diffs[0].Desired).NotTo(Equal(diffs[0].Current))
	})

	It("masks the value of key:value entries and keeps the key", func() {
		masker := changeset.MaskEntriesOrDie("$.config.*.headers", "jumper_config")
		diffs, err := changeset.Compare(
			map[string]any{"config": map[string]any{"add": map[string]any{"headers": []any{"jumper_config:c2VjcmV0", "routing_config:abc"}}}},
			map[string]any{"config": map[string]any{"add": map[string]any{"headers": []any{"jumper_config:b2xk", "routing_config:abc"}}}},
			masker,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(diffs).To(HaveLen(1))
		Expect(diffs[0].Desired).To(HavePrefix("jumper_config:sha256:"))
		Expect(diffs[0].Desired).NotTo(ContainSubstring("c2VjcmV0"))
	})

	It("does not report a secret that did not change", func() {
		diffs, err := changeset.Compare(map[string]any{"secret": "s"}, map[string]any{"secret": "s"}, changeset.MaskPathsOrDie("$.secret"))
		Expect(err).NotTo(HaveOccurred())
		Expect(diffs).To(BeEmpty())
	})

	It("panics on an invalid secret path", func() {
		Expect(func() { changeset.MaskPathsOrDie("secret") }).To(Panic())
		Expect(func() { changeset.MaskEntriesOrDie("$..headers", "k") }).To(Panic())
	})
})

var _ = Describe("Reconcile", func() {
	var (
		gate *changeset.Gate
		plan *changeset.Plan
		ctx  context.Context
	)

	BeforeEach(func() {
		var err error
		gate, err = changeset.New(changeset.Options{Operator: "test", Mode: changeset.ModeOn, Kinds: kinds}, changeset.Config{})
		Expect(err).NotTo(HaveOccurred())
		plan = changeset.NewPlan()
		ctx = changeset.WithPlan(context.Background(), plan)
	})

	It("does not write an unchanged entity", func() {
		e := &stubEntity{current: &response{ID: "1", Name: "r", Value: "v"}}
		got, outcome, err := changeset.Reconcile(ctx, gate, e, request{Name: "r", Value: "v"})
		Expect(err).NotTo(HaveOccurred())
		Expect(outcome).To(Equal(changeset.OutcomeUnchanged))
		Expect(got.ID).To(Equal("1"))
		Expect(plan.Changes()).To(BeEmpty())
	})

	It("records an update in a plan and returns the current entity", func() {
		e := &stubEntity{current: &response{ID: "1", Name: "r", Value: "java"}}
		got, outcome, err := changeset.Reconcile(ctx, gate, e, request{Name: "r", Value: "go"})
		Expect(err).NotTo(HaveOccurred())
		Expect(outcome).To(Equal(changeset.OutcomePlanned))
		Expect(got.Value).To(Equal("java"))
		Expect(e.writes).To(BeZero())
		Expect(plan.Changes()).To(ConsistOf(HaveField("Diffs", ConsistOf(HaveField("Path", "$.value")))))
	})

	It("records a create in a plan and returns a planned entity", func() {
		e := &stubEntity{}
		got, outcome, err := changeset.Reconcile(ctx, gate, e, request{Name: "r", Value: "go"})
		Expect(err).NotTo(HaveOccurred())
		Expect(outcome).To(Equal(changeset.OutcomePlanned))
		Expect(changeset.IsPlanned(got.ID)).To(BeTrue())
		Expect(got.Value).To(Equal("go"))
		Expect(plan.Changes()).To(ConsistOf(HaveField("Operation", changeset.OperationCreate)))
	})

	It("writes while the plan is applied", func() {
		e := &stubEntity{current: &response{ID: "1", Name: "r", Value: "java"}}
		_, outcome, err := changeset.Reconcile(changeset.WithApply(context.Background()), gate, e, request{Name: "r", Value: "go"})
		Expect(err).NotTo(HaveOccurred())
		Expect(outcome).To(Equal(changeset.OutcomeWritten))
		Expect(e.writes).To(Equal(1))
	})

	It("writes directly without a gate", func() {
		e := &stubEntity{}
		_, outcome, err := changeset.Reconcile(context.Background(), nil, e, request{Name: "r"})
		Expect(err).NotTo(HaveOccurred())
		Expect(outcome).To(Equal(changeset.OutcomeWritten))
	})

	It("uses the comparison, the masker and the name of the entity", func() {
		e := secretEntity{&stubEntity{current: &response{ID: "1", Name: "r", Value: "java"}}}
		_, _, err := changeset.Reconcile(ctx, gate, e, request{Name: "r", Value: "go", Password: "top-secret"})
		Expect(err).NotTo(HaveOccurred())
		change := plan.Changes()[0]
		Expect(change.Name).To(Equal("named-r"))
		Expect(change.Diffs).To(ConsistOf(HaveField("Path", "$.value")))

		created := changeset.NewPlan()
		_, _, err = changeset.Reconcile(changeset.WithPlan(context.Background(), created), gate,
			secretEntity{&stubEntity{}}, request{Name: "r", Password: "top-secret"})
		Expect(err).NotTo(HaveOccurred())
		Expect(created.Status("h", changeset.ModeOn).Changes[0].Diffs[0].Desired).NotTo(ContainSubstring("top-secret"))
	})

	DescribeTable("writes nothing when reading the backend fails",
		func(e failingEntity, message string) {
			_, outcome, err := changeset.Reconcile(changeset.WithApply(context.Background()), gate, e, request{Name: "r", Value: "go"})
			Expect(err).To(MatchError(message))
			Expect(outcome).To(Equal(changeset.OutcomeError))
			Expect(e.writes).To(BeZero())
		},
		Entry("read", failingEntity{stubEntity: &stubEntity{}, getErr: errors.New("read failed")}, "read failed"),
		Entry("projection", failingEntity{stubEntity: &stubEntity{current: &response{ID: "1"}}, projectErr: errors.New("projection failed")}, "projection failed"),
	)

	It("deletes only while the plan is applied", func() {
		deleted := 0
		del := func(context.Context) error { deleted++; return nil }
		Expect(changeset.Delete(ctx, gate, "route", "r", del)).To(Succeed())
		Expect(deleted).To(BeZero())
		Expect(plan.Changes()).To(ConsistOf(HaveField("Operation", changeset.OperationDelete)))

		Expect(changeset.Delete(changeset.WithApply(context.Background()), gate, "route", "r", del)).To(Succeed())
		Expect(deleted).To(Equal(1))
	})
})

var _ = Describe("Run", func() {
	var (
		obj      *testObject
		entity   *stubEntity
		builds   int
		recorder *record.FakeRecorder
		ctx      context.Context
	)

	BeforeEach(func() {
		obj = &testObject{ObjectMeta: metav1.ObjectMeta{Name: "eni-echo-v1", Generation: 3}}
		entity = &stubEntity{current: &response{ID: "1", Name: "r", Value: "java"}}
		builds = 0
		recorder = record.NewFakeRecorder(10)
		ctx = contextutil.WithRecorder(context.Background(), recorder)
	})

	gate := func(mode changeset.Mode) *changeset.Gate {
		g, err := changeset.New(changeset.Options{Operator: "test", Mode: mode, Kinds: kinds}, changeset.Config{})
		Expect(err).NotTo(HaveOccurred())
		return g
	}

	build := func(value string) func(context.Context, *testObject) error {
		return func(ctx context.Context, target *testObject) error {
			builds++
			got, _, err := changeset.Reconcile(ctx, gateFrom(ctx), entity, request{Name: "r", Value: value})
			if err != nil {
				return err
			}
			target.Status.Result = got.ID
			return nil
		}
	}

	run := func(g *changeset.Gate, value string) error {
		return changeset.Run(withGate(ctx, g), g, obj, "Route", build(value))
	}

	It("builds once and writes directly without a gate", func() {
		Expect(changeset.Run(ctx, nil, obj, "Route", build("go"))).To(Succeed())
		Expect(builds).To(Equal(1))
		Expect(entity.writes).To(Equal(1))
		Expect(obj.Status.Applied).To(BeNil())
	})

	It("keeps the result of the plan and writes nothing without changes", func() {
		Expect(run(gate(changeset.ModeStrict), "java")).To(Succeed())
		Expect(builds).To(Equal(1))
		Expect(entity.writes).To(BeZero())
		Expect(obj.Status.Result).To(Equal("1"))
		Expect(obj.Status.Applied).To(BeNil())
	})

	It("blocks, shows the pending changes and writes once they are approved", func() {
		g := gate(changeset.ModeOn)
		err := run(g, "go")
		Expect(isBlocked(err)).To(BeTrue())
		Expect(entity.writes).To(BeZero())
		Expect(obj.Status.Migration).NotTo(BeNil())
		Expect(obj.Status.Result).To(BeEmpty(), "the plan result is not kept")

		hash := obj.Status.Migration.Hash
		obj.Annotations = map[string]string{changeset.ApprovalAnnotation: hash}
		Expect(run(g, "go")).To(Succeed())
		Expect(entity.writes).To(Equal(1))
		Expect(obj.Status.Migration).To(BeNil())
		Expect(obj.Status.Applied).To(SatisfyAll(
			HaveField("Hash", hash),
			HaveField("ApprovedHash", hash),
			HaveField("Decision", "approved"),
			HaveField("Mode", "on"),
			HaveField("Generation", int64(3)),
			HaveField("AppliedAt.Time", BeTemporally("~", time.Now(), time.Minute)),
			HaveField("Changes", ConsistOf(HaveField("Diffs", ConsistOf(types.Diff{Path: "$.value", Desired: `"go"`, Current: `"java"`})))),
		))
		Expect(recorder.Events).To(Receive(ContainSubstring("ChangesApplied")))
	})

	It("records the applied changes in mode off", func() {
		Expect(run(gate(changeset.ModeOff), "go")).To(Succeed())
		Expect(builds).To(Equal(2))
		Expect(entity.writes).To(Equal(1))
		Expect(obj.Status.Applied).To(SatisfyAll(HaveField("Decision", "allowed"), HaveField("ApprovedHash", "")))
	})

	It("records a created entity by the hash of its body", func() {
		entity.current = nil
		Expect(run(gate(changeset.ModeOff), "go")).To(Succeed())
		desired := obj.Status.Applied.Changes[0].Diffs[0].Desired
		Expect(desired).To(HavePrefix(`"sha256:`))
		Expect(desired).NotTo(ContainSubstring("go"))
	})

	It("returns a planning error without writing", func() {
		failing := func(context.Context, *testObject) error { return errors.New("backend down") }
		err := changeset.Run(ctx, gate(changeset.ModeOn), obj, "Route", failing)
		Expect(err).To(MatchError(And(ContainSubstring("planning Route"), ContainSubstring("backend down"))))
	})
})

var _ = Describe("ApprovalChangedPredicate", func() {
	p := changeset.ApprovalChangedPredicate()

	object := func(annotations map[string]string) *testObject {
		return &testObject{ObjectMeta: metav1.ObjectMeta{Name: "o", Annotations: annotations}}
	}

	DescribeTable("admits only a changed approval",
		func(old, updated map[string]string, expected bool) {
			Expect(p.Update(event.UpdateEvent{ObjectOld: object(old), ObjectNew: object(updated)})).To(Equal(expected))
		},
		Entry("approval set", nil, map[string]string{changeset.ApprovalAnnotation: "2"}, true),
		Entry("approval changed", map[string]string{changeset.ApprovalAnnotation: "2"}, map[string]string{changeset.ApprovalAnnotation: "3"}, true),
		Entry("approval removed", map[string]string{changeset.ApprovalAnnotation: "2"}, nil, true),
		Entry("other annotation changed", map[string]string{"a": "1"}, map[string]string{"a": "2"}, false),
	)

	It("ignores creates and deletes", func() {
		Expect(p.Create(event.CreateEvent{Object: object(nil)})).To(BeFalse())
		Expect(p.Delete(event.DeleteEvent{Object: object(nil)})).To(BeFalse())
	})
})

var _ = Describe("Flags", func() {
	parse := func(args ...string) (*changeset.Gate, error) {
		f := &changeset.Flags{}
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		f.Bind(fs)
		Expect(fs.Parse(args)).To(Succeed())
		return f.Gate("test", kinds)
	}

	It("defaults to mode off", func() {
		g, err := parse()
		Expect(err).NotTo(HaveOccurred())
		Expect(g.Mode()).To(Equal(changeset.ModeOff))
	})

	It("rejects an unknown mode", func() {
		_, err := parse("--migration-mode=true")
		Expect(err).To(MatchError(ContainSubstring("invalid migration mode")))
	})

	It("rejects a missing ignore list", func() {
		_, err := parse("--migration-mode=on", "--migration-ignore-config=/does/not/exist")
		Expect(err).To(MatchError(ContainSubstring("reading migration config")))
	})
})

type gateKey struct{}

// withGate and gateFrom pass the gate to the build function, as the Kong client of the
// gateway operator holds it.
func withGate(ctx context.Context, g *changeset.Gate) context.Context {
	return context.WithValue(ctx, gateKey{}, g)
}

func gateFrom(ctx context.Context) *changeset.Gate {
	if g, ok := ctx.Value(gateKey{}).(*changeset.Gate); ok {
		return g
	}
	return nil
}
