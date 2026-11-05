// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rollback_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/rollback"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
)

func planStrings(ops []rollback.Operation) []string {
	out := make([]string, len(ops))
	for i, op := range ops {
		out[i] = op.String()
	}
	return out
}

func ptr[T any](v T) *T { return &v }

var _ = Describe("Plan", func() {
	var desired *snapshot.State
	var current *snapshot.State

	BeforeEach(func() {
		desired = desiredSnapshot().State
		current = desiredSnapshot().State
	})

	It("creates everything when the route is absent", func() {
		Expect(planStrings(rollback.Plan(desired, &snapshot.State{}, false))).To(Equal([]string{
			"upsert upstream up-1",
			"upsert target target-1",
			"upsert service svc-1",
			"upsert route route-1",
			"upsert plugin rate-limiting plugin-2",
			"upsert plugin acl plugin-1",
		}))
	})

	It("treats a nil current state as absent", func() {
		Expect(rollback.Plan(desired, nil, false)).To(HaveLen(6))
	})

	It("returns no operations when the state is equal", func() {
		Expect(rollback.Plan(desired, current, true)).To(BeEmpty())
	})

	It("ignores timestamps", func() {
		current.Service.CreatedAt, current.Service.UpdatedAt = ptr(99), ptr(100)
		current.Route.UpdatedAt = ptr(100)
		current.Plugins[0].CreatedAt = ptr(99)
		current.Upstream.CreatedAt = ptr(99)
		current.Targets[0].CreatedAt = ptr(float32(99))
		Expect(rollback.Plan(desired, current, false)).To(BeEmpty())
	})

	It("upserts a changed plugin only", func() {
		(*current.Plugins[0].Config)["minute"] = 20
		ops := rollback.Plan(desired, current, false)
		Expect(planStrings(ops)).To(Equal([]string{"upsert plugin rate-limiting plugin-2"}))
		Expect(ops[0].Entity).To(BeIdenticalTo(&desired.Plugins[0]))
	})

	It("keeps a gateway-only plugin without prune", func() {
		current.Plugins = append(current.Plugins, kong.Plugin{Id: ptr("plugin-x"), Name: ptr("cors")})
		ops := rollback.Plan(desired, current, false)
		Expect(planStrings(ops)).To(Equal([]string{"keep plugin cors plugin-x: would be kept (use --prune to delete)"}))
		Expect(ops[0].IsWrite()).To(BeFalse())
	})

	It("deletes a gateway-only plugin with prune", func() {
		current.Plugins = append(current.Plugins, kong.Plugin{Id: ptr("plugin-x"), Name: ptr("cors")})
		Expect(planStrings(rollback.Plan(desired, current, true))).To(Equal([]string{"delete plugin cors plugin-x (prune)"}))
	})

	It("handles a gateway-only target", func() {
		current.Targets = append(current.Targets, kong.Target{Id: ptr("target-x"), Target: ptr("10.0.0.9:80")})
		ops := rollback.Plan(desired, current, true)
		Expect(planStrings(ops)).To(Equal([]string{"delete target target-x (prune)"}))
		Expect(ops[0].ParentID).To(Equal("up-1"))
		Expect(planStrings(rollback.Plan(desired, current, false))).To(Equal([]string{
			"keep target target-x: would be kept (use --prune to delete)",
		}))
	})

	It("handles an upstream that only exists on the gateway", func() {
		desired.Upstream, desired.Targets = nil, nil
		current.Plugins = append(current.Plugins, kong.Plugin{Id: ptr("plugin-x"), Name: ptr("cors")})
		Expect(planStrings(rollback.Plan(desired, current, true))).To(Equal([]string{
			"delete plugin cors plugin-x (prune)",
			"delete upstream up-1 (prune)",
		}))
		Expect(planStrings(rollback.Plan(desired, current, false))).To(Equal([]string{
			"keep plugin cors plugin-x: would be kept (use --prune to delete)",
			"keep upstream up-1: would be kept (use --prune to delete)",
		}))
	})
})
