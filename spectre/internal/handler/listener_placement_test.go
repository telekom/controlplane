// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"
	"fmt"
	"net/url"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"
	"github.com/telekom/controlplane/spectre/internal/handler/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Placement specs drive the production Listener controller over a fake API
// server with a C (consumer), P (provider), A (observer) topology spread over
// the zones c, p, a3 and t (proxy target).

const (
	plEnv         = "pl-env"
	plTeamNs      = "pl-team"
	plPath        = "/api/v1/orders"
	plAppId       = "team-a--a-app"
	plExposureUID = "pl-exposure-uid"
	plListener    = "pl-listener"
)

var plZones = []string{"c", "p", "a3", "t"}

func plZoneNs(zone string) string { return plEnv + "--" + zone }

func plReady() []metav1.Condition {
	return []metav1.Condition{{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready"}}
}

func plZoneRef(zone string) ctypes.ObjectRef { return ctypes.ObjectRef{Name: zone, Namespace: plEnv} }

func plApp(name, team, zone string) *applicationv1.Application {
	return &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: plTeamNs, UID: k8stypes.UID(name + "-uid")},
		Spec:       applicationv1.ApplicationSpec{Team: team, TeamEmail: team + "@test.com", Zone: plZoneRef(zone)},
		Status:     applicationv1.ApplicationStatus{ClientId: team + "--" + name, Conditions: plReady()},
	}
}

func plAppRef(name string) ctypes.TypedObjectRef {
	return ctypes.TypedObjectRef{
		TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: applicationv1.GroupVersion.String()},
		ObjectRef: ctypes.ObjectRef{Name: name, Namespace: plTeamNs},
	}
}

func plRoute(zone string) *gatewayv1.Route {
	return &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.MakeRouteName(plPath),
			Namespace: plZoneNs(zone),
			Labels:    map[string]string{cconfig.OwnerUidLabelKey: plExposureUID},
		},
		Spec: gatewayv1.RouteSpec{Paths: []string{plPath}},
	}
}

func plRouteRef(zone string) ctypes.ObjectRef {
	return ctypes.ObjectRef{Name: util.MakeRouteName(plPath), Namespace: plZoneNs(zone)}
}

// plFixtures is the topology a spec starts from. Specs mutate it before the
// harness is built; a nil map entry is not created.
type plFixtures struct {
	zones    map[string]*adminv1.Zone
	realms   map[string]*identityv1.Realm
	ecs      map[string]*eventv1.EventConfig
	stores   map[string]*pubsubv1.EventStore
	routes   map[string]*gatewayv1.Route
	apps     []*applicationv1.Application
	provider *applicationv1.Application
	exposure *apiv1.ApiExposure
	sa       *spectrev1.SpectreApplication
	listener *spectrev1.Listener
}

