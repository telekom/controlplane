// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"errors"
	"reflect"

	"github.com/emirpasic/gods/sets/hashset"

	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/gateway/pkg/kong/migration"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type (
	stubRequest  struct{ Value string }
	stubResponse struct{ Value string }
)

// stubEntity is a minimal entity used to exercise the reconcile driver on its own.
type stubEntity struct {
	current   *stubResponse
	found     bool
	getErr    error
	projected *stubRequest
	projErr   error
	writes    *int
	compare   func(desired, current stubRequest) (stubRequest, stubRequest)
}

func (e *stubEntity) Name() string { return "route" }

func (e *stubEntity) Get(context.Context) (*stubResponse, bool, error) {
	return e.current, e.found, e.getErr
}

func (e *stubEntity) Project(current *stubResponse) (stubRequest, error) {
	if e.projErr != nil {
		return stubRequest{}, e.projErr
	}
	if e.projected != nil {
		return *e.projected, nil
	}
	return stubRequest{Value: current.Value}, nil
}

func (e *stubEntity) Write(_ context.Context, desired *stubRequest) (*stubResponse, error) {
	*e.writes++
	return &stubResponse{Value: desired.Value}, nil
}

// comparingStubEntity adds the optional comparer capability.
type comparingStubEntity struct{ *stubEntity }

func (e comparingStubEntity) Comparable(desired, current stubRequest) (stubRequest, stubRequest) {
	return e.compare(desired, current)
}

var _ = Describe("reconcile", func() {
	var writes int

	BeforeEach(func() { writes = 0 })

	It("skips the write when the projected state matches", func() {
		got, changed, err := reconcile(context.Background(), nil,
			&stubEntity{current: &stubResponse{Value: "same"}, found: true, writes: &writes},
			stubRequest{Value: "same"})

		Expect(err).NotTo(HaveOccurred())
		Expect(got.Value).To(Equal("same"))
		Expect(changed).To(BeFalse())
		Expect(writes).To(BeZero())
	})

	It("writes when the entity is absent", func() {
		_, changed, err := reconcile(context.Background(), nil,
			&stubEntity{found: false, projErr: errors.New("projection must not run"), writes: &writes},
			stubRequest{Value: "desired"})

		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		Expect(writes).To(Equal(1))
	})

	It("writes when the projected state differs", func() {
		_, changed, err := reconcile(context.Background(), nil,
			&stubEntity{current: &stubResponse{Value: "old"}, found: true, writes: &writes},
			stubRequest{Value: "desired"})

		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		Expect(writes).To(Equal(1))
	})

	It("does not write when reading fails", func() {
		readErr := errors.New("read failed")

		_, _, err := reconcile(context.Background(), nil,
			&stubEntity{getErr: readErr, writes: &writes}, stubRequest{})

		Expect(err).To(MatchError(readErr))
		Expect(writes).To(BeZero())
	})

	It("does not write when projecting fails", func() {
		projErr := errors.New("projection failed")

		_, _, err := reconcile(context.Background(), nil,
			&stubEntity{current: &stubResponse{}, found: true, projErr: projErr, writes: &writes},
			stubRequest{})

		Expect(err).To(MatchError(projErr))
		Expect(writes).To(BeZero())
	})

	It("uses the entity comparison when the entity provides one", func() {
		entity := comparingStubEntity{&stubEntity{
			current: &stubResponse{Value: "old"}, found: true, writes: &writes,
			compare: func(_, current stubRequest) (stubRequest, stubRequest) { return current, current },
		}}

		_, changed, err := reconcile(context.Background(), nil, entity, stubRequest{Value: "desired"})

		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())
		Expect(writes).To(BeZero())
	})
})

func isBlockedErr(err error) bool {
	var blocked ctrlerrors.BlockedError
	return errors.As(err, &blocked) && blocked.IsBlocked()
}

