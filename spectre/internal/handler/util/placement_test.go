// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"errors"
	"fmt"

	pkgerrors "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const placementEnv = "test-env"

// makeRoute creates a minimal gateway Route for testing.
func makeRoute(name, namespace, path string) *gatewayv1.Route {
	return &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gatewayv1.RouteSpec{
			Paths: []string{path},
		},
	}
}

// placementClient is a scoped janitor client over a controller-runtime fake
// that records every Get/List and can fail the next EventConfig List.
type placementClient struct {
	gets    []string
	lists   []string
	listErr error
}

func (p *placementClient) context(objs ...client.Object) context.Context {
	sch := runtime.NewScheme()
	Expect(eventv1.AddToScheme(sch)).To(Succeed())
	Expect(pubsubv1.AddToScheme(sch)).To(Succeed())
	for _, o := range objs {
		labels := o.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[cconfig.EnvironmentLabelKey] = placementEnv
		o.SetLabels(labels)
	}
	c := fake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(objs...).
		WithIndex(&eventv1.EventConfig{}, util.EventConfigZoneIndex, func(o client.Object) []string {
			return []string{o.(*eventv1.EventConfig).Spec.Zone.Name}
		}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				p.gets = append(p.gets, fmt.Sprintf("%T %s", obj, key))
				return c.Get(ctx, key, obj, opts...)
			},
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				p.lists = append(p.lists, fmt.Sprintf("%T", list))
				if p.listErr != nil {
					return p.listErr
				}
				return c.List(ctx, list, opts...)
			},
		}).
		Build()
	return cclient.WithClient(context.Background(), cclient.NewJanitorClient(cclient.NewScopedClient(c, placementEnv)))
}

func zoneIn(name, namespace string) *adminv1.Zone {
	return &adminv1.Zone{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
}

// eventConfigFor returns a ready local EventConfig for zone whose status
// references the EventStore es.
func eventConfigFor(zoneName string, es *pubsubv1.EventStore) *eventv1.EventConfig {
	ec := makeReadyEventConfig("ec-"+zoneName, zoneName)
	ec.Status.CallbackURL = "https://" + zoneName + ".example.com/callback"
	ec.Status.EventStore = ctypes.ObjectRefFromObject(es)
	return &ec
}

// proxyEventConfigFor turns eventConfigFor into a proxy zone targeting zone "t".
func proxyEventConfigFor(zoneName string, es *pubsubv1.EventStore) *eventv1.EventConfig {
	ec := eventConfigFor(zoneName, es)
	ec.Spec.Local = nil
	ec.Spec.Proxy = &eventv1.ProxyBackend{TargetZone: ctypes.ObjectRef{Name: "t", Namespace: placementEnv}}
	return ec
}

func storeFor(zoneName string) *pubsubv1.EventStore {
	return makeReadyEventStore("es-"+zoneName, placementEnv+"--"+zoneName)
}

var _ = Describe("CaptureCandidateZones", func() {
	names := func(zones []*adminv1.Zone) []string {
		out := make([]string, 0, len(zones))
		for _, z := range zones {
			out = append(out, z.Name)
		}
		return out
	}

	It("orders C, P and dedups by name and namespace", func() {
		c, p := makeZone("c"), makeZone("p")
		Expect(names(util.CaptureCandidateZones(c, p))).To(Equal([]string{"c", "p"}))

		z := makeZone("z")
		got := util.CaptureCandidateZones(z, makeZone("z"))
		Expect(got).To(HaveLen(1))
		Expect(got[0]).To(BeIdenticalTo(z))
	})

	It("treats the same name in another namespace as distinct", func() {
		got := util.CaptureCandidateZones(makeZone("c"), zoneIn("c", "other-env"))
		Expect(got).To(HaveLen(2))
		Expect(got[0].Namespace).To(Equal("test-env"))
		Expect(got[1].Namespace).To(Equal("other-env"))
	})
})

var _ = Describe("ResolveDelivery", func() {
	var pc *placementClient

	BeforeEach(func() { pc = &placementClient{} })

	It("resolves A's EventConfig and EventStore", func() {
		es := storeFor("a")
		ec := eventConfigFor("a", es)
		ctx := pc.context(ec, es)

		d, err := util.ResolveDelivery(ctx, makeZone("a"))
		Expect(err).ToNot(HaveOccurred())
		Expect(d.Zone.Name).To(Equal("a"))
		Expect(d.EventConfig.Name).To(Equal("ec-a"))
		Expect(d.EventStore.Name).To(Equal("es-a"))
		Expect(d.EventStore.Namespace).To(Equal("test-env--a"))
	})

	It("blocks without fallback when A's zone has no EventConfig", func() {
		es := storeFor("c")
		ctx := pc.context(eventConfigFor("c", es), es)

		d, err := util.ResolveDelivery(ctx, makeZone("a"))
		Expect(d).To(BeNil())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err).To(MatchError(`no EventConfig found for zone "a"`))
	})

	It("blocks when A's EventConfig is NotReady", func() {
		es := storeFor("a")
		ec := eventConfigFor("a", es)
		ec.Status.Conditions = nil
		ctx := pc.context(ec, es)

		d, err := util.ResolveDelivery(ctx, makeZone("a"))
		Expect(d).To(BeNil())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})

	It("blocks with an EnvironmentMismatchError when A's realm differs from A's EventStore environment", func() {
		es := storeFor("a")
		es.Spec.OverwriteEnvironmentName = "custom-env"
		ec := eventConfigFor("a", es)
		ec.Spec.OverwriteEnvironmentName = "custom-env"
		ctx := pc.context(ec, es)

		d, err := util.ResolveDelivery(contextutil.WithEnv(ctx, placementEnv), makeZone("a"))
		Expect(d).To(BeNil())
		var mismatch *util.EnvironmentMismatchError
		Expect(errors.As(err, &mismatch)).To(BeTrue())
		Expect(mismatch.EventConfig).To(Equal("test-env--a/ec-a"))
		Expect(mismatch.EventStore).To(Equal("test-env--a/es-a"))
		Expect(mismatch.Environment).To(Equal(placementEnv + "/" + placementEnv))
	})

	It("resolves with an EnvironmentAlias when A's EventStore subscribes with \"default\" and A's realm differs", func() {
		es := storeFor("a")
		ec := eventConfigFor("a", es)
		ec.Spec.OverwriteEnvironmentName = "default"
		ctx := pc.context(ec, es)
		a := makeZone("a")
		a.Status.RealmName = "custom-env"

		d, err := util.ResolveDelivery(ctx, a)
		Expect(err).ToNot(HaveOccurred())
		Expect(d.EnvironmentAlias).To(Equal(&util.EnvironmentAlias{
			Role: "observer", Zone: "a", PublishEnvironment: "custom-env", EventStore: "test-env--a/es-a",
			SubscribeSource: "EventConfig spec.overwriteEnvironmentName",
		}))
	})

	It("blocks when A's EventConfig has no EventStore reference", func() {
		ec := eventConfigFor("a", storeFor("a"))
		ec.Status.EventStore = nil
		ctx := pc.context(ec)

		d, err := util.ResolveDelivery(ctx, makeZone("a"))
		Expect(d).To(BeNil())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("EventStore"))
	})
})