// newPlFixtures places C in c, P in p (exposure zone, primary route) with a
// proxy route for c, and A in observerZone.
func newPlFixtures(observerZone string) *plFixtures {
	f := &plFixtures{
		zones:  map[string]*adminv1.Zone{},
		realms: map[string]*identityv1.Realm{},
		ecs:    map[string]*eventv1.EventConfig{},
		stores: map[string]*pubsubv1.EventStore{},
		routes: map[string]*gatewayv1.Route{"p": plRoute("p"), "c": plRoute("c")},
	}
	for _, z := range plZones {
		f.zones[z] = &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{Name: z, Namespace: plEnv},
			Status: adminv1.ZoneStatus{
				Namespace:     plZoneNs(z),
				IdentityRealm: &ctypes.ObjectRef{Name: "realm-" + z, Namespace: plEnv},
				Conditions:    plReady(),
			},
		}
		f.realms[z] = &identityv1.Realm{
			ObjectMeta: metav1.ObjectMeta{Name: "realm-" + z, Namespace: plEnv},
			Status:     identityv1.RealmStatus{IssuerUrl: "https://iris." + z + ".example/auth/realms/default"},
		}
		f.stores[z] = &pubsubv1.EventStore{
			ObjectMeta: metav1.ObjectMeta{Name: "es-" + z, Namespace: plZoneNs(z)},
			Spec:       pubsubv1.EventStoreSpec{Url: "http://" + z + ".admin", TokenUrl: "http://" + z + ".token", ClientId: "id", ClientSecret: "secret"},
			Status:     pubsubv1.EventStoreStatus{Conditions: plReady()},
		}
		proxies := map[string]string{}
		for _, other := range plZones {
			if other != z {
				proxies[other] = "https://" + z + ".gw.example/proxy/" + other
			}
		}
		f.ecs[z] = &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-" + z, Namespace: plZoneNs(z)},
			Spec: eventv1.EventConfigSpec{
				Zone: plZoneRef(z),
				Local: &eventv1.LocalBackend{
					Admin:              eventv1.AdminConfig{Url: "http://" + z + ".admin"},
					ServerSendEventUrl: "https://" + z + ".sse.internal/sse",
					PublishEventUrl:    "http://" + z + ".publish",
				},
			},
			Status: eventv1.EventConfigStatus{
				CallbackURL:       "https://" + z + ".gw.example/callback",
				ProxyCallbackURLs: proxies,
				EventStore:        &ctypes.ObjectRef{Name: "es-" + z, Namespace: plZoneNs(z)},
				Conditions:        plReady(),
			},
		}
	}

	consumer := plApp("c-app", "team-c", "c")
	f.provider = plApp("p-app", "team-p", "p")
	observer := plApp("a-app", "team-a", observerZone)
	f.apps = []*applicationv1.Application{consumer, f.provider, observer}

	f.exposure = &apiv1.ApiExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p-app--api-v1-orders",
			Namespace: plTeamNs,
			UID:       plExposureUID,
			Labels:    map[string]string{cconfig.BuildLabelKey("application"): "p-app"},
		},
		Spec: apiv1.ApiExposureSpec{ApiBasePath: plPath, Zone: plZoneRef("p")},
		Status: apiv1.ApiExposureStatus{
			Active:      true,
			Route:       new(plRouteRef("p")),
			ProxyRoutes: []ctypes.ObjectRef{plRouteRef("c")},
		},
	}
	f.sa = &spectrev1.SpectreApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "pl-sa", Namespace: plTeamNs, UID: "pl-sa-uid"},
		Spec:       spectrev1.SpectreApplicationSpec{Application: plAppRef("a-app")},
		Status:     spectrev1.SpectreApplicationStatus{Id: plAppId},
	}
	f.listener = &spectrev1.Listener{
		ObjectMeta: metav1.ObjectMeta{Name: plListener, Namespace: plTeamNs, UID: "pl-listener-uid"},
		Spec: spectrev1.ListenerSpec{
			Consumer:    plAppRef("c-app"),
			Provider:    plAppRef("p-app"),
			Application: ctypes.ObjectRef{Name: "pl-sa", Namespace: plTeamNs},
			ApiListener: &spectrev1.ApiListener{ApiBasePath: plPath},
		},
	}
	return f
}

// proxy turns zone's EventConfig into a proxy zone forwarding to target. The
// zone keeps its own EventStore CR, as the event domain creates it.
func (f *plFixtures) proxy(zone, target string) {
	ec := f.ecs[zone]
	ec.Spec.Local = nil
	ec.Spec.Proxy = &eventv1.ProxyBackend{TargetZone: plZoneRef(target)}
}

func (f *plFixtures) objects() []client.Object {
	var objs []client.Object
	add := func(o client.Object) {
		if reflect.ValueOf(o).IsNil() {
			return
		}
		labels := o.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[cconfig.EnvironmentLabelKey] = plEnv
		o.SetLabels(labels)
		objs = append(objs, o)
	}
	for _, z := range plZones {
		add(f.zones[z])
		add(f.realms[z])
		add(f.ecs[z])
		add(f.stores[z])
		add(f.routes[z])
	}
	for _, a := range f.apps {
		add(a)
	}
	add(f.exposure)
	add(f.sa)
	add(f.listener)
	return objs
}

type plCall struct{ verb, kind, ns, name, field string }

// plHarness runs the Listener controller over a fake client and records every
// client call a reconcile makes.
type plHarness struct {
	raw       client.Client
	ctrl      cc.Controller[*spectrev1.Listener]
	calls     []plCall
	recording bool
	uids      int
	// listErr, when set, fails a List before it reaches the fake client.
	listErr func(list client.ObjectList) error
}

func plKind(o any) string { return reflect.TypeOf(o).Elem().Name() }

