// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql_test

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/controlplane-api/ent"
	cpgraphql "github.com/telekom/controlplane/controlplane-api/internal/graphql"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
)

var _ = Describe("ViewerFromBusinessContext", func() {
	var client *ent.Client

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
	})

	AfterEach(func() {
		client.Close()
	})

	// captureViewer invokes the middleware and returns the Viewer that was set in the context.
	captureViewer := func(ctx context.Context, securityEnabled ...bool) *viewer.Viewer {
		mw := cpgraphql.ViewerFromBusinessContext(client, securityEnabled...)
		var captured *viewer.Viewer
		next := func(ctx context.Context) graphql.ResponseHandler {
			captured = viewer.FromContext(ctx)
			return func(ctx context.Context) *graphql.Response { return nil }
		}
		mw(ctx, next)
		return captured
	}

	Context("when no BusinessContext is present", func() {
		It("should inject admin viewer when security is disabled", func() {
			v := captureViewer(context.Background(), false)
			Expect(v).NotTo(BeNil())
			Expect(v.Admin).To(BeTrue())
		})

		It("should not inject a viewer when security is enabled", func() {
			v := captureViewer(context.Background(), true)
			Expect(v).To(BeNil())
		})

		It("should default to security enabled", func() {
			v := captureViewer(context.Background())
			Expect(v).To(BeNil())
		})
	})

	Context("when BusinessContext has ClientTypeAdmin", func() {
		It("should set admin=true on the viewer", func() {
			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				ClientType: security.ClientTypeAdmin,
			})
			v := captureViewer(ctx)
			Expect(v).NotTo(BeNil())
			Expect(v.Admin).To(BeTrue())
			Expect(v.Teams).To(BeEmpty())
		})
	})

	Context("when BusinessContext has ClientTypeTeam", func() {
		It("should set the single team on the viewer", func() {
			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				ClientType: security.ClientTypeTeam,
				Team:       "team-alpha",
			})
			v := captureViewer(ctx)
			Expect(v).NotTo(BeNil())
			Expect(v.Admin).To(BeFalse())
			Expect(v.Teams).To(ConsistOf("team-alpha"))
		})
	})

	Context("when BusinessContext has ClientTypeGroup", func() {
		It("should resolve all teams belonging to the group", func() {
			// Seed teams in group-a
			s := testutil.SeedStandard(client)
			_ = s

			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				ClientType: security.ClientTypeGroup,
				Group:      "group-a",
			})
			v := captureViewer(ctx)
			Expect(v).NotTo(BeNil())
			Expect(v.Admin).To(BeFalse())
			Expect(v.Teams).To(ConsistOf("team-alpha"))
		})

		It("should return empty teams when group has no teams", func() {
			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				ClientType: security.ClientTypeGroup,
				Group:      "nonexistent-group",
			})
			v := captureViewer(ctx)
			Expect(v).NotTo(BeNil())
			Expect(v.Teams).To(BeEmpty())
		})
	})
})