// deliveryPathReason returns the reason of a *util.DeliveryPathError, which
// must be blocked.
func deliveryPathReason(err error) string {
	var noPath *util.DeliveryPathError
	ExpectWithOffset(1, errors.As(err, &noPath)).To(BeTrue(), "want *util.DeliveryPathError, got %v", err)
	ExpectWithOffset(1, err).To(Satisfy(isBlockedError))
	return noPath.Reason
}

var _ = Describe("ResolveCaptureZone", func() {
	var (
		pc    *placementClient
		route *gatewayv1.Route
	)

	BeforeEach(func() {
		pc = &placementClient{}
		route = makeRoute("route-api-v1", "test-env--c", "/api/v1")
	})

	delivery := func(zone string, ec *eventv1.EventConfig, es *pubsubv1.EventStore) *util.DeliveryPlacement {
		return &util.DeliveryPlacement{Zone: makeZone(zone), EventConfig: ec, EventStore: es}
	}

	It("reuses the delivery EventConfig and EventStore with the local CallbackURL in the same zone", func() {
		es := makeReadyEventStore("es-custom-ns", "custom-namespace")
		ec := eventConfigFor("c", es)
		ctx := pc.context()

		lp, reason, err := util.ResolveCaptureZone(ctx, makeZone("c"), route, delivery("c", ec, es))
		Expect(err).ToNot(HaveOccurred())
		Expect(reason).To(BeEmpty())
		Expect(pc.gets).To(BeEmpty())
		Expect(pc.lists).To(BeEmpty())

		Expect(lp.CaptureZone.Name).To(Equal("c"))
		Expect(lp.DeliveryZone.Name).To(Equal("c"))
		Expect(lp.CallbackOriginZone.Name).To(Equal("c"))
		Expect(lp.CaptureRoute).To(Equal(route))
		Expect(lp.CaptureEventConfig).To(BeIdenticalTo(ec))
		Expect(lp.DeliveryEventConfig).To(BeIdenticalTo(ec))
		Expect(lp.CaptureEventStore).To(BeIdenticalTo(es))
		Expect(lp.DeliveryEventStore).To(BeIdenticalTo(es))
		Expect(lp.BridgeNamespace).To(Equal("custom-namespace"))
		Expect(lp.CallbackBaseURL).To(Equal("https://c.example.com/callback"))
	})

	It("reports a same-zone EventConfig without CallbackURL as a delivery path error", func() {
		es := storeFor("c")
		ec := eventConfigFor("c", es)
		ec.Status.CallbackURL = ""
		ec.Status.ProxyCallbackURLs = map[string]string{"c": "https://c.example.com/proxy/c"}

		lp, reason, err := util.ResolveCaptureZone(pc.context(), makeZone("c"), route, delivery("c", ec, es))
		Expect(lp).To(BeNil())
		Expect(reason).To(BeEmpty())
		Expect(deliveryPathReason(err)).To(ContainSubstring("CallbackURL"))
	})

	It("uses the capture EventConfig's ProxyCallbackURLs entry for the delivery zone for a cross-zone local capture", func() {
		captureES := storeFor("c")
		captureEC := eventConfigFor("c", captureES)
		captureEC.Status.ProxyCallbackURLs = map[string]string{
			"a":     "https://c.example.com/proxy/a",
			"other": "https://c.example.com/proxy/other",
		}
		deliveryES := storeFor("a")
		deliveryEC := eventConfigFor("a", deliveryES)
		deliveryEC.Status.ProxyCallbackURLs = map[string]string{"c": "https://a.example.com/proxy/c"}
		ctx := pc.context(captureEC, captureES)

		lp, reason, err := util.ResolveCaptureZone(ctx, makeZone("c"), route, delivery("a", deliveryEC, deliveryES))
		Expect(err).ToNot(HaveOccurred())
		Expect(reason).To(BeEmpty())

		Expect(lp.CaptureZone.Name).To(Equal("c"))
		Expect(lp.CaptureEventConfig.Name).To(Equal("ec-c"))
		Expect(lp.CaptureEventStore.Name).To(Equal("es-c"))
		Expect(lp.CaptureRoute).To(Equal(route))
		Expect(lp.DeliveryZone.Name).To(Equal("a"))
		Expect(lp.DeliveryEventConfig.Name).To(Equal("ec-a"))
		Expect(lp.DeliveryEventStore.Name).To(Equal("es-a"))
		Expect(lp.CallbackOriginZone.Name).To(Equal("c"))
		Expect(lp.CallbackBaseURL).To(Equal("https://c.example.com/proxy/a"))
		Expect(lp.BridgeNamespace).To(Equal("test-env--c"))
		Expect(lp.CaptureEventStore.Namespace).To(Equal("test-env--c"))
		Expect(lp.DeliveryEventStore.Namespace).To(Equal("test-env--a"))
	})

	It("treats a zone without EventConfig as unsuitable", func() {
		deliveryES := storeFor("a")
		d := delivery("a", eventConfigFor("a", deliveryES), deliveryES)

		lp, reason, err := util.ResolveCaptureZone(pc.context(), makeZone("c"), route, d)
		Expect(err).ToNot(HaveOccurred())
		Expect(lp).To(BeNil())
		Expect(reason).To(ContainSubstring("EventConfig"))
		Expect(reason).To(ContainSubstring(`"c"`))
	})

	It("returns a NotReady capture EventConfig as a transient error, not a reason", func() {
		captureES := storeFor("c")
		captureEC := eventConfigFor("c", captureES)
		captureEC.Status.Conditions = nil
		deliveryES := storeFor("a")
		d := delivery("a", eventConfigFor("a", deliveryES), deliveryES)

		lp, reason, err := util.ResolveCaptureZone(pc.context(captureEC, captureES), makeZone("c"), route, d)
		Expect(lp).To(BeNil())
		Expect(reason).To(BeEmpty())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})

	It("propagates List errors", func() {
		deliveryES := storeFor("a")
		d := delivery("a", eventConfigFor("a", deliveryES), deliveryES)
		pc.listErr = fmt.Errorf("connection refused")

		lp, reason, err := util.ResolveCaptureZone(pc.context(), makeZone("c"), route, d)
		Expect(lp).To(BeNil())
		Expect(reason).To(BeEmpty())
		Expect(err).To(MatchError(ContainSubstring("connection refused")))
	})

	It("returns an error for two EventConfigs in the capture zone", func() {
		captureES := storeFor("c")
		ec1 := eventConfigFor("c", captureES)
		ec2 := eventConfigFor("c", captureES)
		ec2.Name = "ec-c-2"
		deliveryES := storeFor("a")
		d := delivery("a", eventConfigFor("a", deliveryES), deliveryES)

		lp, reason, err := util.ResolveCaptureZone(pc.context(ec1, ec2, captureES), makeZone("c"), route, d)
		Expect(lp).To(BeNil())
		Expect(reason).To(BeEmpty())
		Expect(err).To(MatchError(ContainSubstring("found 2 EventConfigs")))
	})

	It("returns an error for a capture EventConfig without EventStore reference", func() {
		captureEC := eventConfigFor("c", storeFor("c"))
		captureEC.Status.EventStore = nil
		deliveryES := storeFor("a")
		d := delivery("a", eventConfigFor("a", deliveryES), deliveryES)

		lp, reason, err := util.ResolveCaptureZone(pc.context(captureEC), makeZone("c"), route, d)
		Expect(lp).To(BeNil())
		Expect(reason).To(BeEmpty())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("EventStore"))
	})

	It("reports a capture mesh excluding the delivery zone as a delivery path error", func() {
		captureES := storeFor("c")
		captureEC := eventConfigFor("c", captureES)
		captureEC.Spec.Mesh = &eventv1.MeshConfig{FullMesh: false, ZoneNames: []string{"p"}}
		captureEC.Status.ProxyCallbackURLs = map[string]string{"a": "https://c.example.com/proxy/a"}
		deliveryES := storeFor("a")
		deliveryEC := eventConfigFor("a", deliveryES)
		deliveryEC.Status.ProxyCallbackURLs = map[string]string{"c": "https://a.example.com/proxy/c"}

		lp, reason, err := util.ResolveCaptureZone(pc.context(captureEC, captureES), makeZone("c"), route,
			delivery("a", deliveryEC, deliveryES))
		Expect(lp).To(BeNil())
		Expect(reason).To(BeEmpty())
		Expect(deliveryPathReason(err)).To(ContainSubstring("does not mesh with delivery zone"))
		var noPath *util.DeliveryPathError
		Expect(errors.As(err, &noPath)).To(BeTrue())
		Expect(noPath.CaptureZone).To(Equal("c"))
		Expect(noPath.DeliveryZone).To(Equal("a"))
	})

	It("reports nil capture ProxyCallbackURLs as a delivery path error even when A has an entry for the capture zone", func() {
		captureES := storeFor("c")
		deliveryES := storeFor("a")
		deliveryEC := eventConfigFor("a", deliveryES)
		deliveryEC.Status.ProxyCallbackURLs = map[string]string{"c": "https://a.example.com/proxy/c"}

		lp, reason, err := util.ResolveCaptureZone(pc.context(eventConfigFor("c", captureES), captureES), makeZone("c"), route,
			delivery("a", deliveryEC, deliveryES))
		Expect(lp).To(BeNil())
		Expect(reason).To(BeEmpty())
		Expect(deliveryPathReason(err)).To(Equal(`capture EventConfig "ec-c" has no ProxyCallbackURLs entry for delivery zone "a"`))
	})

	It("reports a missing or empty ProxyCallbackURLs key as a delivery path error and never substitutes another", func() {
		captureES := storeFor("c")
		captureEC := eventConfigFor("c", captureES)
		captureEC.Status.ProxyCallbackURLs = map[string]string{
			"p": "https://c.example.com/proxy/p",
			"t": "https://c.example.com/proxy/t",
		}
		deliveryES := storeFor("a")
		deliveryEC := eventConfigFor("a", deliveryES)
		deliveryEC.Status.ProxyCallbackURLs = map[string]string{"c": "https://a.example.com/proxy/c"}

		lp, reason, err := util.ResolveCaptureZone(pc.context(captureEC, captureES), makeZone("c"), route,
			delivery("a", deliveryEC, deliveryES))
		Expect(lp).To(BeNil())
		Expect(reason).To(BeEmpty())
		Expect(deliveryPathReason(err)).To(Equal(`capture EventConfig "ec-c" has no ProxyCallbackURLs entry for delivery zone "a"`))

		emptyEC := eventConfigFor("c", captureES)
		emptyEC.Status.ProxyCallbackURLs = map[string]string{"a": "", "p": "https://c.example.com/proxy/p"}
		lp, reason, err = util.ResolveCaptureZone(pc.context(emptyEC, captureES), makeZone("c"), route,
			delivery("a", deliveryEC, deliveryES))
		Expect(lp).To(BeNil())
		Expect(reason).To(BeEmpty())
		Expect(deliveryPathReason(err)).To(Equal(`capture EventConfig "ec-c" has no ProxyCallbackURLs entry for delivery zone "a"`))
	})

	It("keys a proxy-backed capture by the proxy zone and uses its own EventStore", func() {
		xES := storeFor("x")
		xEC := proxyEventConfigFor("x", xES)
		xEC.Status.ProxyCallbackURLs = map[string]string{
			"d": "https://x.example.com/proxy/d",
			"t": "https://x.example.com/proxy/t",
		}
		tES := storeFor("t")
		tEC := eventConfigFor("t", tES)
		tEC.Status.ProxyCallbackURLs = map[string]string{"d": "https://t.example.com/proxy/d"}
		dES := storeFor("d")
		dEC := eventConfigFor("d", dES)
		dEC.Status.ProxyCallbackURLs = map[string]string{
			"x": "https://d.example.com/proxy/x",
			"t": "https://d.example.com/proxy/t",
		}
		ctx := pc.context(xEC, xES, tEC, tES)

		lp, reason, err := util.ResolveCaptureZone(ctx, makeZone("x"), route, delivery("d", dEC, dES))
		Expect(err).ToNot(HaveOccurred())
		Expect(reason).To(BeEmpty())
		Expect(lp.CallbackOriginZone.Name).To(Equal("x"))
		Expect(lp.CallbackBaseURL).To(Equal("https://x.example.com/proxy/d"))
		Expect(lp.CaptureEventStore.Name).To(Equal("es-x"))
		Expect(lp.CaptureEventStore.Namespace).To(Equal("test-env--x"))
		Expect(lp.BridgeNamespace).To(Equal("test-env--x"))
	})

	It("uses the proxy capture zone's ProxyCallbackURLs entry for its target zone when delivered there", func() {
		xES := storeFor("x")
		xEC := proxyEventConfigFor("x", xES)
		xEC.Status.ProxyCallbackURLs = map[string]string{"t": "https://x.example.com/proxy/t"}
		tES := storeFor("t")
		tEC := eventConfigFor("t", tES)
		tEC.Status.ProxyCallbackURLs = map[string]string{"x": "https://t.example.com/proxy/x"}
		ctx := pc.context(xEC, xES)

		lp, reason, err := util.ResolveCaptureZone(ctx, makeZone("x"), route, delivery("t", tEC, tES))
		Expect(err).ToNot(HaveOccurred())
		Expect(reason).To(BeEmpty())
		Expect(lp.CallbackBaseURL).To(Equal("https://x.example.com/proxy/t"))
		Expect(lp.CallbackBaseURL).ToNot(Equal(tEC.Status.CallbackURL))
		Expect(lp.CaptureEventStore.Name).To(Equal("es-x"))
	})

	It("delivers a local capture into a proxy-backed A through A's own EventStore", func() {
		cES := storeFor("c")
		dES := storeFor("d")
		dEC := proxyEventConfigFor("d", dES)
		dEC.Status.ProxyCallbackURLs = map[string]string{"c": "https://d.example.com/proxy/c"}
		cEC := eventConfigFor("c", cES)
		cEC.Status.ProxyCallbackURLs = map[string]string{"d": "https://c.example.com/proxy/d"}
		ctx := pc.context(cEC, cES)

		lp, reason, err := util.ResolveCaptureZone(ctx, makeZone("c"), route, delivery("d", dEC, dES))
		Expect(err).ToNot(HaveOccurred())
		Expect(reason).To(BeEmpty())
		Expect(lp.DeliveryEventStore.Name).To(Equal("es-d"))
		Expect(lp.CaptureEventStore.Name).To(Equal("es-c"))
		Expect(lp.CallbackOriginZone.Name).To(Equal("c"))
		Expect(lp.CallbackBaseURL).To(Equal("https://c.example.com/proxy/d"))
	})

	It("treats A==C inside a proxy zone as local", func() {
		dES := storeFor("d")
		dEC := proxyEventConfigFor("d", dES)
		dEC.Status.ProxyCallbackURLs = map[string]string{"d": "https://d.example.com/proxy/d"}

		lp, reason, err := util.ResolveCaptureZone(pc.context(), makeZone("d"), route, delivery("d", dEC, dES))
		Expect(err).ToNot(HaveOccurred())
		Expect(reason).To(BeEmpty())
		Expect(lp.CallbackBaseURL).To(Equal("https://d.example.com/callback"))
		Expect(lp.CaptureEventStore).To(BeIdenticalTo(dES))
		Expect(lp.DeliveryEventStore).To(BeIdenticalTo(dES))
	})
})