func newPlHarness(f *plFixtures) *plHarness {
	h := &plHarness{}
	sch := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		spectrev1.AddToScheme, applicationv1.AddToScheme, adminv1.AddToScheme, approvalv1.AddToScheme,
		apiv1.AddToScheme, eventv1.AddToScheme, gatewayv1.AddToScheme, pubsubv1.AddToScheme, identityv1.AddToScheme,
	} {
		Expect(add(sch)).To(Succeed())
	}
	rec := func(c plCall) {
		if h.recording {
			h.calls = append(h.calls, c)
		}
	}
	h.raw = fake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(f.objects()...).
		WithStatusSubresource(&spectrev1.Listener{}).
		WithIndex(&eventv1.EventConfig{}, util.EventConfigZoneIndex, func(o client.Object) []string {
			return []string{o.(*eventv1.EventConfig).Spec.Zone.Name}
		}).
		WithIndex(&approvalv1.ApprovalRequest{}, index.ControllerIndexKey, func(o client.Object) []string {
			if owner := metav1.GetControllerOf(o); owner != nil {
				return []string{string(owner.UID)}
			}
			return nil
		}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				rec(plCall{verb: "Get", kind: plKind(obj), ns: key.Namespace, name: key.Name})
				return c.Get(ctx, key, obj, opts...)
			},
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				lo := (&client.ListOptions{}).ApplyOptions(opts)
				field := ""
				if lo.FieldSelector != nil {
					field = lo.FieldSelector.String()
				}
				rec(plCall{verb: "List", kind: plKind(list), ns: lo.Namespace, field: field})
				if h.listErr != nil {
					if err := h.listErr(list); err != nil {
						return err
					}
				}
				return c.List(ctx, list, opts...)
			},
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if obj.GetUID() == "" {
					h.uids++
					obj.SetUID(k8stypes.UID(fmt.Sprintf("pl-uid-%d", h.uids)))
				}
				rec(plCall{verb: "Create", kind: plKind(obj), ns: obj.GetNamespace(), name: obj.GetName()})
				return c.Create(ctx, obj, opts...)
			},
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				rec(plCall{verb: "Update", kind: plKind(obj), ns: obj.GetNamespace(), name: obj.GetName()})
				return c.Update(ctx, obj, opts...)
			},
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				rec(plCall{verb: "Delete", kind: plKind(obj), ns: obj.GetNamespace(), name: obj.GetName()})
				return c.Delete(ctx, obj, opts...)
			},
		}).
		Build()
	h.ctrl = cc.NewController(&handler.ListenerHandler{}, h.raw, &record.FakeRecorder{})
	return h
}

// reconcile runs one controller pass on the persisted Listener and returns the
// calls it made.
func (h *plHarness) reconcile() ([]plCall, error) {
	h.calls = nil
	h.recording = true
	defer func() { h.recording = false }()
	nn := k8stypes.NamespacedName{Name: plListener, Namespace: plTeamNs}
	_, err := h.ctrl.Reconcile(context.Background(), reconcile.Request{NamespacedName: nn}, &spectrev1.Listener{})
	return h.calls, err
}

func (h *plHarness) mustReconcile() []plCall {
	calls, err := h.reconcile()
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return calls
}

// startup runs the finalizer pass and the first handler pass and returns all
// calls they made.
func (h *plHarness) startup() []plCall {
	first := h.mustReconcile()
	return append(first, h.mustReconcile()...)
}

func (h *plHarness) listener() *spectrev1.Listener {
	l := &spectrev1.Listener{}
	ExpectWithOffset(1, h.raw.Get(context.Background(), k8stypes.NamespacedName{Name: plListener, Namespace: plTeamNs}, l)).To(Succeed())
	return l
}

// grant simulates the approval controller: both scoped Approvals are Granted,
// controlled by the Listener and bound to the Listener's current requests.
func (h *plHarness) grant() {
	ctx := context.Background()
	l := h.listener()
	for _, ref := range []*ctypes.ObjectRef{l.Status.ProviderApprovalRequest, l.Status.ConsumerApprovalRequest} {
		ExpectWithOffset(1, ref).ToNot(BeNil())
		ar := &approvalv1.ApprovalRequest{}
		ExpectWithOffset(1, h.raw.Get(ctx, ref.K8s(), ar)).To(Succeed())
		name, err := approvalv1.ScopedApprovalName(ar.Spec.Target, ar.Spec.ApprovalKey)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		isController := true
		approval := &approvalv1.Approval{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ar.Namespace}}
		_, err = controllerutil.CreateOrUpdate(ctx, h.raw, approval, func() error {
			approval.Labels = map[string]string{cconfig.EnvironmentLabelKey: plEnv}
			approval.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: ar.Spec.Target.APIVersion, Kind: ar.Spec.Target.Kind, Name: ar.Spec.Target.Name,
				UID: ar.Spec.Target.UID, Controller: &isController, BlockOwnerDeletion: &isController,
			}}
			approval.Spec = approvalv1.ApprovalSpec{
				Action:          ar.Spec.Action,
				Target:          ar.Spec.Target,
				Requester:       ar.Spec.Requester,
				Decider:         ar.Spec.Decider,
				Strategy:        ar.Spec.Strategy,
				ApprovalKey:     ar.Spec.ApprovalKey,
				State:           approvalv1.ApprovalStateGranted,
				Decisions:       []approvalv1.Decision{{Name: "System", ResultingState: approvalv1.ApprovalStateGranted}},
				ApprovedRequest: &ctypes.ObjectRef{Name: ar.Name, Namespace: ar.Namespace, UID: ar.UID},
			}
			return nil
		})
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
	}
}

