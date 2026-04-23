// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package viewer_test

import (
	"context"

	"entgo.io/ent/privacy"

	"github.com/telekom/controlplane/controlplane-api/internal/viewer"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NewContext / FromContext", func() {
	It("should round-trip a viewer through the context", func() {
		v := &viewer.Viewer{Teams: []string{"team-alpha"}, Admin: false}
		ctx := viewer.NewContext(context.Background(), v)
		Expect(viewer.FromContext(ctx)).To(BeIdenticalTo(v))
	})

	It("should return nil when no viewer is in the context", func() {
		Expect(viewer.FromContext(context.Background())).To(BeNil())
	})
})

var _ = Describe("HasTeam", func() {
	It("should return true when the viewer belongs to the team", func() {
		v := &viewer.Viewer{Teams: []string{"team-alpha", "team-beta"}}
		Expect(v.HasTeam("team-alpha")).To(BeTrue())
		Expect(v.HasTeam("team-beta")).To(BeTrue())
	})

	It("should return false when the viewer does not belong to the team", func() {
		v := &viewer.Viewer{Teams: []string{"team-alpha"}}
		Expect(v.HasTeam("team-beta")).To(BeFalse())
	})

	It("should return false when teams list is empty", func() {
		v := &viewer.Viewer{Teams: []string{}}
		Expect(v.HasTeam("team-alpha")).To(BeFalse())
	})

	It("should return false when called on a nil viewer", func() {
		var v *viewer.Viewer
		Expect(v.HasTeam("team-alpha")).To(BeFalse())
	})
})

var _ = Describe("SystemContext", func() {
	It("should return a context that carries an Allow privacy decision", func() {
		ctx := viewer.SystemContext(context.Background())
		decision, ok := privacy.DecisionFromContext(ctx)
		Expect(ok).To(BeTrue())
		Expect(decision).ToNot(HaveOccurred())
	})
})
