// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	clientpkg "github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/legacy"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	javaRealmBond      = "ei__telekom__de--realm---bond"
	javaRealmDefault   = "ei__telekom__de--realm---default"
	javaRealmDefaultAI = "ei__telekom__de--realm---default-ai"
)

func snapshotMigrator() legacy.Migrator {
	m, err := legacy.LoadMigrator("../legacy/testdata/legacy-naming.yaml")
	Expect(err).NotTo(HaveOccurred())
	return m
}

func tagsOf(entity map[string]any) []any {
	tags, _ := entity["tags"].([]any)
	return tags
}

func anyTags(tags ...string) []any {
	out := make([]any, len(tags))
	for i, tag := range tags {
		out[i] = tag
	}
	return out
}

var _ = Describe("Realm route", func() {
	var (
		ctx      context.Context
		kongFake *fakeKong
		client   clientpkg.KongClient
		route    *v1.Route
		upstream clientpkg.Upstream
	)

	newRoute := func(name, prefix string) *v1.Route {
		r := &v1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: v1.RouteSpec{
				Hostnames: []string{"stargate.example"},
				Paths:     []string{"/eni/echo/v3"},
			},
		}
		r.SetNamePrefix(prefix)
		return r
	}

	// cpEntity is a route or service the control plane created before the migration.
	cpEntity := func(name string, extraTags ...string) map[string]any {
		return map[string]any{"name": name, "tags": anyTags(append([]string{"env--bond", "route--eni-example-v1"}, extraTags...)...)}
	}

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "bond")
		kongFake = newFakeKong()
		client = clientpkg.NewKongClient(kongFake.api(), clientpkg.WithMigrator(snapshotMigrator()))
		route = newRoute("eni-example-v1", "bond")
		upstream = clientpkg.NewUpstreamOrDie("http://localhost:8080/proxy")
	})

	reconcileTwice := func() {
		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
		kongFake.resetCalls()
		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
		Expect(kongFake.writes()).To(BeEmpty(), "second reconcile must not write")
	}

	Describe("CreateOrReplaceRoute", func() {
		It("creates route and service under the realm name with the realm tag", func() {
			reconcileTwice()

			Expect(kongFake.names("routes")).To(Equal([]string{"bond--eni-example-v1"}))
			Expect(kongFake.names("services")).To(Equal([]string{"bond--eni-example-v1"}))
			kongRoute := kongFake.get("routes", route.GetRouteId())
			Expect(tagsOf(kongRoute)).To(ConsistOf(anyTags("env--bond", "route--eni-example-v1", "realm--bond")))
			Expect(tagsOf(kongFake.get("services", route.GetProperty("serviceId")))).To(ConsistOf(anyTags("env--bond", "route--eni-example-v1", "realm--bond")))
			Expect(route.GetProperty("legacyName")).To(BeEmpty())
		})

		It("finds the route by its stored ID and renames it by ID", func() {
			id := kongFake.add("routes", map[string]any{"name": "manually-renamed"})
			route.SetRouteId(id)

			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
			Expect(route.GetRouteId()).To(Equal(id))
			Expect(kongFake.get("routes", id)["name"]).To(Equal("bond--eni-example-v1"))
			Expect(kongFake.writes()).To(ContainElement("PUT /routes/" + id))
		})

		It("goes on to the next step if the stored ID is gone", func() {
			route.SetRouteId("gone")
			id := kongFake.add("routes", cpEntity("bond--eni-example-v1"))

			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
			Expect(route.GetRouteId()).To(Equal(id))
		})

		It("prefers the stored IDs captured before a status reset", func() {
			id := kongFake.add("routes", map[string]any{"name": "manually-renamed"})
			route.SetRouteId(id)
			route.CaptureStoredIds()
			route.Status.Properties = map[string]string{}

			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
			Expect(route.GetRouteId()).To(Equal(id))
		})

		It("finds the route by its new name without further lookups", func() {
			id := kongFake.add("routes", cpEntity("bond--eni-example-v1", "realm--bond"))
			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
			Expect(route.GetRouteId()).To(Equal(id))
			Expect(kongFake.calls).NotTo(ContainElement("GET /routes/eni-example-v1"))
		})

		It("finds the route by its CP tags after a name prefix change", func() {
			id := kongFake.add("routes", cpEntity("old--eni-example-v1", "realm--old"))

			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
			Expect(route.GetRouteId()).To(Equal(id))
			kongRoute := kongFake.get("routes", id)
			Expect(kongRoute["name"]).To(Equal("bond--eni-example-v1"))
			Expect(tagsOf(kongRoute)).NotTo(ContainElement("realm--old"))
		})

		It("blocks if the CP tags match more than one route", func() {
			kongFake.add("routes", cpEntity("a--eni-example-v1"))
			kongFake.add("routes", cpEntity("b--eni-example-v1"))

			err := client.CreateOrReplaceRoute(ctx, route, upstream)
			Expect(err).To(MatchError(ContainSubstring("expected at most one")))
			Expect(isBlocked(err)).To(BeTrue())
			Expect(kongFake.writes()).To(BeEmpty())
		})

		It("adopts the pre-migration CP route and service and records the legacy name", func() {
			serviceID := kongFake.add("services", cpEntity("eni-example-v1"))
			routeEntity := cpEntity("eni-example-v1")
			routeEntity["service"] = map[string]any{"id": serviceID}
			id := kongFake.add("routes", routeEntity)
			route.Spec.AdditionalTags = []string{"variant--mcp"}

			reconcileTwice()
			Expect(route.GetRouteId()).To(Equal(id))
			Expect(route.GetProperty("serviceId")).To(Equal(serviceID))
			Expect(kongFake.names("routes")).To(Equal([]string{"bond--eni-example-v1"}))
			Expect(kongFake.names("services")).To(Equal([]string{"bond--eni-example-v1"}))
			Expect(tagsOf(kongFake.get("routes", id))).To(ConsistOf(anyTags("env--bond", "route--eni-example-v1", "realm--bond", "variant--mcp")))
		})

		It("records the legacy name on adoption", func() {
			kongFake.add("routes", cpEntity("eni-example-v1"))
			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
			Expect(route.GetProperty("legacyName")).To(Equal("eni-example-v1"))
		})

		It("blocks on a pre-migration route without CP tags and writes nothing", func() {
			kongFake.add("routes", map[string]any{"name": "eni-example-v1", "tags": anyTags(javaRealmDefault)})

			err := client.CreateOrReplaceRoute(ctx, route, upstream)
			Expect(err).To(MatchError(ContainSubstring("not owned")))
			Expect(isBlocked(err)).To(BeTrue())
			Expect(kongFake.writes()).To(BeEmpty())
		})

		It("blocks on a pre-migration route with only one CP tag", func() {
			kongFake.add("routes", map[string]any{"name": "eni-example-v1", "tags": anyTags("route--eni-example-v1")})
			Expect(isBlocked(client.CreateOrReplaceRoute(ctx, route, upstream))).To(BeTrue())
		})

		DescribeTable("adopts the Java route and service of a snapshot without a rewrite loop",
			func(file, name, prefix, ownerTag string, labels map[string]string) {
				snapshot := loadSnapshot(file)
				kongFake.add("services", snapshot.Service)
				kongFake.add("routes", snapshot.Route)
				javaName := snapshot.Route["name"]
				route = newRoute(name, prefix)
				route.Labels = labels
				route.Spec.Hostnames = nil
				route.Spec.Paths = []string{"/new/path"}

				reconcileTwice()

				Expect(route.GetRouteId()).To(Equal("legacy-route-id"))
				Expect(route.GetProperty("serviceId")).To(Equal("legacy-service-id"))
				Expect(route.GetProperty("legacyName")).To(Equal(javaName))
				Expect(kongFake.names("routes")).To(Equal([]string{prefix + "--" + name}))
				Expect(kongFake.names("services")).To(Equal([]string{prefix + "--" + name}))

				kongRoute := kongFake.get("routes", "legacy-route-id")
				Expect(kongRoute["paths"]).To(Equal(anyTags("/new/path")))
				Expect(tagsOf(kongRoute)).To(ContainElements(anyTags(ownerTag, "env--bond", "route--"+name, "realm--"+prefix)))
				for _, tag := range tagsOf(snapshot.Route) {
					Expect(tagsOf(kongRoute)).To(ContainElement(tag))
				}
				Expect(tagsOf(kongFake.get("services", "legacy-service-id"))).To(ContainElements(anyTags(ownerTag, "realm--"+prefix)))
			},
			Entry("bond API route", "bond-api.yaml", "eni-example-v1", "bond", javaRealmBond, nil),
			Entry("default API route named like metadata.name", "default-api.yaml", "eni-example-v2", "default", javaRealmDefault, nil),
			Entry("default-ai route", "default-ai.yaml", "ai-gateway--example-mcp-v1", "default", javaRealmDefaultAI, nil),
			Entry("event publish route", "bond-publish.yaml", "publish", "bond", javaRealmBond, map[string]string{"cp.ei.telekom.de/domain": "event"}),
			Entry("event callback route", "bond-callback.yaml", "callback--alpha", "bond", javaRealmBond, map[string]string{"cp.ei.telekom.de/domain": "event"}),
			Entry("event sse route", "bond-sse.yaml", "sse--de-example-orders-v1", "bond", javaRealmBond, map[string]string{"cp.ei.telekom.de/domain": "event"}),
			Entry("event voyager route", "bond-voyager.yaml", "voyager--alpha", "bond", javaRealmBond, map[string]string{"cp.ei.telekom.de/domain": "event"}),
		)

		It("does not adopt the Java event route for an API route named publish", func() {
			snapshot := loadSnapshot("bond-publish.yaml")
			kongFake.add("routes", snapshot.Route)
			route = newRoute("publish", "bond")

			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
			Expect(route.GetRouteId()).NotTo(Equal("legacy-route-id"))
			Expect(kongFake.get("routes", "legacy-route-id")["name"]).To(Equal("bond.horizon-events"))
		})

		It("blocks on a Java route of another realm under metadata.name", func() {
			kongFake.add("routes", map[string]any{"name": "eni-example-v1", "tags": anyTags(javaRealmDefault)})
			route = newRoute("eni-example-v1", "bond")
			Expect(isBlocked(client.CreateOrReplaceRoute(ctx, route, upstream))).To(BeTrue())
		})

		It("blocks on a Java candidate without the owner tag", func() {
			kongFake.add("routes", map[string]any{"name": "bond.eni-example-v1", "tags": anyTags(javaRealmDefault)})
			err := client.CreateOrReplaceRoute(ctx, route, upstream)
			Expect(isBlocked(err)).To(BeTrue())
			Expect(kongFake.writes()).To(BeEmpty())
		})

		It("adopts a Java candidate that carries the CP tags", func() {
			id := kongFake.add("routes", cpEntity("bond.eni-example-v1"))
			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
			Expect(route.GetRouteId()).To(Equal(id))
		})

		It("tries no Java candidates without a Migrator", func() {
			client = clientpkg.NewKongClient(kongFake.api())
			kongFake.add("routes", map[string]any{"name": "bond.eni-example-v1", "tags": anyTags(javaRealmBond)})

			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
			Expect(route.GetRouteId()).NotTo(BeEmpty())
			Expect(kongFake.names("routes")).To(ConsistOf("bond.eni-example-v1", "bond--eni-example-v1"))
		})

		It("keeps Java tags but drops a removed additional tag", func() {
			snapshot := loadSnapshot("bond-api.yaml")
			kongFake.add("services", snapshot.Service)
			kongFake.add("routes", snapshot.Route)
			route.Spec.AdditionalTags = []string{"variant--mcp"}
			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())

			route.Spec.AdditionalTags = nil
			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
			tags := tagsOf(kongFake.get("routes", "legacy-route-id"))
			Expect(tags).NotTo(ContainElement("variant--mcp"))
			Expect(tags).To(ContainElement(javaRealmBond))
		})

		Describe("name collision with an unprefixed route", func() {
			var foreignRoute, foreignService string

			BeforeEach(func() {
				route = newRoute("orders", "bond")
				foreignService = kongFake.add("services", map[string]any{"name": "bond--orders", "tags": anyTags("env--bond", "route--bond--orders")})
				foreignRoute = kongFake.add("routes", map[string]any{
					"name": "bond--orders", "tags": anyTags("env--bond", "route--bond--orders"), "service": map[string]any{"id": foreignService},
				})
			})

			It("blocks the reconcile and leaves the foreign entities untouched", func() {
				err := client.CreateOrReplaceRoute(ctx, route, upstream)
				Expect(isBlocked(err)).To(BeTrue())
				Expect(err).To(MatchError(ContainSubstring("not owned")))
				Expect(kongFake.writes()).To(BeEmpty())
			})

			It("does not delete the foreign entities", func() {
				Expect(client.DeleteRoute(ctx, route)).To(Succeed())
				Expect(kongFake.get("routes", foreignRoute)).NotTo(BeNil())
				Expect(kongFake.get("services", foreignService)).NotTo(BeNil())
				Expect(kongFake.writes()).To(BeEmpty())
			})

			It("blocks on a foreign upstream under the new name", func() {
				kongFake.add("upstreams", map[string]any{"name": "bond--eni-example-v1", "tags": anyTags("env--other", "route--eni-example-v1")})
				route = newRoute("eni-example-v1", "bond")
				Expect(isBlocked(client.CreateOrReplaceUpstream(ctx, route, &kong.CreateUpstreamJSONRequestBody{Name: "bond--eni-example-v1"}, &kong.CreateTargetForUpstreamJSONRequestBody{Target: ptr("localhost:8080")}))).To(BeTrue())
			})

			It("adopts an entity under the new name without CP tags", func() {
				id := kongFake.add("routes", map[string]any{"name": "bond--eni-example-v1"})
				route = newRoute("eni-example-v1", "bond")
				Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
				Expect(route.GetRouteId()).To(Equal(id))
			})
		})

		Describe("shared service", func() {
			var serviceID, routeID string

			BeforeEach(func() {
				serviceID = kongFake.add("services", cpEntity("eni-example-v1"))
				own := cpEntity("bond--eni-example-v1")
				own["service"] = map[string]any{"id": serviceID}
				routeID = kongFake.add("routes", own)
			})

			It("blocks the reconcile if another route uses the service found through the parent", func() {
				kongFake.add("routes", map[string]any{"name": "other-route", "service": map[string]any{"id": serviceID}})
				err := client.CreateOrReplaceRoute(ctx, route, upstream)
				Expect(isBlocked(err)).To(BeTrue())
				Expect(err).To(MatchError(ContainSubstring(`route "other-route"`)))
				Expect(kongFake.writes()).To(BeEmpty())
			})

			It("does not delete a shared service", func() {
				kongFake.add("routes", map[string]any{"name": "other-route", "service": map[string]any{"id": serviceID}})
				Expect(client.DeleteRoute(ctx, route)).To(Succeed())
				Expect(kongFake.get("services", serviceID)).NotTo(BeNil())
				Expect(kongFake.get("routes", routeID)).To(BeNil())
			})

			It("adopts an unshared service and checks no more once it is migrated", func() {
				Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
				Expect(route.GetProperty("serviceId")).To(Equal(serviceID))
				Expect(kongFake.calls).To(ContainElement("GET /services/" + serviceID + "/routes"))

				kongFake.resetCalls()
				Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
				Expect(kongFake.calls).NotTo(ContainElement("GET /services/" + serviceID + "/routes"))
			})
		})

		Describe("conflict guard", func() {
			It("blocks a rename if another owned route exists under a candidate name", func() {
				id := kongFake.add("routes", map[string]any{"name": "manually-renamed"})
				route.SetRouteId(id)
				kongFake.add("routes", map[string]any{"name": "bond.eni-example-v1", "tags": anyTags(javaRealmBond)})

				err := client.CreateOrReplaceRoute(ctx, route, upstream)
				Expect(err).To(MatchError(ContainSubstring("both belong to route")))
				Expect(isBlocked(err)).To(BeTrue())
				Expect(kongFake.writes()).To(BeEmpty())
			})

			It("blocks a rename if the pre-migration route and a Java route both exist", func() {
				kongFake.add("routes", cpEntity("eni-example-v1"))
				kongFake.add("routes", map[string]any{"name": "bond.eni-example-v1", "tags": anyTags(javaRealmBond)})

				Expect(isBlocked(client.CreateOrReplaceRoute(ctx, route, upstream))).To(BeTrue())
			})

			It("ignores a route under another candidate name that is not ours", func() {
				id := kongFake.add("routes", cpEntity("eni-example-v1"))
				kongFake.add("routes", map[string]any{"name": "bond-ai.eni-example-v1", "tags": anyTags(javaRealmDefault)})

				Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
				Expect(route.GetRouteId()).To(Equal(id))
			})

			It("skips the check once the route has its new name", func() {
				kongFake.add("routes", cpEntity("bond--eni-example-v1"))
				kongFake.add("routes", map[string]any{"name": "bond.eni-example-v1", "tags": anyTags(javaRealmBond)})

				Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
				Expect(kongFake.calls).NotTo(ContainElement("GET /routes/bond.eni-example-v1"))
			})
		})

		Describe("service", func() {
			It("finds the service by its stored ID", func() {
				id := kongFake.add("services", map[string]any{"name": "manually-renamed"})
				route.SetServiceId(id)
				Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
				Expect(route.GetProperty("serviceId")).To(Equal(id))
				Expect(kongFake.get("services", id)["name"]).To(Equal("bond--eni-example-v1"))
			})

			It("finds the service the found route points to", func() {
				serviceID := kongFake.add("services", map[string]any{"name": "unrelated-name"})
				kongFake.add("routes", map[string]any{"name": "bond--eni-example-v1", "service": map[string]any{"id": serviceID}})
				Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
				Expect(route.GetProperty("serviceId")).To(Equal(serviceID))
			})

			It("finds the service by its CP tags", func() {
				serviceID := kongFake.add("services", cpEntity("old--eni-example-v1"))
				Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
				Expect(route.GetProperty("serviceId")).To(Equal(serviceID))
			})

			It("blocks on a foreign service under the Java name", func() {
				kongFake.add("services", map[string]any{"name": "bond.eni-example-v1", "tags": anyTags("foreign")})
				Expect(isBlocked(client.CreateOrReplaceRoute(ctx, route, upstream))).To(BeTrue())
			})
		})
	})

	Describe("DeleteRoute", func() {
		expectAllDeleted := func() {
			Expect(kongFake.names("routes")).To(BeEmpty())
			Expect(kongFake.names("services")).To(BeEmpty())
			Expect(kongFake.names("upstreams")).To(BeEmpty())
		}

		It("deletes the entities found by stored ID by ID", func() {
			routeID := kongFake.add("routes", map[string]any{"name": "x"})
			serviceID := kongFake.add("services", map[string]any{"name": "y"})
			upstreamID := kongFake.add("upstreams", map[string]any{"name": "z"})
			route.SetRouteId(routeID)
			route.SetServiceId(serviceID)
			route.SetUpstreamId(upstreamID)

			Expect(client.DeleteRoute(ctx, route)).To(Succeed())
			expectAllDeleted()
			Expect(kongFake.writes()).To(ConsistOf("DELETE /routes/"+routeID, "DELETE /services/"+serviceID, "DELETE /upstreams/"+upstreamID))
		})

		It("deletes the entities found by new name", func() {
			kongFake.add("routes", cpEntity("bond--eni-example-v1"))
			kongFake.add("services", cpEntity("bond--eni-example-v1"))
			kongFake.add("upstreams", cpEntity("bond--eni-example-v1"))
			Expect(client.DeleteRoute(ctx, route)).To(Succeed())
			expectAllDeleted()
		})

		It("deletes the entities found by CP tags", func() {
			kongFake.add("routes", cpEntity("old--eni-example-v1"))
			kongFake.add("services", cpEntity("old--eni-example-v1"))
			kongFake.add("upstreams", cpEntity("old--eni-example-v1"))
			Expect(client.DeleteRoute(ctx, route)).To(Succeed())
			expectAllDeleted()
		})

		It("deletes the pre-migration entities", func() {
			kongFake.add("routes", cpEntity("eni-example-v1"))
			kongFake.add("services", cpEntity("eni-example-v1"))
			kongFake.add("upstreams", cpEntity("eni-example-v1"))
			Expect(client.DeleteRoute(ctx, route)).To(Succeed())
			expectAllDeleted()
		})

		It("deletes the Java route and service", func() {
			snapshot := loadSnapshot("bond-api.yaml")
			kongFake.add("services", snapshot.Service)
			kongFake.add("routes", snapshot.Route)
			Expect(client.DeleteRoute(ctx, route)).To(Succeed())
			expectAllDeleted()
			Expect(kongFake.writes()).To(ConsistOf("DELETE /routes/legacy-route-id", "DELETE /services/legacy-service-id"))
		})

		It("succeeds without deleting if nothing is found", func() {
			Expect(client.DeleteRoute(ctx, route)).To(Succeed())
			Expect(kongFake.writes()).To(BeEmpty())
		})

		It("skips a foreign route under metadata.name and succeeds", func() {
			foreignID := kongFake.add("routes", map[string]any{"name": "eni-example-v1", "tags": anyTags(javaRealmDefault)})

			Expect(client.DeleteRoute(ctx, route)).To(Succeed())
			Expect(kongFake.get("routes", foreignID)).NotTo(BeNil())
			Expect(kongFake.writes()).To(BeEmpty())
		})

		It("deletes the owned service but skips a foreign upstream", func() {
			kongFake.add("routes", cpEntity("bond--eni-example-v1"))
			foreignService := kongFake.add("services", map[string]any{"name": "eni-example-v1", "tags": anyTags("foreign")})
			foreignUpstream := kongFake.add("upstreams", map[string]any{"name": "eni-example-v1", "tags": anyTags("foreign")})

			Expect(client.DeleteRoute(ctx, route)).To(Succeed())
			Expect(kongFake.names("routes")).To(BeEmpty())
			Expect(kongFake.get("services", foreignService)).NotTo(BeNil())
			Expect(kongFake.get("upstreams", foreignUpstream)).NotTo(BeNil())
		})

		It("deletes only the entity of the earliest step if a duplicate exists", func() {
			cpID := kongFake.add("routes", cpEntity("eni-example-v1"))
			javaID := kongFake.add("routes", map[string]any{"name": "bond.eni-example-v1", "tags": anyTags(javaRealmBond)})

			Expect(client.DeleteRoute(ctx, route)).To(Succeed())
			Expect(kongFake.get("routes", cpID)).To(BeNil())
			Expect(kongFake.get("routes", javaID)).NotTo(BeNil())
			Expect(kongFake.writes()).To(Equal([]string{"DELETE /routes/" + cpID}))
		})

		It("deletes only the stored-ID route if a legacy duplicate exists", func() {
			storedID := kongFake.add("routes", map[string]any{"name": "manually-renamed"})
			route.SetRouteId(storedID)
			javaID := kongFake.add("routes", map[string]any{"name": "bond.eni-example-v1", "tags": anyTags(javaRealmBond)})

			Expect(client.DeleteRoute(ctx, route)).To(Succeed())
			Expect(kongFake.get("routes", storedID)).To(BeNil())
			Expect(kongFake.get("routes", javaID)).NotTo(BeNil())
		})

		It("skips ambiguous tag matches and succeeds", func() {
			a := kongFake.add("routes", cpEntity("a--eni-example-v1"))
			b := kongFake.add("routes", cpEntity("b--eni-example-v1"))

			Expect(client.DeleteRoute(ctx, route)).To(Succeed())
			Expect(kongFake.get("routes", a)).NotTo(BeNil())
			Expect(kongFake.get("routes", b)).NotTo(BeNil())
		})

		It("still blocks the reconcile in the same situations", func() {
			kongFake.add("routes", map[string]any{"name": "eni-example-v1", "tags": anyTags(javaRealmDefault)})
			Expect(isBlocked(client.CreateOrReplaceRoute(ctx, route, upstream))).To(BeTrue())

			other := newFakeKong()
			client = clientpkg.NewKongClient(other.api(), clientpkg.WithMigrator(snapshotMigrator()))
			other.add("routes", cpEntity("eni-example-v1"))
			other.add("routes", map[string]any{"name": "bond.eni-example-v1", "tags": anyTags(javaRealmBond)})
			Expect(isBlocked(client.CreateOrReplaceRoute(ctx, route, upstream))).To(BeTrue())
		})
	})

	Describe("Upstream", func() {
		upstreamBody := func() *kong.CreateUpstreamJSONRequestBody {
			return &kong.CreateUpstreamJSONRequestBody{
				Name: route.KongName(),
				Tags: &[]string{"env--bond", "upstream--" + route.KongName(), "route--eni-example-v1", "realm--bond"},
			}
		}
		targetBody := func() *kong.CreateTargetForUpstreamJSONRequestBody {
			return &kong.CreateTargetForUpstreamJSONRequestBody{Target: ptr("localhost:8080"), Weight: ptr(100)}
		}

		It("creates the upstream under the realm name and its target by upstream ID", func() {
			Expect(client.CreateOrReplaceUpstream(ctx, route, upstreamBody(), targetBody())).To(Succeed())
			Expect(kongFake.names("upstreams")).To(Equal([]string{"bond--eni-example-v1"}))
			Expect(kongFake.writes()).To(ContainElement("POST /upstreams/" + route.GetUpstreamId() + "/targets"))

			kongFake.resetCalls()
			Expect(client.CreateOrReplaceUpstream(ctx, route, upstreamBody(), targetBody())).To(Succeed())
			Expect(kongFake.writes()).To(BeEmpty())
		})

		It("renames the pre-migration upstream by ID", func() {
			id := kongFake.add("upstreams", cpEntity("eni-example-v1", "upstream--eni-example-v1"))
			Expect(client.CreateOrReplaceUpstream(ctx, route, upstreamBody(), targetBody())).To(Succeed())
			Expect(route.GetUpstreamId()).To(Equal(id))
			Expect(kongFake.get("upstreams", id)["name"]).To(Equal("bond--eni-example-v1"))
			Expect(kongFake.writes()).To(ContainElement("PUT /upstreams/" + id))
		})

		It("finds the upstream by stored ID", func() {
			id := kongFake.add("upstreams", map[string]any{"name": "x"})
			route.SetUpstreamId(id)
			Expect(client.CreateOrReplaceUpstream(ctx, route, upstreamBody(), targetBody())).To(Succeed())
			Expect(route.GetUpstreamId()).To(Equal(id))
		})

		It("does not look up Java candidates for the upstream", func() {
			Expect(client.CreateOrReplaceUpstream(ctx, route, upstreamBody(), targetBody())).To(Succeed())
			Expect(kongFake.calls).NotTo(ContainElement("GET /upstreams/bond.eni-example-v1"))
		})

		It("deletes the located upstream and its target by ID", func() {
			id := kongFake.add("upstreams", cpEntity("bond--eni-example-v1"))
			route.SetTargetsId("t1")
			Expect(client.DeleteUpstream(ctx, route)).To(Succeed())
			Expect(kongFake.writes()).To(Equal([]string{"DELETE /upstreams/" + id, "DELETE /upstreams/" + id + "/targets/t1"}))
		})

		It("skips a foreign upstream without blocking", func() {
			foreign := kongFake.add("upstreams", map[string]any{"name": "eni-example-v1", "tags": anyTags("foreign")})
			Expect(client.DeleteUpstream(ctx, route)).To(Succeed())
			Expect(kongFake.get("upstreams", foreign)).NotTo(BeNil())
		})

		It("deletes nothing if no upstream is found", func() {
			route.SetTargetsId("t1")
			Expect(client.DeleteUpstream(ctx, route)).To(Succeed())
			Expect(kongFake.writes()).To(BeEmpty())
		})
	})

	Describe("Build with the circuit breaker", func() {
		buildWith := func(cb *v1.CircuitBreaker) error {
			route.Spec.Type = v1.RouteTypePrimary
			route.Spec.Traffic.CircuitBreaker = cb
			builder := features.NewFeatureBuilder(client, route, nil, &v1.Gateway{})
			builder.EnableFeature(feature.InstanceCircuitBreakerFeature)
			return builder.Build(ctx)
		}

		Describe("desired name taken by a foreign entity", func() {
			// foreign is an entity of another environment under the desired name.
			foreign := func() map[string]any {
				return map[string]any{"name": "bond--eni-example-v1", "tags": anyTags("env--other", "route--bond--eni-example-v1")}
			}

			// ours adds the pre-migration route and service and stores the route ID, so a rename is pending.
			ours := func() (string, string) {
				serviceID := kongFake.add("services", cpEntity("eni-example-v1"))
				own := cpEntity("eni-example-v1")
				own["service"] = map[string]any{"id": serviceID}
				routeID := kongFake.add("routes", own)
				route.SetRouteId(routeID)
				return routeID, serviceID
			}

			It("blocks the rename of the route without writing", func() {
				ours()
				foreignID := kongFake.add("routes", foreign())

				err := client.CreateOrReplaceRoute(ctx, route, upstream)
				Expect(isBlocked(err)).To(BeTrue())
				Expect(err).To(MatchError(ContainSubstring(`route "bond--eni-example-v1" (` + foreignID + `) is already used by env "other", route "bond--eni-example-v1"; cannot rename "eni-example-v1"`)))
				Expect(kongFake.writes()).To(BeEmpty())
			})

			It("blocks the rename of the service without writing", func() {
				ours()
				kongFake.add("services", foreign())

				err := client.CreateOrReplaceRoute(ctx, route, upstream)
				Expect(isBlocked(err)).To(BeTrue())
				Expect(err).To(MatchError(ContainSubstring(`service "bond--eni-example-v1"`)))
				Expect(kongFake.writes()).To(BeEmpty())
			})

			It("does not rename the upstream if the service name is taken", func() {
				ours()
				upstreamID := kongFake.add("upstreams", cpEntity("eni-example-v1"))
				kongFake.add("services", foreign())

				Expect(isBlocked(buildWith(&v1.CircuitBreaker{Enabled: true}))).To(BeTrue())
				Expect(kongFake.writes()).To(BeEmpty())
				Expect(kongFake.get("upstreams", upstreamID)["name"]).To(Equal("eni-example-v1"))
			})

			It("blocks the rename of the upstream without writing", func() {
				kongFake.add("upstreams", cpEntity("eni-example-v1"))
				kongFake.add("upstreams", foreign())

				err := buildWith(&v1.CircuitBreaker{Enabled: true})
				Expect(isBlocked(err)).To(BeTrue())
				Expect(err).To(MatchError(ContainSubstring(`upstream "bond--eni-example-v1"`)))
				Expect(kongFake.writes()).To(BeEmpty())
			})

			It("deletes our entities and leaves the foreign one", func() {
				routeID, serviceID := ours()
				foreignID := kongFake.add("routes", foreign())

				Expect(client.DeleteRoute(ctx, route)).To(Succeed())
				Expect(kongFake.get("routes", routeID)).To(BeNil())
				Expect(kongFake.get("services", serviceID)).To(BeNil())
				Expect(kongFake.get("routes", foreignID)).NotTo(BeNil())
			})

			It("looks up nothing else once the route is migrated", func() {
				routeID := kongFake.add("routes", cpEntity("bond--eni-example-v1"))
				route.SetRouteId(routeID)

				Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
				var routeReads []string
				for _, call := range kongFake.calls {
					if strings.HasPrefix(call, "GET /routes") {
						routeReads = append(routeReads, call)
					}
				}
				Expect(routeReads).To(Equal([]string{"GET /routes/" + routeID}))
			})
		})

		It("writes nothing if the migration is blocked by a duplicate", func() {
			serviceID := kongFake.add("services", map[string]any{"name": "eni-example-v1", "host": "eni-example-v1", "tags": anyTags("env--bond", "route--eni-example-v1")})
			routeEntity := cpEntity("eni-example-v1")
			routeEntity["service"] = map[string]any{"id": serviceID}
			kongFake.add("routes", routeEntity)
			upstreamID := kongFake.add("upstreams", cpEntity("eni-example-v1", "upstream--eni-example-v1"))
			kongFake.add("routes", map[string]any{"name": "bond.eni-example-v1", "tags": anyTags(javaRealmBond)})

			err := buildWith(&v1.CircuitBreaker{Enabled: true})
			Expect(isBlocked(err)).To(BeTrue())
			Expect(kongFake.writes()).To(BeEmpty())
			Expect(kongFake.get("upstreams", upstreamID)["name"]).To(Equal("eni-example-v1"))
			Expect(kongFake.get("services", serviceID)["host"]).To(Equal("eni-example-v1"))
		})

		It("points the service away from the upstream before deleting it", func() {
			Expect(buildWith(&v1.CircuitBreaker{Enabled: true})).To(Succeed())
			upstreamID := route.GetUpstreamId()
			serviceID := route.GetProperty("serviceId")

			kongFake.resetCalls()
			Expect(buildWith(nil)).To(Succeed())
			writes := kongFake.writes()
			Expect(writes).To(ContainElements("PUT /services/"+serviceID, "DELETE /upstreams/"+upstreamID))
			Expect(slices.Index(writes, "PUT /services/"+serviceID)).To(BeNumerically("<", slices.Index(writes, "DELETE /upstreams/"+upstreamID)))
			Expect(kongFake.get("services", serviceID)["host"]).To(Equal("localhost"))
			Expect(kongFake.names("upstreams")).To(BeEmpty())
			Expect(route.Status.Properties).NotTo(HaveKey("upstreamId"))
			Expect(route.Status.Properties).NotTo(HaveKey("targetsId"))
		})

		DescribeTable("deletes an owned upstream of a disabled CB without stored IDs",
			func(cb *v1.CircuitBreaker) {
				upstreamID := kongFake.add("upstreams", cpEntity("bond--eni-example-v1"))
				Expect(buildWith(cb)).To(Succeed())
				Expect(kongFake.get("upstreams", upstreamID)).To(BeNil())
			},
			Entry("disabled", &v1.CircuitBreaker{Enabled: false}),
			Entry("removed", nil),
		)

		It("keeps a foreign upstream when the CB is off", func() {
			foreignID := kongFake.add("upstreams", map[string]any{"name": "eni-example-v1", "tags": anyTags("foreign")})
			Expect(buildWith(nil)).To(Succeed())
			Expect(kongFake.get("upstreams", foreignID)).NotTo(BeNil())
		})

		It("persists the upstream ID and looks the upstream up by it on the next reconcile", func() {
			route.Spec.Type = v1.RouteTypePrimary
			route.Spec.Traffic.CircuitBreaker = &v1.CircuitBreaker{Enabled: true}
			build := func() {
				builder := features.NewFeatureBuilder(client, route, nil, &v1.Gateway{})
				builder.EnableFeature(feature.InstanceCircuitBreakerFeature)
				Expect(builder.Build(ctx)).To(Succeed())
			}

			build()
			upstreamID := route.GetUpstreamId()
			Expect(upstreamID).NotTo(BeEmpty())
			Expect(route.GetTargetsId()).NotTo(BeEmpty())
			Expect(kongFake.names("upstreams")).To(Equal([]string{"bond--eni-example-v1"}))
			Expect(kongFake.get("services", route.GetProperty("serviceId"))["host"]).To(Equal("bond--eni-example-v1"))

			kongFake.resetCalls()
			build()
			Expect(route.GetUpstreamId()).To(Equal(upstreamID))
			Expect(kongFake.calls).To(ContainElement("GET /upstreams/" + upstreamID))
			Expect(kongFake.calls).NotTo(ContainElement("GET /upstreams/bond--eni-example-v1"))
			Expect(kongFake.writes()).To(BeEmpty())
		})
	})

	Describe("without name prefix", func() {
		BeforeEach(func() {
			route = newRoute("eni-example-v1", "")
		})

		It("keeps names, tags and lookups unchanged", func() {
			kongFake.add("routes", map[string]any{"name": "bond.eni-example-v1", "tags": anyTags(javaRealmBond)})
			reconcileTwice()

			Expect(kongFake.names("routes")).To(ConsistOf("bond.eni-example-v1", "eni-example-v1"))
			Expect(kongFake.names("services")).To(Equal([]string{"eni-example-v1"}))
			Expect(tagsOf(kongFake.get("routes", route.GetRouteId()))).To(ConsistOf(anyTags("env--bond", "route--eni-example-v1")))
			for _, call := range kongFake.calls {
				Expect(call).To(BeElementOf("GET /services/eni-example-v1", "GET /routes/eni-example-v1"))
			}
			Expect(route.GetProperty("legacyName")).To(BeEmpty())
		})

		It("deletes by name as before", func() {
			kongFake.add("routes", cpEntity("eni-example-v1"))
			Expect(client.DeleteRoute(ctx, route)).To(Succeed())
			Expect(kongFake.writes()).To(Equal([]string{"DELETE /routes/eni-example-v1", "DELETE /services/eni-example-v1", "DELETE /upstreams/eni-example-v1"}))
		})
	})
})