// provision runs the finalizer pass, the pending pass, grants both gates and
// runs the provisioning pass. It returns every call those passes made.
func (h *plHarness) provision() (*spectrev1.Listener, []plCall) {
	all := h.startup()
	l := h.listener()
	ExpectWithOffset(1, l.Status.AppliedPlacement).To(BeNil())
	ExpectWithOffset(1, l.Status.ProviderApprovalRequest).ToNot(BeNil())
	h.grant()
	all = append(all, h.mustReconcile()...)
	l = h.listener()
	ExpectWithOffset(1, l.Status.AppliedPlacement).ToNot(BeNil(), "not provisioned: %v", l.Status.Conditions)
	ExpectWithOffset(1, l.Status.RouteListener).ToNot(BeNil())
	ExpectWithOffset(1, l.Status.EventSubscriptions).To(HaveLen(2))
	return l, all
}

func (h *plHarness) update(obj client.Object) {
	ExpectWithOffset(1, h.raw.Update(context.Background(), obj)).To(Succeed())
}

func (h *plHarness) exists(ref *ctypes.ObjectRef, obj client.Object) bool {
	err := h.raw.Get(context.Background(), ref.K8s(), obj)
	if apierrors.IsNotFound(err) {
		return false
	}
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return true
}

// plCount returns the calls with verb and kind, in ns when ns is non-empty.
func plCount(calls []plCall, verb, kind, ns string) int {
	n := 0
	for _, c := range calls {
		if c.verb == verb && c.kind == kind && (ns == "" || c.ns == ns) {
			n++
		}
	}
	return n
}

// plCountVerb returns the number of calls with verb.
func plCountVerb(calls []plCall, verb string) int {
	n := 0
	for _, c := range calls {
		if c.verb == verb {
			n++
		}
	}
	return n
}

// plECListed reports whether an EventConfig lookup for zone was made.
func plECListed(calls []plCall, zone string) bool {
	for _, c := range calls {
		if c.verb == "List" && c.kind == "EventConfigList" && c.field == util.EventConfigZoneIndex+"="+zone {
			return true
		}
	}
	return false
}

// plExpectNoCaptureWrites asserts that no capture child was created, updated
// or deleted.
func plExpectNoCaptureWrites(calls []plCall) {
	for _, kind := range []string{"RouteListener", "Subscriber", "Publisher"} {
		for _, verb := range []string{"Create", "Update", "Delete"} {
			ExpectWithOffset(1, plCount(calls, verb, kind, "")).To(BeZero(), "%s %s", verb, kind)
		}
	}
}

func plBlocked(l *spectrev1.Listener) string {
	c := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeProcessing)
	ExpectWithOffset(1, c).ToNot(BeNil())
	ExpectWithOffset(1, c.Reason).To(Equal(condition.ReasonBlocked))
	return c.Message
}

func plExpectPlacement(l *spectrev1.Listener, capture, delivery, origin, base string) {
	ap := l.Status.AppliedPlacement
	ExpectWithOffset(1, ap).ToNot(BeNil())
	ExpectWithOffset(1, ap.CaptureZone.Name).To(Equal(capture))
	ExpectWithOffset(1, ap.DeliveryZone.Name).To(Equal(delivery))
	ExpectWithOffset(1, ap.CallbackOriginZone.Name).To(Equal(origin))
	ExpectWithOffset(1, ap.CallbackBaseURL).To(Equal(base))
	ExpectWithOffset(1, ap.CaptureRoute.Namespace).To(Equal(plZoneNs(capture)))
	ExpectWithOffset(1, l.Status.RouteListener.Namespace).To(Equal(plZoneNs(capture)))
}

