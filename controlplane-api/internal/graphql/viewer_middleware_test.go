// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql_test

import (
	"context"
	"fmt"
	"os"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/99designs/gqlgen/graphql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common/pkg/util/emailutil"
	"github.com/telekom/controlplane/controlplane-api/ent"
	cpgraphql "github.com/telekom/controlplane/controlplane-api/internal/graphql"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ViewerFromBusinessContext", func() {
	var client *ent.Client

	BeforeEach(func() {
		// Opt-in PostgreSQL runs use a fresh schema per spec, like the SQLite database.
		url := os.Getenv("CP_TEST_POSTGRES_URL")
		if url == "" {
			client = testutil.NewTestClient(GinkgoT())
			DeferCleanup(func() { Expect(client.Close()).To(Succeed()) })
			return
		}
		cfg, err := pgx.ParseConfig(url)
		Expect(err).NotTo(HaveOccurred())
		schemaName := fmt.Sprintf("pr670_viewer_%d", time.Now().UnixNano())
		cfg.RuntimeParams["search_path"] = schemaName
		db := stdlib.OpenDB(*cfg)
		DeferCleanup(func() { Expect(db.Close()).To(Succeed()) })
		_, err = db.Exec("CREATE SCHEMA " + pgx.Identifier{schemaName}.Sanitize())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_, dropErr := db.Exec("DROP SCHEMA " + pgx.Identifier{schemaName}.Sanitize() + " CASCADE")
			Expect(dropErr).NotTo(HaveOccurred())
		})
		client = ent.NewClient(ent.Driver(sql.OpenDB(dialect.Postgres, db)))
		Expect(client.Schema.Create(context.Background())).To(Succeed())
	})

	// captureViewer invokes the middleware and returns the Viewer that was set in the context.
	captureViewer := func(ctx context.Context) *viewer.Viewer {
		mw := cpgraphql.ViewerFromBusinessContext(client)
		var captured *viewer.Viewer
		next := func(ctx context.Context) graphql.ResponseHandler {
			captured = viewer.FromContext(ctx)
			return func(ctx context.Context) *graphql.Response { return nil }
		}
		mw(ctx, next)
		return captured
	}

	Context("when no BusinessContext is present", func() {
		It("should not inject a viewer", func() {
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

	Context("when ForwardedUser is in context", func() {
		It("should populate UserName and UserEmail on the viewer", func() {
			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				ClientType: security.ClientTypeAdmin,
			})
			ctx = viewer.NewForwardedUserContext(ctx, viewer.ForwardedUser{
				Name:  "Jane Doe",
				Email: "jane@example.com",
			})
			v := captureViewer(ctx)
			Expect(v).NotTo(BeNil())
			Expect(v.UserName).To(Equal("Jane Doe"))
			Expect(v.UserEmail).To(Equal("jane@example.com"))
		})

		It("should leave UserName and UserEmail empty when no ForwardedUser", func() {
			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				ClientType: security.ClientTypeTeam,
				Team:       "team-alpha",
			})
			v := captureViewer(ctx)
			Expect(v).NotTo(BeNil())
			Expect(v.UserName).To(BeEmpty())
			Expect(v.UserEmail).To(BeEmpty())
		})
	})

	Context("user-scoped access via ForwardedUser email", func() {
		It("should scope admin JWT down to user's team memberships", func() {
			s := testutil.SeedStandard(client)
			_ = s

			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				ClientType: security.ClientTypeAdmin,
			})
			ctx = viewer.NewForwardedUserContext(ctx, viewer.ForwardedUser{
				Email: "alice@test.dev",
			})
			v := captureViewer(ctx)
			Expect(v).NotTo(BeNil())
			Expect(v.Admin).To(BeFalse(), "admin should be overridden when user email is present")
			Expect(v.Teams).To(ConsistOf("team-alpha"))
		})

		DescribeTable("matches lowercase equivalents across teams without granting unrelated access", func(storedEmail, requestEmail string) {
			s := testutil.SeedStandard(client)
			seedCtx := testutil.AllowContext()
			_, err := client.Member.UpdateOne(s.MemberAlpha).SetEmail(storedEmail).Save(seedCtx)
			Expect(err).NotTo(HaveOccurred())
			_, err = client.Member.Create().SetName("Alice").SetEmail(storedEmail).SetTeam(s.TeamBeta).Save(seedCtx)
			Expect(err).NotTo(HaveOccurred())
			unrelated, err := client.Team.Create().SetNamespace("default").SetName("team-unrelated").
				SetEmail("unrelated@test.dev").SetGroup(s.GroupA).Save(seedCtx)
			Expect(err).NotTo(HaveOccurred())
			_, err = client.Member.Create().SetName("Other Alice").SetEmail("alice.other@test.dev").SetTeam(unrelated).Save(seedCtx)
			Expect(err).NotTo(HaveOccurred())

			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				ClientType: security.ClientTypeAdmin,
			})
			ctx = viewer.NewForwardedUserContext(ctx, viewer.ForwardedUser{
				Email: requestEmail,
			})
			v := captureViewer(ctx)
			Expect(v).NotTo(BeNil())
			Expect(v.Teams).To(ConsistOf("team-alpha", "team-beta"))
			Expect(v.Admin).To(BeFalse())
			Expect(v.UserEmail).To(Equal(requestEmail))
			stored, err := client.Member.Get(seedCtx, s.MemberAlpha.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(stored.Email).To(Equal(storedEmail))
		},
			Entry("ASCII", "alice@test.dev", "aLICE@tEST.dEV"),
			Entry("Unicode", "üser@bücher.dev", "ÜSER@BÜCHER.DEV"),
		)

		DescribeTable("literal email membership matching",
			func(email, nearMatch string) {
				s := testutil.SeedStandard(client)
				seedCtx := testutil.AllowContext()
				_, err := client.Member.UpdateOne(s.MemberBeta).SetEmail(nearMatch).Save(seedCtx)
				Expect(err).NotTo(HaveOccurred())
				ctx := security.ToContext(context.Background(), &security.BusinessContext{
					ClientType: security.ClientTypeAdmin,
				})
				ctx = viewer.NewForwardedUserContext(ctx, viewer.ForwardedUser{Email: email})
				v := captureViewer(ctx)
				Expect(v).NotTo(BeNil())
				Expect(v.Teams).To(BeEmpty(), "near-matches must not grant membership")
				Expect(v.Admin).To(BeFalse())
				Expect(v.UserEmail).To(Equal(email))

				_, err = client.Member.UpdateOne(s.MemberAlpha).SetEmail(emailutil.Canonicalize(email)).Save(seedCtx)
				Expect(err).NotTo(HaveOccurred())
				v = captureViewer(ctx)
				Expect(v).NotTo(BeNil())
				Expect(v.Teams).To(ConsistOf("team-alpha"))
				Expect(v.Admin).To(BeFalse())
				Expect(v.UserEmail).To(Equal(email))
			},
			Entry("percent", "A%ICE@TEST.DEV", "alice@test.dev"),
			Entry("underscore", "AL_CE@TEST.DEV", "alice@test.dev"),
			Entry("backslash", `"ALI\\CE"@TEST.DEV`, "alice@test.dev"),
			Entry("quote", "O'NEIL@TEST.DEV", "oneil@test.dev"),
			Entry("SQL-like input", `"' OR 1=1 --"@TEST.DEV`, "alice@test.dev"),
			Entry("no full case folding", "STRASSE@TEST.DEV", "straße@test.dev"),
			Entry("no Unicode normalization", "U\u0308SER@TEST.DEV", "üser@test.dev"),
		)

		It("should keep admin=true when ForwardedUser has IsAdmin=true", func() {
			testutil.SeedStandard(client)

			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				ClientType: security.ClientTypeAdmin,
			})
			ctx = viewer.NewForwardedUserContext(ctx, viewer.ForwardedUser{
				Email:   "alice@test.dev",
				IsAdmin: true,
			})
			v := captureViewer(ctx)
			Expect(v).NotTo(BeNil())
			Expect(v.Admin).To(BeTrue(), "admin bypass should be preserved when IsAdmin header is set")
		})

		It("should return empty teams when user has no memberships", func() {
			testutil.SeedStandard(client)

			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				ClientType: security.ClientTypeAdmin,
			})
			ctx = viewer.NewForwardedUserContext(ctx, viewer.ForwardedUser{
				Email: "unknown@test.dev",
			})
			v := captureViewer(ctx)
			Expect(v).NotTo(BeNil())
			Expect(v.Admin).To(BeFalse())
			Expect(v.Teams).To(BeEmpty())
		})

		It("should scope group JWT down to user's team memberships", func() {
			testutil.SeedStandard(client)

			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				ClientType: security.ClientTypeGroup,
				Group:      "group-a",
			})
			ctx = viewer.NewForwardedUserContext(ctx, viewer.ForwardedUser{
				Email: "alice@test.dev",
			})
			v := captureViewer(ctx)
			Expect(v).NotTo(BeNil())
			Expect(v.Admin).To(BeFalse())
			Expect(v.Teams).To(ConsistOf("team-alpha"))
		})

		It("should not alter viewer when ForwardedUser has no email", func() {
			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				ClientType: security.ClientTypeAdmin,
			})
			ctx = viewer.NewForwardedUserContext(ctx, viewer.ForwardedUser{
				Name: "Jane Doe",
			})
			v := captureViewer(ctx)
			Expect(v).NotTo(BeNil())
			Expect(v.Admin).To(BeTrue(), "admin should remain when no email is forwarded")
		})
	})
})