var _ = Describe("reconcile in migration mode", func() {
	var (
		writes int
		plan   *migration.Plan
		ctx    context.Context
	)

	BeforeEach(func() {
		writes = 0
		plan = migration.NewPlan()
		ctx = migration.WithPlan(context.Background(), plan)
	})

	guard := func() *migration.Guard {
		g, err := migration.New(migration.Config{}, migration.ModeOn)
		Expect(err).NotTo(HaveOccurred())
		return g
	}

	It("records an update and returns the current entity without writing", func() {
		got, changed, err := reconcile(ctx, guard(),
			&stubEntity{current: &stubResponse{Value: "java"}, found: true, writes: &writes},
			stubRequest{Value: "go"})

		Expect(err).NotTo(HaveOccurred())
		Expect(got.Value).To(Equal("java"))
		Expect(changed).To(BeFalse())
		Expect(writes).To(BeZero())
		Expect(plan.Changes()).To(ConsistOf(HaveField("Operation", "update")))
	})

	It("records a create and returns a planned entity", func() {
		got, _, err := reconcile(ctx, guard(), &stubEntity{found: false, writes: &writes}, stubRequest{Value: "go"})

		Expect(err).NotTo(HaveOccurred())
		Expect(got.Value).To(Equal("go"))
		Expect(writes).To(BeZero())
		Expect(plan.Changes()).To(ConsistOf(HaveField("Operation", "create")))
	})

	It("writes while an allowed plan is applied", func() {
		_, changed, err := reconcile(migration.WithApply(context.Background()), guard(),
			&stubEntity{current: &stubResponse{Value: "java"}, found: true, writes: &writes},
			stubRequest{Value: "go"})

		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		Expect(writes).To(Equal(1))
	})

	It("does not record a diff the entity comparison excludes", func() {
		entity := comparingStubEntity{&stubEntity{
			current: &stubResponse{Value: "kong-default"}, found: true, writes: &writes,
			compare: func(desired, _ stubRequest) (stubRequest, stubRequest) { return desired, desired },
		}}

		_, _, err := reconcile(ctx, guard(), entity, stubRequest{Value: "go"})

		Expect(err).NotTo(HaveOccurred())
		Expect(plan.Changes()).To(BeEmpty())
	})

	It("blocks an unexpected diff outside a plan", func() {
		_, _, err := reconcile(context.Background(), guard(),
			&stubEntity{current: &stubResponse{Value: "java"}, found: true, writes: &writes},
			stubRequest{Value: "go"})

		Expect(isBlockedErr(err)).To(BeTrue())
		Expect(writes).To(BeZero())
	})
})

var _ = Describe("normalizeSet", func() {
	It("sorts and removes duplicates without mutating the input", func() {
		input := []string{"beta", "alpha", "beta"}

		normalized := normalizeSet(&input)

		Expect(*normalized).To(Equal([]string{"alpha", "beta"}))
		Expect(input).To(Equal([]string{"beta", "alpha", "beta"}))
	})

	It("normalizes nil and empty slices to nil", func() {
		empty := []string{}

		Expect(normalizeSet[string](nil)).To(BeNil())
		Expect(normalizeSet(&empty)).To(BeNil())
	})
})