var _ = Describe("Listener capture placement (controller)", func() {
	It("H1: A at C's zone captures locally", func() {
		f := newPlFixtures("c")
		h := newPlHarness(f)
		l, calls := h.provision()

		plExpectPlacement(l, "c", "c", "c", f.ecs["c"].Status.CallbackURL)
		Expect(plCount(calls, "Get", "Route", plZoneNs("p"))).To(BeZero())
		rl := &gatewayv1.RouteListener{}
		Expect(h.exists(l.Status.RouteListener, rl)).To(BeTrue())
		Expect(rl.Spec.Route).To(Equal(plRouteRef("c")))
		Expect(rl.Spec.Zone).To(Equal(plZoneRef("c")))
	})

	It("H2: A at P's zone with C elsewhere prefers A's zone", func() {
		f := newPlFixtures("p")
		h := newPlHarness(f)
		l, calls := h.provision()

		plExpectPlacement(l, "p", "p", "p", f.ecs["p"].Status.CallbackURL)
		Expect(plCount(calls, "Get", "Route", plZoneNs("c"))).To(BeZero())
		rl := &gatewayv1.RouteListener{}
		Expect(h.exists(l.Status.RouteListener, rl)).To(BeTrue())
		Expect(rl.Spec.Route).To(Equal(plRouteRef("p")))
	})

	It("H3: A off-path in a third zone captures per order and delivers at A through the proxy callback", func() {
		f := newPlFixtures("a3")
		f.routes["a3"] = plRoute("a3") // proxy route serving another subscriber in a3
		f.exposure.Status.ProxyRoutes = append(f.exposure.Status.ProxyRoutes, plRouteRef("a3"))
		h := newPlHarness(f)
		l, calls := h.provision()

		base := f.ecs["a3"].Status.ProxyCallbackURLs["c"]
		plExpectPlacement(l, "c", "a3", "c", base)
		Expect(plCount(calls, "Get", "Route", plZoneNs("a3"))).To(BeZero())
		ap := l.Status.AppliedPlacement
		Expect(ap.DeliveryEventStore.Name).To(Equal("es-a3"))
		Expect(ap.CaptureEventStore.Name).To(Equal("es-c"))
		Expect(ap.Publisher.Namespace).To(Equal(plZoneNs("c")))
		for i := range l.Status.EventSubscriptions {
			ref := &l.Status.EventSubscriptions[i]
			Expect(ref.Namespace).To(Equal(plZoneNs("c")))
			sub := &pubsubv1.Subscriber{}
			Expect(h.exists(ref, sub)).To(BeTrue())
			u, err := url.Parse(sub.Spec.Delivery.Callback)
			Expect(err).ToNot(HaveOccurred())
			Expect(u.Scheme + "://" + u.Host + u.Path).To(Equal(base))
			Expect(u.Query().Get(util.CallbackQueryParam)).To(Equal("http://localhost:8080/autoevent?listener=" + plAppId))
		}
	})

	It("H4: skips a candidate whose Route is not referenced by the exposure", func() {
		f := newPlFixtures("c")
		f.exposure.Status.ProxyRoutes = []ctypes.ObjectRef{{Name: "other-name", Namespace: plZoneNs("c")}}
		h := newPlHarness(f)
		l, _ := h.provision()

		plExpectPlacement(l, "p", "c", "p", f.ecs["c"].Status.ProxyCallbackURLs["p"])
		processing := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeProcessing)
		Expect(processing).ToNot(BeNil())
		Expect(processing.Reason).ToNot(Equal(condition.ReasonBlocked))
	})

	It("H5: skips a pass-through candidate for the next one", func() {
		f := newPlFixtures("c")
		f.routes["c"].Spec.PassThrough = true
		h := newPlHarness(f)
		l, _ := h.provision()

		plExpectPlacement(l, "p", "c", "p", f.ecs["c"].Status.ProxyCallbackURLs["p"])
	})

	It("H5b: relocation drains the old capture before provisioning the replacement", func() {
		f := newPlFixtures("c")
		h := newPlHarness(f)
		l, _ := h.provision()
		oldRL := l.Status.RouteListener.DeepCopy()
		oldSubs := append([]ctypes.ObjectRef(nil), l.Status.EventSubscriptions...)
		oldFP := l.Status.AppliedPlacement.Fingerprint
		oldProviderReq := l.Status.ProviderApprovalRequest.Name

		route := &gatewayv1.Route{}
		Expect(h.exists(new(plRouteRef("c")), route)).To(BeTrue())
		route.Spec.PassThrough = true
		h.update(route)

		calls := h.mustReconcile()
		l = h.listener()
		Expect(l.Status.Draining).ToNot(BeNil())
		Expect(l.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
		Expect(l.Status.Draining.Reason).To(Equal("fingerprint changed"))
		Expect(l.Status.Draining.OldFingerprint).To(Equal(oldFP))
		plExpectNoCaptureWrites(calls)

		for pass := 0; l.Status.Draining != nil; pass++ {
			Expect(pass).To(BeNumerically("<", 10), "drain did not complete")
			calls = h.mustReconcile()
			for _, kind := range []string{"RouteListener", "Subscriber", "Publisher"} {
				Expect(plCount(calls, "Create", kind, plZoneNs("p"))).To(BeZero())
			}
			l = h.listener()
		}
		Expect(h.exists(oldRL, &gatewayv1.RouteListener{})).To(BeFalse())
		for i := range oldSubs {
			Expect(h.exists(&oldSubs[i], &pubsubv1.Subscriber{})).To(BeFalse())
		}
		// The replacement intent needs new approvals: nothing is provisioned yet.
		Expect(l.Status.ProviderApprovalRequest.Name).ToNot(Equal(oldProviderReq))
		Expect(l.Status.RouteListener).To(BeNil())
		Expect(l.Status.EventSubscriptions).To(BeEmpty())
		Expect(plCount(calls, "Create", "RouteListener", "")).To(BeZero())

		h.grant()
		h.mustReconcile()
		l = h.listener()
		plExpectPlacement(l, "p", "c", "p", f.ecs["c"].Status.ProxyCallbackURLs["p"])
		Expect(l.Status.AppliedPlacement.Fingerprint).ToNot(Equal(oldFP))
		err := h.raw.Get(context.Background(), oldRL.K8s(), &gatewayv1.RouteListener{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("H6: a transient capture EventConfig error does not relocate or stop capture", func() {
		f := newPlFixtures("a3")
		h := newPlHarness(f)
		l, _ := h.provision()
		plExpectPlacement(l, "c", "a3", "c", f.ecs["a3"].Status.ProxyCallbackURLs["c"])
		applied := l.Status.AppliedPlacement.DeepCopy()
		rlRef := l.Status.RouteListener.DeepCopy()

		ec := &eventv1.EventConfig{}
		Expect(h.raw.Get(context.Background(), client.ObjectKeyFromObject(f.ecs["c"]), ec)).To(Succeed())
		ec.Status.Conditions = []metav1.Condition{{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: "Degraded"}}
		h.update(ec)

		calls := h.mustReconcile()
		l = h.listener()
		Expect(plBlocked(l)).To(ContainSubstring("not ready"))
		Expect(plCount(calls, "Get", "Route", plZoneNs("p"))).To(BeZero())
		Expect(plECListed(calls, "p")).To(BeFalse())
		plExpectNoCaptureWrites(calls)
		Expect(l.Status.Draining).To(BeNil())
		Expect(l.Status.AppliedPlacement).To(Equal(applied))

		fresh := &eventv1.EventConfig{}
		Expect(h.raw.Get(context.Background(), client.ObjectKeyFromObject(ec), fresh)).To(Succeed())
		fresh.Status.Conditions = plReady()
		h.update(fresh)

		calls = h.mustReconcile()
		l = h.listener()
		Expect(l.Status.Draining).To(BeNil())
		Expect(l.Status.AppliedPlacement.Fingerprint).To(Equal(applied.Fingerprint))
		Expect(plCount(calls, "Delete", "RouteListener", "")).To(BeZero())
		Expect(plCount(calls, "Delete", "Subscriber", "")).To(BeZero())
		Expect(h.exists(rlRef, &gatewayv1.RouteListener{})).To(BeTrue())
	})

	It("H7: blocks with every candidate's reason when all are unsuitable", func() {
		f := newPlFixtures("a3")
		f.routes["c"] = nil
		f.routes["p"].Spec.PassThrough = true
		h := newPlHarness(f)

		all := h.startup()
		msg := plBlocked(h.listener())
		for _, s := range []string{`zone "a3"`, `zone "c"`, "no Route", `zone "p"`, "pass-through"} {
			Expect(msg).To(ContainSubstring(s))
		}
		for _, kind := range []string{"ApprovalRequest", "RouteListener", "Subscriber", "Publisher"} {
			Expect(plCount(all, "Create", kind, "")).To(BeZero(), "Create %s", kind)
		}
	})

	It("H8: drains applied capture whose Route became unsupported when no other candidate works", func() {
		f := newPlFixtures("c")
		f.apps[1].Spec.Zone = plZoneRef("c") // P in c as well: a single-zone topology
		f.exposure.Spec.Zone = plZoneRef("c")
		f.exposure.Status.Route = new(plRouteRef("c"))
		f.exposure.Status.ProxyRoutes = nil
		f.routes["p"] = nil
		h := newPlHarness(f)
		l, _ := h.provision()
		plExpectPlacement(l, "c", "c", "c", f.ecs["c"].Status.CallbackURL)
		rlRef := l.Status.RouteListener.DeepCopy()
		subRefs := append([]ctypes.ObjectRef(nil), l.Status.EventSubscriptions...)

		route := &gatewayv1.Route{}
		Expect(h.exists(new(plRouteRef("c")), route)).To(BeTrue())
		route.Spec.PassThrough = true
		h.update(route)

		// Checkpoint only.
		calls := h.mustReconcile()
		l = h.listener()
		Expect(l.Status.Draining).ToNot(BeNil())
		Expect(l.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
		Expect(l.Status.Draining.Reason).To(Equal("no supported capture placement"))
		Expect(plCountVerb(calls, "Delete")).To(BeZero())
		Expect(plBlocked(l)).To(ContainSubstring("pass-through"))

		// Stopping deletes the RouteListener before any Subscriber.
		calls = h.mustReconcile()
		Expect(plCount(calls, "Delete", "RouteListener", "")).To(Equal(1))
		Expect(plCount(calls, "Delete", "Subscriber", "")).To(BeZero())
		err := h.raw.Get(context.Background(), rlRef.K8s(), &gatewayv1.RouteListener{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		for i := range subRefs {
			Expect(h.exists(&subRefs[i], &pubsubv1.Subscriber{})).To(BeTrue())
		}

		calls = h.mustReconcile()
		Expect(plCountVerb(calls, "Delete")).To(BeZero())
		Expect(h.listener().Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))

		calls = h.mustReconcile()
		Expect(plCount(calls, "Delete", "Subscriber", "")).To(Equal(2))
		for i := range subRefs {
			err := h.raw.Get(context.Background(), subRefs[i].K8s(), &pubsubv1.Subscriber{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}

		h.mustReconcile()
		Expect(h.listener().Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseCleaningPublisher))

		h.mustReconcile()
		l = h.listener()
		Expect(l.Status.Draining).To(BeNil())
		Expect(l.Status.AppliedPlacement).To(BeNil())
		Expect(plBlocked(l)).To(ContainSubstring("pass-through"))

		for range 2 {
			calls = h.mustReconcile()
			l = h.listener()
			Expect(l.Status.Draining).To(BeNil())
			Expect(l.Status.AppliedPlacement).To(BeNil())
			Expect(plCount(calls, "Delete", "RouteListener", "")).To(BeZero())
			Expect(plCount(calls, "Delete", "Subscriber", "")).To(BeZero())
			Expect(plCountVerb(calls, "Create")).To(BeZero(), "unexpected create in %+v", calls)
		}
	})

	It("H9: proxy-backed capture keys the callback by the proxy zone and uses its own EventStore", func() {
		f := newPlFixtures("a3")
		f.proxy("c", "t")
		h := newPlHarness(f)
		l, _ := h.provision()

		plExpectPlacement(l, "c", "a3", "c", f.ecs["a3"].Status.ProxyCallbackURLs["c"])
		ap := l.Status.AppliedPlacement
		Expect(ap.CaptureEventStore.Name).To(Equal("es-c"))
		Expect(ap.CaptureEventStore.Namespace).To(Equal(plZoneNs("c")))
		Expect(ap.Publisher.Namespace).To(Equal(plZoneNs("c")))
		for _, ref := range l.Status.EventSubscriptions {
			Expect(ref.Namespace).To(Equal(plZoneNs("c")))
		}
	})

	It("H9b: a proxy chain at a capture candidate blocks without relocating", func() {
		f := newPlFixtures("a3")
		f.proxy("c", "t")
		f.proxy("t", "p")
		h := newPlHarness(f)

		all := h.startup()
		Expect(plBlocked(h.listener())).To(ContainSubstring("must be a local (non-proxy) zone"))
		Expect(plCount(all, "Get", "Route", plZoneNs("p"))).To(BeZero())
		Expect(plECListed(all, "p")).To(BeFalse())
		Expect(plCount(all, "Create", "ApprovalRequest", "")).To(BeZero())
	})

	It("H10: proxy-backed A delivers through A's own EventStore and proxy callback", func() {
		f := newPlFixtures("a3")
		f.proxy("a3", "t")
		h := newPlHarness(f)
		l, _ := h.provision()

		plExpectPlacement(l, "c", "a3", "c", f.ecs["a3"].Status.ProxyCallbackURLs["c"])
		Expect(l.Status.AppliedPlacement.DeliveryEventStore.Name).To(Equal("es-a3"))
		Expect(l.Status.AppliedPlacement.DeliveryEventStore.Namespace).To(Equal(plZoneNs("a3")))
	})

	It("H11: a missing ProxyCallbackURLs key makes that candidate unsuitable without substitution", func() {
		f := newPlFixtures("a3")
		f.ecs["a3"].Status.ProxyCallbackURLs = map[string]string{"p": "https://a3.gw.example/proxy/p"}
		h := newPlHarness(f)
		l, _ := h.provision()

		plExpectPlacement(l, "p", "a3", "p", "https://a3.gw.example/proxy/p")
	})

	It("H11b: no ProxyCallbackURLs at A blocks naming every candidate", func() {
		f := newPlFixtures("a3")
		f.ecs["a3"].Status.ProxyCallbackURLs = nil
		h := newPlHarness(f)
		h.startup()
		msg := plBlocked(h.listener())
		Expect(msg).To(ContainSubstring(`zone "c": delivery EventConfig "ec-a3" has no ProxyCallbackURLs entry`))
		Expect(msg).To(ContainSubstring(`zone "p": delivery EventConfig "ec-a3" has no ProxyCallbackURLs entry`))
	})

	It("H12: an unverifiable provider binding blocks without trying later candidates", func() {
		f := newPlFixtures("a3")
		f.routes["c"].Labels[cconfig.OwnerUidLabelKey] = "foreign-uid"
		h := newPlHarness(f)
		all := h.startup()
		Expect(plBlocked(h.listener())).To(ContainSubstring(`no ApiExposure with UID "foreign-uid"`))
		Expect(plCount(all, "Get", "Route", plZoneNs("p"))).To(BeZero())
		Expect(plECListed(all, "p")).To(BeFalse())
	})

	It("H12b: an ApiExposure read error with applied capture errors without draining or deleting", func() {
		f := newPlFixtures("c")
		h := newPlHarness(f)
		l, _ := h.provision()
		plExpectPlacement(l, "c", "c", "c", f.ecs["c"].Status.CallbackURL)
		applied := l.Status.AppliedPlacement.DeepCopy()
		rlRef := l.Status.RouteListener.DeepCopy()
		subRefs := append([]ctypes.ObjectRef(nil), l.Status.EventSubscriptions...)

		h.listErr = func(list client.ObjectList) error {
			if _, ok := list.(*apiv1.ApiExposureList); ok {
				return apierrors.NewServiceUnavailable("etcd leader changed")
			}
			return nil
		}
		for range 3 {
			calls, err := h.reconcile()
			Expect(err).To(MatchError(ContainSubstring("etcd leader changed")))
			l = h.listener()
			Expect(l.Status.Draining).To(BeNil())
			Expect(l.Status.AppliedPlacement).To(Equal(applied))
			Expect(plCountVerb(calls, "Delete")).To(BeZero(), "unexpected delete in %+v", calls)
			plExpectNoCaptureWrites(calls)
			Expect(plCount(calls, "Get", "Route", plZoneNs("p"))).To(BeZero())
			ready := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeReady)
			Expect(ready).ToNot(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		}

		h.listErr = nil
		calls := h.mustReconcile()
		l = h.listener()
		Expect(l.Status.Draining).To(BeNil())
		Expect(l.Status.AppliedPlacement.Fingerprint).To(Equal(applied.Fingerprint))
		Expect(plCountVerb(calls, "Delete")).To(BeZero())
		Expect(h.exists(rlRef, &gatewayv1.RouteListener{})).To(BeTrue())
		for i := range subRefs {
			Expect(h.exists(&subRefs[i], &pubsubv1.Subscriber{})).To(BeTrue())
		}
	})

	It("H12c: an exposure that became inactive checkpoints first, then drains applied capture", func() {
		f := newPlFixtures("c")
		h := newPlHarness(f)
		l, _ := h.provision()
		oldFP := l.Status.AppliedPlacement.Fingerprint
		rlRef := l.Status.RouteListener.DeepCopy()
		subRefs := append([]ctypes.ObjectRef(nil), l.Status.EventSubscriptions...)

		exposure := &apiv1.ApiExposure{}
		Expect(h.raw.Get(context.Background(), client.ObjectKeyFromObject(f.exposure), exposure)).To(Succeed())
		exposure.Status.Active = false
		h.update(exposure)

		// Checkpoint only: the drain is persisted before anything is deleted.
		calls := h.mustReconcile()
		l = h.listener()
		Expect(l.Status.Draining).ToNot(BeNil())
		Expect(l.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
		Expect(l.Status.Draining.Reason).To(Equal("provider binding invalidated"))
		Expect(l.Status.Draining.OldFingerprint).To(Equal(oldFP))
		Expect(plCountVerb(calls, "Delete")).To(BeZero())
		Expect(plCount(calls, "Get", "Route", plZoneNs("p"))).To(BeZero())
		Expect(plBlocked(l)).To(ContainSubstring("is not active"))
		ready := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeReady)
		Expect(ready).ToNot(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Message).To(ContainSubstring("is not active"))

		// Stopping deletes the RouteListener before any Subscriber.
		calls = h.mustReconcile()
		Expect(plCount(calls, "Delete", "RouteListener", "")).To(Equal(1))
		Expect(plCount(calls, "Delete", "Subscriber", "")).To(BeZero())
		err := h.raw.Get(context.Background(), rlRef.K8s(), &gatewayv1.RouteListener{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		for i := range subRefs {
			Expect(h.exists(&subRefs[i], &pubsubv1.Subscriber{})).To(BeTrue())
		}

		for pass := 0; h.listener().Status.Draining != nil; pass++ {
			Expect(pass).To(BeNumerically("<", 10), "drain did not complete")
			calls = h.mustReconcile()
			Expect(plCountVerb(calls, "Create")).To(BeZero(), "unexpected create in %+v", calls)
		}
		for i := range subRefs {
			err := h.raw.Get(context.Background(), subRefs[i].K8s(), &pubsubv1.Subscriber{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}
		l = h.listener()
		Expect(l.Status.AppliedPlacement).To(BeNil())
		Expect(plBlocked(l)).To(ContainSubstring("is not active"))

		calls = h.mustReconcile()
		Expect(h.listener().Status.Draining).To(BeNil())
		Expect(plCountVerb(calls, "Create")).To(BeZero())
		Expect(plCountVerb(calls, "Delete")).To(BeZero())
	})

	It("H13: delivery never falls back when A's zone has no EventConfig", func() {
		f := newPlFixtures("a3")
		f.ecs["a3"] = nil
		h := newPlHarness(f)
		all := h.startup()
		l := h.listener()
		Expect(plBlocked(l)).To(Equal(`no EventConfig found for zone "a3"`))
		ready := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeReady)
		Expect(ready).ToNot(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Message).To(ContainSubstring("delivery EventConfig for observer zone"))
		Expect(plCount(all, "Get", "Route", "")).To(BeZero())
		Expect(plCount(all, "Create", "ApprovalRequest", "")).To(BeZero())
	})
})
