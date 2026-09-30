// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subscription scope fields", func() {
	It("exposes separate requested and active scopes and deprecates only the legacy subscription fields", func() {
		schema := resolvers.NewExecutableSchema(resolvers.Config{}).Schema()
		for _, name := range []string{"ApiSubscription", "EventSubscription", "AgenticSubscription"} {
			Expect(schema.Types[name].Fields.ForName("requestedScopes").Type.String()).To(Equal("[String!]"))
			Expect(schema.Types[name].Fields.ForName("activeScopes").Type.String()).To(Equal("[String!]"))
		}
		for _, name := range []string{"EventSubscription", "SubscriberMachine2MachineAuthentication"} {
			Expect(schema.Types[name].Fields.ForName("scopes").Directives.ForName("deprecated")).NotTo(BeNil())
		}
		Expect(schema.Types["Machine2MachineAuthentication"].Fields.ForName("scopes").Directives.ForName("deprecated")).To(BeNil())
	})

	It("returns distinct scope sets for all three subscription types through GraphQL", func() {
		db := enttest.Open(GinkgoT(), "sqlite3", "file:scope-fields?mode=memory&cache=shared&_fk=1")
		DeferCleanup(func() { Expect(db.Close()).To(Succeed()) })
		seed := testutil.SeedStandard(db)
		ctx := testutil.AllowContext()
		requested := []string{"read", "write"}
		active := []string{"read"}
		m2m := &model.SubscriberMachine2MachineAuthentication{Scopes: requested}
		Expect(db.ApiSubscription.UpdateOne(seed.Subscription).
			SetRequestedScopes(requested).SetActiveScopes(active).
			SetSecurity(&model.ApiSubscriptionSecurity{M2M: m2m}).Exec(ctx)).To(Succeed())
		Expect(db.EventSubscription.UpdateOne(seed.EventSubscription).
			SetRequestedScopes(requested).SetActiveScopes(active).SetScopes(requested).Exec(ctx)).To(Succeed())
		Expect(db.AgenticSubscription.UpdateOne(seed.AgenticSubscription).
			SetRequestedScopes(requested).SetActiveScopes(active).
			SetSecurity(model.AgenticSubscriptionSecurity{M2M: m2m}).Exec(ctx)).To(Succeed())
		server := handler.New(resolvers.NewExecutableSchema(resolvers.Config{
			Resolvers: resolvers.NewResolver(db, service.Services{}, nil, ""),
		}))
		server.AddTransport(transport.POST{})
		body, err := json.Marshal(map[string]string{"query": `{
			apiSubscriptions { edges { node { requestedScopes activeScopes security { m2m { scopes } } } } }
			eventSubscriptions { edges { node { requestedScopes activeScopes scopes } } }
			agenticSubscriptions { edges { node { requestedScopes activeScopes security { m2m { scopes } } } } }
		}`})
		Expect(err).NotTo(HaveOccurred())
		req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)
		Expect(recorder.Code).To(Equal(http.StatusOK))
		var result struct {
			Data map[string]struct {
				Edges []struct {
					Node struct {
						RequestedScopes []string
						ActiveScopes    []string
						Scopes          []string
						Security        *model.ApiSubscriptionSecurity
					}
				}
			}
			Errors []any
		}
		Expect(json.Unmarshal(recorder.Body.Bytes(), &result)).To(Succeed())
		Expect(result.Errors).To(BeEmpty(), recorder.Body.String())
		Expect(result.Data).To(HaveLen(3))
		for name, connection := range result.Data {
			Expect(connection.Edges).To(HaveLen(1), name)
			node := connection.Edges[0].Node
			Expect(node.RequestedScopes).To(Equal(requested), name)
			Expect(node.ActiveScopes).To(Equal(active), name)
			if name == "eventSubscriptions" {
				Expect(node.Scopes).To(Equal(requested))
			} else {
				Expect(node.Security.M2M.Scopes).To(Equal(requested))
			}
		}
	})
})