var _ = Describe("plugin config normalization", func() {
	It("normalizes equivalent JSON numbers identically", func() {
		integer, err := normalizeConfig(map[string]any{"limit": 1})
		Expect(err).NotTo(HaveOccurred())

		decimal, err := normalizeConfig(map[string]any{"limit": float64(1)})
		Expect(err).NotTo(HaveOccurred())

		Expect(reflect.DeepEqual(integer, decimal)).To(BeTrue())
	})

	It("sorts set-like string arrays so that ordering cannot look like a change", func() {
		desired, err := normalizeConfig(map[string]any{
			"allow": []string{"gamma", "alpha", "beta"},
		})
		Expect(err).NotTo(HaveOccurred())

		current := map[string]any{"allow": []any{"beta", "gamma", "alpha"}}
		sortConfigSets(current)

		Expect(reflect.DeepEqual(desired, current)).To(BeTrue())
	})

	It("sorts arrays nested inside the configuration", func() {
		desired, err := normalizeConfig(map[string]any{
			"append": map[string]any{"headers": []string{"b:2", "a:1"}},
		})
		Expect(err).NotTo(HaveOccurred())

		current := map[string]any{"append": map[string]any{"headers": []any{"a:1", "b:2"}}}
		sortConfigSets(current)

		Expect(reflect.DeepEqual(desired, current)).To(BeTrue())
	})

	It("leaves arrays that are not all strings untouched", func() {
		config := map[string]any{"statuses": []any{float64(3), float64(1), float64(2)}}

		sortConfigSets(config)

		Expect(config["statuses"]).To(Equal([]any{float64(3), float64(1), float64(2)}))
	})

	It("sorts values produced by set types, whose iteration order is randomized", func() {
		// An ACL allow list is built from a hash set, which marshals in Go map
		// iteration order. Without sorting, the desired config would differ
		// from the one read back from Kong on nearly every reconciliation.
		config := map[string]any{
			"allow": hashset.New("gamma", "alpha", "epsilon", "beta", "delta"),
		}

		normalized, err := normalizeConfig(config)

		Expect(err).NotTo(HaveOccurred())
		Expect(normalized["allow"]).To(Equal([]any{"alpha", "beta", "delta", "epsilon", "gamma"}))
	})

	It("normalizes a nil configuration to nil", func() {
		normalized, err := normalizeConfig(nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(normalized).To(BeNil())
	})

	It("copies the configuration it normalizes", func() {
		config := map[string]any{"allow": []any{"beta", "alpha"}}

		_, err := normalizeConfig(config)

		Expect(err).NotTo(HaveOccurred())
		Expect(config["allow"]).To(Equal([]any{"beta", "alpha"}))
	})
})

var _ = Describe("narrowToDesired", func() {
	It("drops the keys Kong fills in from its defaults", func() {
		desired := map[string]any{"allow": []any{"group-a"}}
		current := map[string]any{"allow": []any{"group-a"}, "deny": nil, "hide_groups_header": false}

		Expect(narrowToDesired(desired, current)).To(Equal(desired))
	})

	It("drops defaults nested inside a managed group", func() {
		desired := map[string]any{"limit": map[string]any{"second": float64(5)}}
		current := map[string]any{"limit": map[string]any{"second": float64(5), "minute": nil}}

		Expect(narrowToDesired(desired, current)).To(Equal(desired))
	})

	It("keeps a differing value so that it still counts as a change", func() {
		desired := map[string]any{"allow": []any{"group-a"}}
		current := map[string]any{"allow": []any{"group-b"}, "deny": nil}

		Expect(narrowToDesired(desired, current)).To(Equal(map[string]any{"allow": []any{"group-b"}}))
	})

	It("leaves out a desired key Kong does not report", func() {
		narrowed := narrowToDesired(map[string]any{"allow": []any{"group-a"}}, map[string]any{})

		Expect(narrowed).To(BeEmpty())
		Expect(reflect.DeepEqual(map[string]any{"allow": []any{"group-a"}}, narrowed)).To(BeFalse())
	})

	It("narrows a nil desired configuration to nil", func() {
		Expect(narrowToDesired(nil, map[string]any{"deny": nil})).To(BeNil())
	})

	It("does not modify the configuration it narrows", func() {
		current := map[string]any{"allow": []any{"group-a"}, "deny": nil}

		narrowToDesired(map[string]any{"allow": []any{"group-a"}}, current)

		Expect(current).To(HaveKey("deny"))
	})
})

var _ = Describe("summarizeBody", func() {
	It("keeps the description Kong gives for a rejected request", func() {
		Expect(summarizeBody([]byte(`{"name":"schema violation","message":"2 schema violations"}`))).
			To(Equal("schema violation: 2 schema violations"))
	})

	It("drops the echoed request, which carries the credentials of an auth plugin", func() {
		body := []byte(`{"name":"schema violation","message":"config.secret: required",` +
			`"fields":{"config":{"secret":"s3cr3t-value","key":"client-key"}}}`)

		summary := summarizeBody(body)

		Expect(summary).NotTo(ContainSubstring("s3cr3t-value"))
		Expect(summary).NotTo(ContainSubstring("client-key"))
		Expect(summary).To(ContainSubstring("config.secret: required"))
	})

	It("falls back to whichever description Kong gives", func() {
		Expect(summarizeBody([]byte(`{"message":"no route matched"}`))).To(Equal("no route matched"))
		Expect(summarizeBody([]byte(`{"name":"unique violation"}`))).To(Equal("unique violation"))
		Expect(summarizeBody([]byte(`{}`))).To(Equal("<no description>"))
	})

	It("reports a body it cannot read without echoing it", func() {
		Expect(summarizeBody([]byte(`<html>s3cr3t</html>`))).To(Equal("<unreadable body>"))
		Expect(summarizeBody(nil)).To(Equal("<unreadable body>"))
	})
})
