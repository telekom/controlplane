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

	It("orders A, C, P and dedups by name and namespace", func() {
		c, p := makeZone("c"), makeZone("p")

		got, rejections := util.CaptureCandidateZones(makeZone("c"), c, p)
		Expect(names(got)).To(Equal([]string{"c", "p"}))
		Expect(rejections).To(BeEmpty())

		got, rejections = util.CaptureCandidateZones(makeZone("p"), c, p)
		Expect(names(got)).To(Equal([]string{"p", "c"}))
		Expect(rejections).To(BeEmpty())

		z := makeZone("z")
		got, rejections = util.CaptureCandidateZones(z, makeZone("z"), makeZone("z"))
		Expect(got).To(HaveLen(1))
		Expect(got[0]).To(BeIdenticalTo(z))
		Expect(rejections).To(BeEmpty())
	})

	It("rejects an observer zone that is neither C nor P", func() {
		got, rejections := util.CaptureCandidateZones(makeZone("a3"), makeZone("c"), makeZone("p"))
		Expect(names(got)).To(Equal([]string{"c", "p"}))
		Expect(rejections).To(HaveLen(1))
		Expect(rejections[0].Zone).To(Equal("a3"))
		Expect(rejections[0].Reason).To(ContainSubstring("not on the consumer→provider traffic path"))
	})

	It("treats the same name in another namespace as distinct", func() {
		got, rejections := util.CaptureCandidateZones(zoneIn("c", "other-env"), makeZone("c"), makeZone("p"))
		Expect(got).To(HaveLen(2))
		Expect(got[0].Namespace).To(Equal("test-env"))
		Expect(names(got)).To(Equal([]string{"c", "p"}))
		Expect(rejections).To(HaveLen(1))
		Expect(rejections[0].Zone).To(Equal("c"))

		got, _ = util.CaptureCandidateZones(makeZone("c"), makeZone("c"), zoneIn("c", "other-env"))
		Expect(got).To(HaveLen(2))
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
		Expect(err.Error()).To(ContainSubstring("delivery EventConfig"))
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

	It("treats a same-zone EventConfig without CallbackURL as unsuitable", func() {
		es := storeFor("c")
		ec := eventConfigFor("c", es)
		ec.Status.CallbackURL = ""
		ec.Status.ProxyCallbackURLs = map[string]string{"c": "https://c.example.com/proxy/c"}

		lp, reason, err := util.ResolveCaptureZone(pc.context(), makeZone("c"), route, delivery("c", ec, es))
		Expect(err).ToNot(HaveOccurred())
		Expect(lp).To(BeNil())
		Expect(reason).To(ContainSubstring("CallbackURL"))
	})

	It("uses A's ProxyCallbackURLs entry for a cross-zone local capture", func() {
		captureES := storeFor("c")
		captureEC := eventConfigFor("c", captureES)
		deliveryES := storeFor("a")
		deliveryEC := eventConfigFor("a", deliveryES)
		deliveryEC.Status.ProxyCallbackURLs = map[string]string{
			"c":     "https://a.example.com/proxy/c",
			"other": "https://a.example.com/proxy/other",
		}
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
		Expect(lp.CallbackBaseURL).To(Equal("https://a.example.com/proxy/c"))
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

	It("treats a capture mesh excluding the delivery zone as unsuitable", func() {
		captureES := storeFor("c")
		captureEC := eventConfigFor("c", captureES)
		captureEC.Spec.Mesh = &eventv1.MeshConfig{FullMesh: false, ZoneNames: []string{"p"}}
		deliveryES := storeFor("a")
		deliveryEC := eventConfigFor("a", deliveryES)
		deliveryEC.Status.ProxyCallbackURLs = map[string]string{"c": "https://a.example.com/proxy/c"}

		lp, reason, err := util.ResolveCaptureZone(pc.context(captureEC, captureES), makeZone("c"), route,
			delivery("a", deliveryEC, deliveryES))
		Expect(err).ToNot(HaveOccurred())
		Expect(lp).To(BeNil())
		Expect(reason).To(ContainSubstring("does not mesh with delivery zone"))
	})

	It("treats nil ProxyCallbackURLs as unsuitable", func() {
		captureES := storeFor("c")
		deliveryES := storeFor("a")
		d := delivery("a", eventConfigFor("a", deliveryES), deliveryES)

		lp, reason, err := util.ResolveCaptureZone(pc.context(eventConfigFor("c", captureES), captureES), makeZone("c"), route, d)
		Expect(err).ToNot(HaveOccurred())
		Expect(lp).To(BeNil())
		Expect(reason).To(ContainSubstring("ProxyCallbackURLs"))
	})

	It("treats a missing or empty ProxyCallbackURLs key as unsuitable and never substitutes another", func() {
		captureES := storeFor("c")
		deliveryES := storeFor("a")
		deliveryEC := eventConfigFor("a", deliveryES)
		deliveryEC.Status.ProxyCallbackURLs = map[string]string{
			"p": "https://a.example.com/proxy/p",
			"t": "https://a.example.com/proxy/t",
		}
		ctx := pc.context(eventConfigFor("c", captureES), captureES)

		lp, reason, err := util.ResolveCaptureZone(ctx, makeZone("c"), route, delivery("a", deliveryEC, deliveryES))
		Expect(err).ToNot(HaveOccurred())
		Expect(lp).To(BeNil())
		Expect(reason).To(ContainSubstring("proxy callback"))
		Expect(reason).To(ContainSubstring(`"c"`))

		deliveryEC.Status.ProxyCallbackURLs["c"] = ""
		lp, reason, err = util.ResolveCaptureZone(ctx, makeZone("c"), route, delivery("a", deliveryEC, deliveryES))
		Expect(err).ToNot(HaveOccurred())
		Expect(lp).To(BeNil())
		Expect(reason).To(ContainSubstring("proxy callback"))
	})

	It("keys a proxy-backed capture by the proxy zone and uses its own EventStore", func() {
		xES := storeFor("x")
		xEC := proxyEventConfigFor("x", xES)
		tES := storeFor("t")
		tEC := eventConfigFor("t", tES)
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
		Expect(lp.CallbackBaseURL).To(Equal("https://d.example.com/proxy/x"))
		Expect(lp.CaptureEventStore.Name).To(Equal("es-x"))
		Expect(lp.CaptureEventStore.Namespace).To(Equal("test-env--x"))
		Expect(lp.BridgeNamespace).To(Equal("test-env--x"))
	})

	It("uses the target zone's ProxyCallbackURLs entry for a proxy capture delivered in its target zone", func() {
		xES := storeFor("x")
		xEC := proxyEventConfigFor("x", xES)
		tES := storeFor("t")
		tEC := eventConfigFor("t", tES)
		tEC.Status.ProxyCallbackURLs = map[string]string{"x": "https://t.example.com/proxy/x"}
		ctx := pc.context(xEC, xES)

		lp, reason, err := util.ResolveCaptureZone(ctx, makeZone("x"), route, delivery("t", tEC, tES))
		Expect(err).ToNot(HaveOccurred())
		Expect(reason).To(BeEmpty())
		Expect(lp.CallbackBaseURL).To(Equal("https://t.example.com/proxy/x"))
		Expect(lp.CallbackBaseURL).ToNot(Equal(tEC.Status.CallbackURL))
		Expect(lp.CaptureEventStore.Name).To(Equal("es-x"))
	})

	It("delivers a local capture into a proxy-backed A through A's own EventStore", func() {
		cES := storeFor("c")
		dES := storeFor("d")
		dEC := proxyEventConfigFor("d", dES)
		dEC.Status.ProxyCallbackURLs = map[string]string{"c": "https://d.example.com/proxy/c"}
		ctx := pc.context(eventConfigFor("c", cES), cES)

		lp, reason, err := util.ResolveCaptureZone(ctx, makeZone("c"), route, delivery("d", dEC, dES))
		Expect(err).ToNot(HaveOccurred())
		Expect(reason).To(BeEmpty())
		Expect(lp.DeliveryEventStore.Name).To(Equal("es-d"))
		Expect(lp.CaptureEventStore.Name).To(Equal("es-c"))
		Expect(lp.CallbackOriginZone.Name).To(Equal("c"))
		Expect(lp.CallbackBaseURL).To(Equal("https://d.example.com/proxy/c"))
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
				{Zone: "a3", Reason: "off path"},
				{Zone: "c", Reason: "no Route"},
				{Zone: "p", Reason: "pass-through"},
			},
		}
		Expect(err.Error()).To(Equal(`no supported capture zone for path "/api/v1": ` +
			`zone "a3": off path; zone "c": no Route; zone "p": pass-through`))
		Expect(err).To(Satisfy(isBlockedError))

		wrapped := pkgerrors.Wrap(err, "outer")
		var target *util.NoCaptureCandidateError
		Expect(errors.As(wrapped, &target)).To(BeTrue())
		Expect(target.Rejections).To(HaveLen(3))
		Expect(wrapped).To(Satisfy(isBlockedError))
	})
})
