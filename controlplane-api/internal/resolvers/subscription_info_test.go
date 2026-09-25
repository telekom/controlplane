// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	"github.com/telekom/controlplane/controlplane-api/internal/interceptor"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	gqlmodel "github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	_ gqlmodel.SubscriptionInfo = (*gqlmodel.APISubscriptionInfo)(nil)
	_ gqlmodel.SubscriptionInfo = (*gqlmodel.EventSubscriptionInfo)(nil)
	_ gqlmodel.SubscriptionInfo = (*gqlmodel.AgenticSubscriptionInfo)(nil)
)

var _ = Describe("SubscriptionInfo", func() {
	It("exposes application external IDs without sensitive direct application fields", func() {
		schema := resolvers.NewExecutableSchema(resolvers.Config{}).Schema()
		applicationInfo := schema.Types["ApplicationInfo"]
		field := applicationInfo.Fields.ForName("externalIDs")
		Expect(field).NotTo(BeNil())
		Expect(field.Type.String()).To(Equal("[ExternalID!]"))
		Expect(field.Type.String()).To(Equal(schema.Types["Application"].Fields.ForName("externalIDs").Type.String()))
		for _, name := range []string{"clientID", "clientSecret", "permissionSet", "exposedAPIs", "subscribedAPIs"} {
			Expect(schema.Types["Application"].Fields.ForName(name)).NotTo(BeNil(), name)
			Expect(applicationInfo.Fields.ForName(name)).To(BeNil(), name)
		}
	})

	It("exposes only common ownership fields and preserves approval subscription field types", func() {
		schema := resolvers.NewExecutableSchema(resolvers.Config{}).Schema()
		subscription := schema.Types["SubscriptionInfo"]
		Expect(subscription.Kind).To(Equal(ast.Interface))
		Expect(subscription.Fields).To(HaveLen(2))
		Expect(subscription.Fields.ForName("id").Type.String()).To(Equal("ID!"))
		Expect(subscription.Fields.ForName("ownerApplication").Type.String()).To(Equal("ApplicationInfo!"))
		possibleTypes := schema.GetPossibleTypes(subscription)
		implementors := make([]string, 0, len(possibleTypes))
		for _, typ := range possibleTypes {
			implementors = append(implementors, typ.Name)
		}
		Expect(implementors).To(ConsistOf("APISubscriptionInfo", "EventSubscriptionInfo", "AgenticSubscriptionInfo"))
		Expect(schema.Types).NotTo(HaveKey("OwnedSubscriptionInfo"))
		for _, name := range []string{"Approval", "ApprovalRequest"} {
			Expect(schema.Types[name].Fields.ForName("subscription").Type.String()).To(Equal("SubscriptionInfo!"))
		}
	})

	Describe("GraphQL execution", func() {
		var (
			db     *ent.Client
			seed   *testutil.SeedData
			server *handler.Server
		)

		BeforeEach(func() {
			// GraphQL resolves sibling fields concurrently, so connections must share the in-memory database.
			db = enttest.Open(GinkgoT(), "sqlite3", "file:subscriptions?mode=memory&cache=shared&_fk=1")
			db.Intercept(interceptor.TeamFilterInterceptor())
			seed = testutil.SeedStandard(db)
			ctx := testutil.AllowContext()
			_, err := db.Approval.Create().
				SetNamespace("prod").SetName("event-approval").SetAction("ALLOW").
				SetRequester(seed.Approval.Requester).SetDecider(seed.Approval.Decider).
				SetDeciderTeamName("team-alpha").SetEventSubscription(seed.EventSubscription).Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.ApprovalRequest.Create().
				SetNamespace("prod").SetName("event-approval-request").SetAction("ALLOW").
				SetRequester(seed.ApprovalRequest.Requester).SetDecider(seed.ApprovalRequest.Decider).
				SetDeciderTeamName("team-alpha").SetEventSubscription(seed.EventSubscription).Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			server = handler.New(resolvers.NewExecutableSchema(resolvers.Config{
				Resolvers: resolvers.NewResolver(db, service.Services{}, nil, ""),
			}))
			server.AddTransport(transport.POST{})
		})

		AfterEach(func() {
			Expect(db.Close()).To(Succeed())
		})

		DescribeTable("returns persisted external IDs through a reduced cross-tenant application view",
			func(ids []model.ExternalID, expectedJSON string) {
				if ids != nil {
					_, err := db.Application.UpdateOne(seed.AppBeta).SetExternalIDs(ids).Save(testutil.AllowContext())
					Expect(err).NotTo(HaveOccurred())
				}
				body, err := json.Marshal(map[string]string{"query": `{
					approvals {
						edges { node { subscription {
							ownerApplication { id externalIDs { id scheme } }
						} } }
					}
					applications { edges { node { id } } }
				}`})
				Expect(err).NotTo(HaveOccurred())
				req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req = req.WithContext(viewer.NewContext(context.Background(), &viewer.Viewer{
					Teams: []string{seed.TeamAlpha.Name},
				}))
				recorder := httptest.NewRecorder()
				server.ServeHTTP(recorder, req)
				Expect(recorder.Code).To(Equal(http.StatusOK), recorder.Body.String())

				var response struct {
					Data struct {
						Approvals struct {
							Edges []struct {
								Node struct {
									Subscription struct {
										OwnerApplication struct {
											ID          string
											ExternalIDs json.RawMessage
										}
									}
								}
							}
						}
						Applications struct {
							Edges []struct{ Node struct{ ID string } }
						}
					}
					Errors gqlerror.List
				}
				Expect(json.Unmarshal(recorder.Body.Bytes(), &response)).To(Succeed())
				Expect(response.Errors).To(BeEmpty())
				Expect(response.Data.Approvals.Edges).To(HaveLen(3))
				for _, edge := range response.Data.Approvals.Edges {
					owner := edge.Node.Subscription.OwnerApplication
					Expect(owner.ID).To(Equal(strconv.Itoa(seed.AppBeta.ID)))
					Expect(string(owner.ExternalIDs)).To(MatchJSON(expectedJSON))
				}
				Expect(response.Data.Applications.Edges).To(HaveLen(1))
				Expect(response.Data.Applications.Edges[0].Node.ID).To(Equal(strconv.Itoa(seed.AppAlpha.ID)))
			},
			Entry("populated IDs retain their values and order",
				[]model.ExternalID{{ID: "z-123", Scheme: "inventory"}, {ID: "a-456", Scheme: "catalog"}},
				`[{"id":"z-123","scheme":"inventory"},{"id":"a-456","scheme":"catalog"}]`),
			Entry("absent IDs serialize as null", []model.ExternalID(nil), `null`),
			Entry("explicitly empty IDs serialize as an empty array", []model.ExternalID{}, `[]`),
		)

		DescribeTable("resolves common fields for every implementation alongside concrete fragments",
			func(field, selection, fragment string) {
				query := fmt.Sprintf(`{
					%s {
						edges { node { subscription {
							__typename
							%s
							... on APISubscriptionInfo { basePath }
							... on EventSubscriptionInfo { eventType }
							... on AgenticSubscriptionInfo { basePath }
						} } }
					}
				} %s`, field, selection, fragment)
				body, err := json.Marshal(map[string]string{"query": query})
				Expect(err).NotTo(HaveOccurred())
				req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req = req.WithContext(viewer.NewContext(req.Context(), &viewer.Viewer{Admin: true}))
				recorder := httptest.NewRecorder()
				server.ServeHTTP(recorder, req)
				Expect(recorder.Code).To(Equal(http.StatusOK), recorder.Body.String())

				type subscription struct {
					Typename         string `json:"__typename"`
					ID               string
					BasePath         string
					EventType        string
					OwnerApplication struct{ ID, Name string }
				}
				var response struct {
					Data map[string]struct {
						Edges []struct {
							Node struct{ Subscription subscription }
						}
					}
					Errors gqlerror.List
				}
				Expect(json.Unmarshal(recorder.Body.Bytes(), &response)).To(Succeed())
				Expect(response.Errors).To(BeEmpty())
				expected := map[string]subscription{
					"APISubscriptionInfo":     {ID: strconv.Itoa(seed.Subscription.ID), BasePath: "/alpha"},
					"EventSubscriptionInfo":   {ID: strconv.Itoa(seed.EventSubscription.ID), EventType: "order.created"},
					"AgenticSubscriptionInfo": {ID: strconv.Itoa(seed.AgenticSubscription.ID), BasePath: "/mcp-alpha"},
				}
				Expect(response.Data[field].Edges).To(HaveLen(3))
				for _, edge := range response.Data[field].Edges {
					actual := edge.Node.Subscription
					Expect(expected).To(HaveKey(actual.Typename))
					want := expected[actual.Typename]
					want.Typename = actual.Typename
					want.OwnerApplication.ID = strconv.Itoa(seed.AppBeta.ID)
					want.OwnerApplication.Name = seed.AppBeta.Name
					Expect(actual).To(Equal(want))
					delete(expected, actual.Typename)
				}
				Expect(expected).To(BeEmpty())
			},
			Entry("direct common fields on approvals", "approvals",
				"id ownerApplication { id name }", ""),
			Entry("inline interface fragment on approvals", "approvals",
				"... on SubscriptionInfo { id ownerApplication { id name } }", ""),
			Entry("named interface fragment on approvals", "approvals", "...Owner",
				"fragment Owner on SubscriptionInfo { id ownerApplication { id name } }"),
			Entry("direct common fields on approval requests", "approvalRequests",
				"id ownerApplication { id name }", ""),
			Entry("inline interface fragment on approval requests", "approvalRequests",
				"... on SubscriptionInfo { id ownerApplication { id name } }", ""),
			Entry("named interface fragment on approval requests", "approvalRequests", "...Owner",
				"fragment Owner on SubscriptionInfo { id ownerApplication { id name } }"),
		)
	})
})