// chainRoute is one callback Route as the event domain builds it
// (eventconfig handler createCallbackRoutes): a Route on gatewayZone's gateway
// that forwards to upstream, or, for the zone's primary callback Route, makes
// the final DynamicUpstream hop itself (empty upstream).
type chainRoute struct {
	gatewayZone string
	upstream    string
}

// chainURL is the downstream URL of the callback Route for target served by
// gateway's zone: /horizon-<target>/callback/v1 on gateway's hostname.
func chainURL(gateway, target string) string {
	return "https://gw-" + gateway + ".example/horizon-" + target + "/callback/v1"
}

// chainGateways follows base through routes and returns the zones whose
// gateways it traverses, ending at the one making the DynamicUpstream hop.
func chainGateways(routes map[string]chainRoute, base string) []string {
	var zones []string
	for u := base; ; {
		r, ok := routes[u]
		ExpectWithOffset(1, ok).To(BeTrue(), "no callback Route serves %q", u)
		zones = append(zones, r.gatewayZone)
		if r.upstream == "" {
			return zones
		}
		ExpectWithOffset(1, len(zones)).To(BeNumerically("<", 5), "callback Route chain does not terminate")
		u = r.upstream
	}
}

var _ = Describe("ResolveCaptureZone callback Route chain", func() {
	zones := []string{"a", "c", "p", "x", "t"}

	DescribeTable("makes the final DynamicUpstream hop, localhost:8080, on A's gateway",
		func(observer, capture string, want []string) {
			routes := map[string]chainRoute{}
			ecs := map[string]*eventv1.EventConfig{}
			stores := map[string]*pubsubv1.EventStore{}
			objs := make([]client.Object, 0, 2*len(zones))
			for _, s := range zones {
				stores[s] = storeFor(s)
				ec := eventConfigFor(s, stores[s])
				if s == "x" {
					ec = proxyEventConfigFor(s, stores[s])
				}
				// Primary callback Route: localhost:8080 via DynamicUpstream on S's gateway.
				ec.Status.CallbackURL = chainURL(s, s)
				routes[ec.Status.CallbackURL] = chainRoute{gatewayZone: s}
				// Proxy callback Route per peer T: on S's gateway, upstream T's primary.
				ec.Status.ProxyCallbackURLs = map[string]string{}
				for _, t := range zones {
					if t != s {
						ec.Status.ProxyCallbackURLs[t] = chainURL(s, t)
						routes[chainURL(s, t)] = chainRoute{gatewayZone: s, upstream: chainURL(t, t)}
					}
				}
				ecs[s] = ec
				objs = append(objs, ec, stores[s])
			}
			pc := &placementClient{}
			d := &util.DeliveryPlacement{Zone: makeZone(observer), EventConfig: ecs[observer], EventStore: stores[observer]}

			lp, reason, err := util.ResolveCaptureZone(pc.context(objs...), makeZone(capture),
				makeRoute("route-api-v1", "test-env--"+capture, "/api/v1"), d)
			Expect(err).ToNot(HaveOccurred())
			Expect(reason).To(BeEmpty())
			Expect(lp.CallbackOriginZone.Name).To(Equal(capture))
			Expect(chainGateways(routes, lp.CallbackBaseURL)).To(Equal(want))
		},
		Entry("A in a third zone, capture in C's zone", "a", "c", []string{"c", "a"}),
		Entry("A in a third zone, capture in P's zone", "a", "p", []string{"p", "a"}),
		Entry("A in P's zone, capture in C's zone", "p", "c", []string{"c", "p"}),
		Entry("A in C's zone, capture in P's zone", "c", "p", []string{"p", "c"}),
		Entry("A in a third zone, proxy-backed capture zone", "a", "x", []string{"x", "a"}),
		Entry("A in the capture zone", "c", "c", []string{"c"}),
	)
})

