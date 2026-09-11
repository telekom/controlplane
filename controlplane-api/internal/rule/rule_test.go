// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rule_test

import (
	"context"

	"entgo.io/ent/privacy"

	"github.com/telekom/controlplane/controlplane-api/internal/rule"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DenyIfNoViewer", func() {
	r := rule.DenyIfNoViewer()

	It("should deny when no viewer is in context", func() {
		err := r.EvalQuery(context.Background(), nil)
		Expect(err).To(MatchError(privacy.Denyf("viewer-context is missing")))
	})

	It("should skip when a viewer is present", func() {
		ctx := viewer.NewContext(context.Background(), &viewer.Viewer{})
		err := r.EvalQuery(ctx, nil)
		Expect(err).To(MatchError(privacy.Skip))
	})
})

var _ = Describe("DenyIfNoTeams", func() {
	r := rule.DenyIfNoTeams()

	It("should deny when no viewer is in context", func() {
		err := r.EvalQuery(context.Background(), nil)
		Expect(err).To(MatchError(privacy.Denyf("viewer-context is missing")))
	})

	It("should deny when viewer has no teams and is not admin", func() {
		ctx := viewer.NewContext(context.Background(), &viewer.Viewer{Teams: []string{}})
		err := r.EvalQuery(ctx, nil)
		Expect(err).To(MatchError(privacy.Denyf("viewer has no team access")))
	})

	It("should skip when viewer is admin even with no teams", func() {
		ctx := viewer.NewContext(context.Background(), &viewer.Viewer{Admin: true})
		err := r.EvalQuery(ctx, nil)
		Expect(err).To(MatchError(privacy.Skip))
	})

	It("should skip when viewer has teams", func() {
		ctx := viewer.NewContext(context.Background(), &viewer.Viewer{Teams: []string{"team-alpha"}})
		err := r.EvalQuery(ctx, nil)
		Expect(err).To(MatchError(privacy.Skip))
	})
})