var _ = Describe("CallbackOriginZone", func() {
	It("returns the capture zone for its own EventConfig", func() {
		zone := makeZone("x")
		origin, err := util.CallbackOriginZone(zone, proxyEventConfigFor("x", storeFor("x")))
		Expect(err).ToNot(HaveOccurred())
		Expect(origin).To(BeIdenticalTo(zone))
	})

	It("rejects an EventConfig of another zone", func() {
		origin, err := util.CallbackOriginZone(makeZone("x"), eventConfigFor("t", storeFor("t")))
		Expect(origin).To(BeNil())
		Expect(err).To(MatchError(ContainSubstring(`belongs to zone "t"`)))
	})
})

var _ = Describe("NoCaptureCandidateError", func() {
	It("names every zone and reason, is blocked, and is found through wraps", func() {
		var err error = &util.NoCaptureCandidateError{
			ApiBasePath: "/api/v1",
			Rejections: []util.CandidateRejection{
				{Zone: "c", Reason: "no Route"},
				{Zone: "p", Reason: "pass-through"},
			},
		}
		Expect(err.Error()).To(Equal(`no supported capture zone for path "/api/v1": ` +
			`zone "c": no Route; zone "p": pass-through`))
		Expect(err).To(Satisfy(isBlockedError))
		var noPath *util.DeliveryPathError
		Expect(errors.As(err, &noPath)).To(BeFalse())

		wrapped := pkgerrors.Wrap(err, "outer")
		var target *util.NoCaptureCandidateError
		Expect(errors.As(wrapped, &target)).To(BeTrue())
		Expect(target.Rejections).To(HaveLen(2))
		Expect(wrapped).To(Satisfy(isBlockedError))
	})

	It("says the capture zone cannot deliver to the observer zone and exposes the DeliveryPathError", func() {
		noPath := &util.DeliveryPathError{CaptureZone: "p", DeliveryZone: "a", Reason: "mesh excludes a"}
		var err error = &util.NoCaptureCandidateError{
			ApiBasePath:  "/api/v1",
			Rejections:   []util.CandidateRejection{{Zone: "c", Reason: "no Route"}},
			DeliveryPath: noPath,
		}
		Expect(err.Error()).To(Equal(`capture zone "p" for path "/api/v1" cannot deliver to observer zone "a": mesh excludes a; ` +
			`capture is not moved to another zone because Listeners with the same consumer, provider and path capture in one zone ` +
			`(skipped zone "c": no Route)`))
		Expect(err).To(Satisfy(isBlockedError))

		var target *util.DeliveryPathError
		Expect(errors.As(pkgerrors.Wrap(err, "outer"), &target)).To(BeTrue())
		Expect(target).To(BeIdenticalTo(noPath))
	})
})

var _ = Describe("Event environment checks", func() {
	envCtx := contextutil.WithEnv(context.Background(), "cp-env")
	ec := &eventv1.EventConfig{ObjectMeta: metav1.ObjectMeta{Name: "ec-c", Namespace: "env--c"}}
	overwritingEC := func(overwrite string) *eventv1.EventConfig {
		c := ec.DeepCopy()
		c.Spec.OverwriteEnvironmentName = overwrite
		return c
	}
	store := func(overwrite string) *pubsubv1.EventStore {
		return &pubsubv1.EventStore{
			ObjectMeta: metav1.ObjectMeta{Name: "es-c", Namespace: "env--c"},
			Spec:       pubsubv1.EventStoreSpec{OverwriteEnvironmentName: overwrite},
		}
	}
	zone := func(realmName string) *adminv1.Zone {
		return &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "env"},
			Status:     adminv1.ZoneStatus{RealmName: realmName},
		}
	}
	const issuer = "https://iris.example/auth/realms/"

	It("accepts a capture issuer realm equal to the EventStore's overwriteEnvironmentName", func() {
		alias, err := util.CheckCaptureEnvironment(envCtx, zone("default"), issuer+"default", ec, store("default"))
		Expect(err).ToNot(HaveOccurred())
		Expect(alias).To(BeNil())
	})

	It("takes the capture realm after the last realms/ of the issuer, as the Jumper does", func() {
		alias, err := util.CheckCaptureEnvironment(envCtx, zone(""), "https://realms/x/auth/realms/cp-env", ec, store(""))
		Expect(err).ToNot(HaveOccurred())
		Expect(alias).To(BeNil())
	})

	It("falls back to the Control Plane environment when the EventStore has no overwriteEnvironmentName", func() {
		alias, err := util.CheckCaptureEnvironment(envCtx, zone(""), issuer+"cp-env", ec, store(""))
		Expect(err).ToNot(HaveOccurred())
		Expect(alias).To(BeNil())

		_, err = util.CheckCaptureEnvironment(envCtx, zone(""), issuer+"default", ec, store(""))
		Expect(err).To(MatchError(`capture zone "c" publishes events with environment "default" (identity realm of the zone: Environment spec.realmName, else the Environment name) ` +
			`but its EventStore "env--c/es-c" subscribes with environment "cp-env" (Control Plane environment name, copied by the event operator); ` +
			`Galaxy would drop every event; ` +
			`set EventConfig "env--c/ec-c" spec.overwriteEnvironmentName or Environment "cp-env/cp-env" spec.realmName to match`))
	})

	It("reports a capture mismatch naming both values, their sources and the platform config as a blocked EnvironmentMismatchError", func() {
		alias, err := util.CheckCaptureEnvironment(envCtx, zone("default"), issuer+"default", overwritingEC("custom-env"), store("custom-env"))
		Expect(alias).To(BeNil())
		Expect(err).To(MatchError(`capture zone "c" publishes events with environment "default" (identity realm of the zone: Environment spec.realmName, else the Environment name) ` +
			`but its EventStore "env--c/es-c" subscribes with environment "custom-env" (EventConfig spec.overwriteEnvironmentName); Galaxy would drop every event; ` +
			`set EventConfig "env--c/ec-c" spec.overwriteEnvironmentName or Environment "cp-env/cp-env" spec.realmName to match`))
		Expect(err).To(Satisfy(isBlockedError))
		var mismatch *util.EnvironmentMismatchError
		Expect(errors.As(pkgerrors.Wrap(err, "outer"), &mismatch)).To(BeTrue())
	})

	DescribeTable("returns an EnvironmentAlias instead of an error when the EventStore subscribes with \"default\" and the realm differs",
		func(check func() (*util.EnvironmentAlias, error), want string) {
			alias, err := check()
			Expect(err).ToNot(HaveOccurred())
			Expect(alias).ToNot(BeNil())
			Expect(alias.String()).To(Equal(want))
		},
		Entry("capture", func() (*util.EnvironmentAlias, error) {
			return util.CheckCaptureEnvironment(envCtx, zone("default"), issuer+"custom-env", overwritingEC("default"), store("default"))
		}, `capture zone "c" publishes events with environment "custom-env" (identity realm of the zone: Environment spec.realmName, else the Environment name) `+
			`but its EventStore "env--c/es-c" subscribes with environment "default" (EventConfig spec.overwriteEnvironmentName); events are delivered only if the Horizon `+
			`defaultEnvironment of Starlight and Galaxy is "custom-env"`),
		Entry("observer", func() (*util.EnvironmentAlias, error) {
			return util.CheckDeliveryEnvironment(envCtx, zone("custom-env"), overwritingEC("default"), store("default"))
		}, `observer zone "c" publishes events with environment "custom-env" (identity realm of the zone: Environment spec.realmName, else the Environment name) `+
			`but its EventStore "env--c/es-c" subscribes with environment "default" (EventConfig spec.overwriteEnvironmentName); events are delivered only if the Horizon `+
			`defaultEnvironment of Starlight and Galaxy is "custom-env"`),
	)

	It("accepts an observer zone RealmName equal to the EventStore environment", func() {
		for _, c := range []struct{ realm, overwrite string }{{"default", "default"}, {"cp-env", ""}} {
			alias, err := util.CheckDeliveryEnvironment(envCtx, zone(c.realm), ec, store(c.overwrite))
			Expect(err).ToNot(HaveOccurred())
			Expect(alias).To(BeNil())
		}
	})

	It("reports an observer mismatch naming the realm, the EventStore environment and the platform config", func() {
		_, err := util.CheckDeliveryEnvironment(envCtx, zone("default"), overwritingEC("custom-env"), store("custom-env"))
		Expect(err).To(MatchError(`observer zone "c" publishes events with environment "default" (identity realm of the zone: Environment spec.realmName, else the Environment name) ` +
			`but its EventStore "env--c/es-c" subscribes with environment "custom-env" (EventConfig spec.overwriteEnvironmentName); Galaxy would drop every event; ` +
			`set EventConfig "env--c/ec-c" spec.overwriteEnvironmentName or Environment "cp-env/cp-env" spec.realmName to match`))
		var mismatch *util.EnvironmentMismatchError
		Expect(errors.As(err, &mismatch)).To(BeTrue())
	})

	It("names the value the EventConfig resolves to while the event operator has not yet updated the EventStore", func() {
		_, err := util.CheckDeliveryEnvironment(envCtx, zone("default"), overwritingEC("default"), store("custom-env"))
		Expect(err).To(MatchError(`observer zone "c" publishes events with environment "default" (identity realm of the zone: Environment spec.realmName, else the Environment name) ` +
			`but its EventStore "env--c/es-c" subscribes with environment "custom-env" ` +
			`(EventStore not yet updated by the event operator, EventConfig resolves to "default"); Galaxy would drop every event; ` +
			`set EventConfig "env--c/ec-c" spec.overwriteEnvironmentName or Environment "cp-env/cp-env" spec.realmName to match`))

		alias, err := util.CheckCaptureEnvironment(envCtx, zone(""), issuer+"custom-env", ec, store("default"))
		Expect(err).ToNot(HaveOccurred())
		Expect(alias.SubscribeSource).To(Equal(`EventStore not yet updated by the event operator, EventConfig resolves to "cp-env"`))
	})

	It("blocks without a mismatch while the observer zone has no RealmName yet", func() {
		_, err := util.CheckDeliveryEnvironment(envCtx, zone(""), ec, store("default"))
		Expect(err).To(MatchError(`zone "c" has no RealmName in status`))
		Expect(err).To(Satisfy(isBlockedError))
		var mismatch *util.EnvironmentMismatchError
		Expect(errors.As(err, &mismatch)).To(BeFalse())
	})
})
