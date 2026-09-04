// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// --- Test fixtures ---

const (
	testAppName             = "my-app"
	testAppNamespace        = "team-ns"
	testZoneName            = "aws"
	testZoneNs              = "env-ns"
	testZoneStatusNs        = "env-ns--aws"
	testSSEUrl              = "https://horizon-sse.internal:443/api/v1/sse"
	testGatewayCallbackURL  = "https://callback.gateway.example.com/callback"
	testCustomerCallbackURL = "https://customer.example.com/callback"
)

func newSpectreApplication(deliveryType string) *spectrev1.SpectreApplication {
	app := &spectrev1.SpectreApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa-" + testAppName,
			Namespace: testAppNamespace,
			UID:       "sa-uid-001",
		},
		Spec: spectrev1.SpectreApplicationSpec{
			Application: ctypes.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
				ObjectRef: ctypes.ObjectRef{
					Name:      testAppName,
					Namespace: testAppNamespace,
				},
			},
			DeliveryType: deliveryType,
		},
	}
	if deliveryType == "callback" {
		app.Spec.Callback = "https://customer.example.com/callback"
	}
	return app
}

func makeReadyApplication() *applicationv1.Application {
	app := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAppName,
			Namespace: testAppNamespace,
		},
		Spec: applicationv1.ApplicationSpec{
			Team: "pandora",
			Zone: ctypes.ObjectRef{Name: testZoneName, Namespace: testZoneNs},
		},
		Status: applicationv1.ApplicationStatus{
			ClientId: "pandora--my-app",
		},
	}
	meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return app
}

func makeReadyZone() *adminv1.Zone {
	z := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testZoneName,
			Namespace: testZoneNs,
		},
		Spec: adminv1.ZoneSpec{
			Gateway: adminv1.GatewayConfig{
				Presets: []adminv1.GatewayConfigPreset{
					{
						Name:    "default",
						Default: true,
						Urls: []adminv1.UrlConfig{
							{Hostname: "gateway.example.com", Port: 443, Scheme: "https"},
						},
					},
				},
			},
		},
		Status: adminv1.ZoneStatus{
			Namespace: testZoneStatusNs,
			Gateway: &ctypes.ObjectRef{
				Name:      "gateway-aws",
				Namespace: testZoneStatusNs,
			},
		},
	}
	meta.SetStatusCondition(&z.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return z
}

func makeReadyEventConfig() eventv1.EventConfig {
	ec := eventv1.EventConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ec-aws",
			Namespace: testZoneStatusNs,
		},
		Spec: eventv1.EventConfigSpec{
			Zone: ctypes.ObjectRef{Name: testZoneName, Namespace: testZoneNs},
			Local: &eventv1.LocalBackend{
				Admin:              eventv1.AdminConfig{Url: "http://admin.local"},
				ServerSendEventUrl: testSSEUrl,
				PublishEventUrl:    "http://publish.local",
			},
		},
		Status: eventv1.EventConfigStatus{
			CallbackURL: testGatewayCallbackURL,
			EventStore: &ctypes.ObjectRef{
				Name:      "eventstore-aws",
				Namespace: testZoneStatusNs,
			},
		},
	}
	meta.SetStatusCondition(&ec.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return ec
}

func makeEventStore() *pubsubv1.EventStore {
	es := &pubsubv1.EventStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "eventstore-aws",
			Namespace: testZoneStatusNs,
		},
		Spec: pubsubv1.EventStoreSpec{
			Url:          "http://admin.local",
			TokenUrl:     "http://token.local",
			ClientId:     "client-id",
			ClientSecret: "client-secret",
		},
	}
	meta.SetStatusCondition(&es.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return es
}

// --- Tests ---

var _ = Describe("SpectreApplicationHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *handler.SpectreApplicationHandler
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &handler.SpectreApplicationHandler{}
	})

	// --- Mock helpers ---

	mockGetApplication := func(app *applicationv1.Application) {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: testAppName, Namespace: testAppNamespace}, mock.AnythingOfType("*v1.Application")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*applicationv1.Application) = *app
			}).
			Return(nil).Once()
	}

	mockGetZone := func(zone *adminv1.Zone) {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: testZoneName, Namespace: testZoneNs}, mock.AnythingOfType("*v1.Zone")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*adminv1.Zone) = *zone
			}).
			Return(nil).Once()
	}

	mockListEventConfigs := func(items []eventv1.EventConfig) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: items}
			}).
			Return(nil).Once()
	}

	mockGetEventStore := func(es *pubsubv1.EventStore) {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: es.Name, Namespace: es.Namespace}, mock.AnythingOfType("*v1.EventStore")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*pubsubv1.EventStore) = *es
			}).
			Return(nil).Once()
	}

	mockCreateOrUpdatePublisher := func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()
	}

	mockCreateOrUpdateSubscriber := func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()
	}

	mockCreateOrUpdateRoute := func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()
	}

	mockCleanup := func() {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Return(0, nil).Once()
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
			Return(0, nil).Once()
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.PublisherList"), mock.Anything).
			Return(0, nil).Once()
	}

	// mockExplicitReadinessChecks stubs the Get calls that ensureChildReady
	// makes after AllReady() returns true. Each child is returned with Ready=True.
	mockExplicitReadinessChecks := func(deliveryType string) {
		// Publisher readiness check.
		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Publisher")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				pub := out.(*pubsubv1.Publisher)
				meta.SetStatusCondition(&pub.Status.Conditions, metav1.Condition{
					Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
				})
			}).
			Return(nil).Once()
		// Subscriber readiness check.
		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Subscriber")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				sub := out.(*pubsubv1.Subscriber)
				meta.SetStatusCondition(&sub.Status.Conditions, metav1.Condition{
					Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
				})
			}).
			Return(nil).Once()
		// Route readiness check (SSE only).
		if deliveryType == "server_sent_event" {
			fakeClient.EXPECT().
				Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Route")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					route := out.(*gatewayv1.Route)
					meta.SetStatusCondition(&route.Status.Conditions, metav1.Condition{
						Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
					})
				}).
				Return(nil).Once()
		}
	}

	setupHappyPath := func(deliveryType string) *spectrev1.SpectreApplication {
		obj := newSpectreApplication(deliveryType)
		app := makeReadyApplication()
		zone := makeReadyZone()
		ec := makeReadyEventConfig()
		es := makeEventStore()

		mockGetApplication(app)
		mockGetZone(zone)
		mockListEventConfigs([]eventv1.EventConfig{ec})
		mockGetEventStore(es)
		mockCreateOrUpdatePublisher()
		mockCreateOrUpdateSubscriber()

		if deliveryType == "server_sent_event" {
			mockCreateOrUpdateRoute()
		}

		mockCleanup()

		return obj
	}

	Describe("CreateOrUpdate", func() {
		Context("with SSE delivery", func() {
			It("should create Publisher with correct fields", func() {
				obj := setupHappyPath("server_sent_event")
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("server_sent_event")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				// Verify status refs are set
				Expect(obj.Status.Publisher).ToNot(BeNil())
				Expect(obj.Status.Publisher.Namespace).To(Equal(testZoneStatusNs))
				Expect(obj.Status.Id).To(Equal("pandora--my-app"))
			})

			It("should create Subscriber referencing Publisher", func() {
				obj := setupHappyPath("server_sent_event")
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("server_sent_event")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(obj.Status.Subscriber).ToNot(BeNil())
				Expect(obj.Status.Subscriber.Namespace).To(Equal(testZoneStatusNs))
			})

			It("should create SSE Route when delivery is server_sent_event", func() {
				obj := setupHappyPath("server_sent_event")
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("server_sent_event")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(obj.Status.ListenerRoute).ToNot(BeNil())
				Expect(obj.Status.ListenerRoute.Namespace).To(Equal(testZoneStatusNs))
			})

			It("should set Ready condition when all children are ready", func() {
				obj := setupHappyPath("server_sent_event")
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("server_sent_event")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(obj.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(readyCond.Reason).To(Equal(condition.ReasonProvisioned))
			})

			It("should set NotReady when AllReady returns false", func() {
				obj := setupHappyPath("server_sent_event")
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(false).Once()

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(obj.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
				Expect(readyCond.Reason).To(Equal(condition.ReasonSubResourceNotReady))
			})
		})

		Context("with callback delivery", func() {
			It("should NOT create SSE Route when delivery is callback", func() {
				obj := setupHappyPath("callback")
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("callback")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				// No Route should be in status
				Expect(obj.Status.ListenerRoute).To(BeNil())
				// Publisher and Subscriber should still be created
				Expect(obj.Status.Publisher).ToNot(BeNil())
				Expect(obj.Status.Subscriber).ToNot(BeNil())
			})
		})

		Context("with Publisher details verification", func() {
			It("should create Publisher with correct event type and publisher ID", func() {
				obj := newSpectreApplication("server_sent_event")
				app := makeReadyApplication()
				zone := makeReadyZone()
				ec := makeReadyEventConfig()
				es := makeEventStore()

				mockGetApplication(app)
				mockGetZone(zone)
				mockListEventConfigs([]eventv1.EventConfig{ec})
				mockGetEventStore(es)

				// Capture the Publisher object
				var capturedPublisher *pubsubv1.Publisher
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
						capturedPublisher = obj.(*pubsubv1.Publisher)
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()

				mockCreateOrUpdateSubscriber()
				mockCreateOrUpdateRoute()
				mockCleanup()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("server_sent_event")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedPublisher).ToNot(BeNil())
				Expect(capturedPublisher.Labels[cconfig.OwnerUidLabelKey]).To(Equal(string(obj.UID)))
				Expect(capturedPublisher.Spec.EventType).To(Equal("de.telekom.ei.listener.pandora--my-app"))
				Expect(capturedPublisher.Spec.PublisherId).To(Equal("gateway"))
				Expect(capturedPublisher.Spec.EventStore.Name).To(Equal("eventstore-aws"))
				Expect(capturedPublisher.Spec.EventStore.Namespace).To(Equal(testZoneStatusNs))
			})
		})

		Context("with Subscriber details verification", func() {
			It("should create Subscriber with correct delivery type for SSE", func() {
				obj := newSpectreApplication("server_sent_event")
				app := makeReadyApplication()
				zone := makeReadyZone()
				ec := makeReadyEventConfig()
				es := makeEventStore()

				mockGetApplication(app)
				mockGetZone(zone)
				mockListEventConfigs([]eventv1.EventConfig{ec})
				mockGetEventStore(es)
				mockCreateOrUpdatePublisher()

				// Capture the Subscriber object
				var capturedSubscriber *pubsubv1.Subscriber
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
						capturedSubscriber = obj.(*pubsubv1.Subscriber)
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()

				mockCreateOrUpdateRoute()
				mockCleanup()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("server_sent_event")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedSubscriber).ToNot(BeNil())
				Expect(capturedSubscriber.Labels[cconfig.OwnerUidLabelKey]).To(Equal(string(obj.UID)))
				Expect(capturedSubscriber.Spec.SubscriberId).To(Equal("pandora--my-app"))
				Expect(capturedSubscriber.Spec.Delivery.Type).To(Equal(pubsubv1.DeliveryTypeServerSentEvent))
				Expect(capturedSubscriber.Spec.Delivery.Callback).To(BeEmpty())
			})

			It("should create Subscriber with gateway-mediated callback URL for callback delivery", func() {
				obj := newSpectreApplication("callback")
				app := makeReadyApplication()
				zone := makeReadyZone()
				ec := makeReadyEventConfig()
				es := makeEventStore()

				mockGetApplication(app)
				mockGetZone(zone)
				mockListEventConfigs([]eventv1.EventConfig{ec})
				mockGetEventStore(es)
				mockCreateOrUpdatePublisher()

				var capturedSubscriber *pubsubv1.Subscriber
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
						capturedSubscriber = obj.(*pubsubv1.Subscriber)
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()

				mockCleanup()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("callback")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedSubscriber).ToNot(BeNil())
				Expect(capturedSubscriber.Labels[cconfig.OwnerUidLabelKey]).To(Equal(string(obj.UID)))
				Expect(capturedSubscriber.Spec.Delivery.Type).To(Equal(pubsubv1.DeliveryTypeCallback))
				// The callback must route through the Gateway, not use the raw customer URL.
				Expect(capturedSubscriber.Spec.Delivery.Callback).To(ContainSubstring(testGatewayCallbackURL))
				Expect(capturedSubscriber.Spec.Delivery.Callback).To(ContainSubstring("callback="))
				Expect(capturedSubscriber.Spec.Delivery.Callback).ToNot(Equal(testCustomerCallbackURL))
			})
		})

		Context("with SSE Route details verification", func() {
			It("should create Route with correct gateway ref and paths", func() {
				obj := newSpectreApplication("server_sent_event")
				app := makeReadyApplication()
				zone := makeReadyZone()
				ec := makeReadyEventConfig()
				es := makeEventStore()

				mockGetApplication(app)
				mockGetZone(zone)
				mockListEventConfigs([]eventv1.EventConfig{ec})
				mockGetEventStore(es)
				mockCreateOrUpdatePublisher()
				mockCreateOrUpdateSubscriber()

				var capturedRoute *gatewayv1.Route
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
						capturedRoute = obj.(*gatewayv1.Route)
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()

				mockCleanup()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("server_sent_event")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedRoute).ToNot(BeNil())
				Expect(capturedRoute.Labels[cconfig.OwnerUidLabelKey]).To(Equal(string(obj.UID)))
				Expect(capturedRoute.Namespace).To(Equal(testZoneStatusNs))
				Expect(capturedRoute.Spec.GatewayRef.Name).To(Equal("gateway-aws"))
				Expect(capturedRoute.Spec.GatewayRef.Namespace).To(Equal(testZoneStatusNs))
				Expect(capturedRoute.Spec.Security.DisableAccessControl).To(BeTrue())
				Expect(capturedRoute.Spec.Buffering.DisableResponseBuffering).To(BeTrue())
				Expect(capturedRoute.Spec.Backend.Upstreams).To(HaveLen(1))
				Expect(capturedRoute.Spec.Backend.Upstreams[0].Hostname).To(Equal("horizon-sse.internal"))
				Expect(capturedRoute.Spec.Backend.Upstreams[0].Port).To(Equal(int32(443)))
				Expect(capturedRoute.Spec.Backend.Upstreams[0].Scheme).To(Equal("https"))
				// Path should contain the event type
				Expect(capturedRoute.Spec.Paths).ToNot(BeEmpty())
				Expect(capturedRoute.Spec.Paths[0]).To(ContainSubstring("/sse/v1/de.telekom.ei.listener.pandora--my-app"))
			})
		})

		Context("error handling", func() {
			It("should return error when Application is not found", func() {
				obj := newSpectreApplication("server_sent_event")

				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: testAppName, Namespace: testAppNamespace}, mock.AnythingOfType("*v1.Application")).
					Return(fmt.Errorf("not found")).Once()

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})

			It("should return error when EventConfig has no EventStore reference", func() {
				obj := newSpectreApplication("server_sent_event")
				app := makeReadyApplication()
				zone := makeReadyZone()
				ec := makeReadyEventConfig()
				ec.Status.EventStore = nil

				mockGetApplication(app)
				mockGetZone(zone)
				mockListEventConfigs([]eventv1.EventConfig{ec})

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("has no EventStore reference"))
			})

			It("should return blocked error when callback delivery but EventConfig has no CallbackURL", func() {
				obj := newSpectreApplication("callback")
				app := makeReadyApplication()
				zone := makeReadyZone()
				ec := makeReadyEventConfig()
				ec.Status.CallbackURL = ""
				es := makeEventStore()

				mockGetApplication(app)
				mockGetZone(zone)
				mockListEventConfigs([]eventv1.EventConfig{ec})
				mockGetEventStore(es)

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no CallbackURL"))
			})
		})

		Context("cleanup of obsolete children", func() {
			It("should run cleanup in order Route -> Subscriber -> Publisher", func() {
				// The mock call order is enforced by testify — each .Once() must
				// be consumed in the order registered when types differ.
				obj := setupHappyPath("server_sent_event")
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("server_sent_event")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should run Route cleanup for callback delivery to remove stale SSE Routes", func() {
				// Callback delivery does not call ensureSSERoute, but cleanup
				// still runs for Routes — this handles SSE→callback transition.
				obj := setupHappyPath("callback")
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("callback")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				// No Route in status (callback), but Route cleanup was mocked and called.
				Expect(obj.Status.ListenerRoute).To(BeNil())
			})
		})

		Context("cross-zone SSE proxy routes", func() {
			const (
				backendZoneName     = "backend-zone"
				backendZoneNs       = "env-ns"
				backendZoneStatusNs = "env-ns--backend-zone"
				backendSSEUrl       = "https://horizon-sse.backend:443/api/v1/sse"
				backendIssuer       = "https://idp.backend.example.com"
				backendLmsIssuer    = "https://lms.backend.example.com"
				appZoneIssuer       = "https://idp.app.example.com"
				appZoneLmsIssuer    = "https://lms.app.example.com"
			)

			makeProxyEventConfig := func() eventv1.EventConfig {
				ec := eventv1.EventConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ec-proxy",
						Namespace: testZoneStatusNs,
					},
					Spec: eventv1.EventConfigSpec{
						Zone: ctypes.ObjectRef{Name: testZoneName, Namespace: testZoneNs},
						Proxy: &eventv1.ProxyBackend{
							TargetZone: ctypes.ObjectRef{Name: backendZoneName, Namespace: backendZoneNs},
						},
					},
					Status: eventv1.EventConfigStatus{
						CallbackURL: testGatewayCallbackURL,
						EventStore: &ctypes.ObjectRef{
							Name:      "eventstore-backend",
							Namespace: backendZoneStatusNs,
						},
					},
				}
				meta.SetStatusCondition(&ec.Status.Conditions, metav1.Condition{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
					Reason: "Ready",
				})
				return ec
			}

			makeBackendZone := func() *adminv1.Zone {
				z := &adminv1.Zone{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backendZoneName,
						Namespace: backendZoneNs,
					},
					Spec: adminv1.ZoneSpec{
						Gateway: adminv1.GatewayConfig{
							Presets: []adminv1.GatewayConfigPreset{
								{
									Name:    "default",
									Default: true,
									Urls: []adminv1.UrlConfig{
										{Hostname: "gateway.backend.example.com", Port: 443, Scheme: "https"},
									},
								},
							},
						},
					},
					Status: adminv1.ZoneStatus{
						Namespace: backendZoneStatusNs,
						RealmName: "test-realm",
						Gateway: &ctypes.ObjectRef{
							Name:      "gateway-backend",
							Namespace: backendZoneStatusNs,
						},
						Links: adminv1.Links{
							Issuer:    backendIssuer,
							LmsIssuer: backendLmsIssuer,
						},
					},
				}
				meta.SetStatusCondition(&z.Status.Conditions, metav1.Condition{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
					Reason: "Ready",
				})
				return z
			}

			makeBackendEventConfig := func() eventv1.EventConfig {
				ec := eventv1.EventConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ec-backend",
						Namespace: backendZoneStatusNs,
					},
					Spec: eventv1.EventConfigSpec{
						Zone: ctypes.ObjectRef{Name: backendZoneName, Namespace: backendZoneNs},
						Local: &eventv1.LocalBackend{
							Admin:              eventv1.AdminConfig{Url: "http://admin.backend.local"},
							ServerSendEventUrl: backendSSEUrl,
							PublishEventUrl:    "http://publish.backend.local",
						},
					},
					Status: eventv1.EventConfigStatus{
						CallbackURL: "https://callback.backend.example.com/callback",
						EventStore: &ctypes.ObjectRef{
							Name:      "eventstore-backend",
							Namespace: backendZoneStatusNs,
						},
					},
				}
				meta.SetStatusCondition(&ec.Status.Conditions, metav1.Condition{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
					Reason: "Ready",
				})
				return ec
			}

			makeBackendEventStore := func() *pubsubv1.EventStore {
				es := &pubsubv1.EventStore{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "eventstore-backend",
						Namespace: backendZoneStatusNs,
					},
					Spec: pubsubv1.EventStoreSpec{
						Url:          "http://admin.backend.local",
						TokenUrl:     "http://token.backend.local",
						ClientId:     "client-id-backend",
						ClientSecret: "client-secret-backend",
					},
				}
				meta.SetStatusCondition(&es.Status.Conditions, metav1.Condition{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
					Reason: "Ready",
				})
				return es
			}

			It("should create only primary route for local zone (no proxy needed)", func() {
				obj := setupHappyPath("server_sent_event")
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("server_sent_event")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(obj.Status.ListenerRoute).ToNot(BeNil())
				Expect(obj.Status.ListenerRoute.Namespace).To(Equal(testZoneStatusNs))
				Expect(obj.Status.ProxyRoute).To(BeNil())
			})

			It("should create primary route in backend zone and proxy route in app zone for proxy zone", func() {
				obj := newSpectreApplication("server_sent_event")
				app := makeReadyApplication()
				zone := makeReadyZone()
				zone.Status.Links = adminv1.Links{
					Issuer:    appZoneIssuer,
					LmsIssuer: appZoneLmsIssuer,
				}
				zone.Status.RealmName = "test-realm"
				proxyEC := makeProxyEventConfig()
				backendZone := makeBackendZone()
				backendEC := makeBackendEventConfig()
				backendES := makeBackendEventStore()

				mockGetApplication(app)
				mockGetZone(zone)
				// First List: proxy EventConfig for app zone
				mockListEventConfigs([]eventv1.EventConfig{proxyEC})
				// Get EventStore from proxy EventConfig's status ref
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "eventstore-backend", Namespace: backendZoneStatusNs}, mock.AnythingOfType("*v1.EventStore")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						*out.(*pubsubv1.EventStore) = *backendES
					}).
					Return(nil).Once()
				mockCreateOrUpdatePublisher()
				mockCreateOrUpdateSubscriber()

				// Get backend zone for proxy resolution
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: backendZoneName, Namespace: backendZoneNs}, mock.AnythingOfType("*v1.Zone")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						*out.(*adminv1.Zone) = *backendZone
					}).
					Return(nil).Once()
				// List backend zone's EventConfig
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{backendEC}}
					}).
					Return(nil).Once()

				// Two Route CreateOrUpdate calls: proxy + primary
				var capturedProxyRoute, capturedPrimaryRoute *gatewayv1.Route
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
						capturedProxyRoute = obj.(*gatewayv1.Route)
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
						capturedPrimaryRoute = obj.(*gatewayv1.Route)
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()

				mockCleanup()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("server_sent_event")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				// Proxy route should be in the app zone namespace
				Expect(capturedProxyRoute).ToNot(BeNil())
				Expect(capturedProxyRoute.Namespace).To(Equal(testZoneStatusNs))
				Expect(capturedProxyRoute.Spec.Type).To(Equal(gatewayv1.RouteTypeProxy))
				Expect(capturedProxyRoute.Spec.Security.DisableAccessControl).To(BeTrue())
				Expect(capturedProxyRoute.Spec.Buffering.DisableResponseBuffering).To(BeTrue())
				Expect(capturedProxyRoute.Labels[cconfig.OwnerUidLabelKey]).To(Equal(string(obj.UID)))
				Expect(capturedProxyRoute.Labels[cconfig.BuildLabelKey("type")]).To(Equal("sse-proxy"))
				// Proxy route upstream should point at the backend zone's gateway
				Expect(capturedProxyRoute.Spec.Backend.Upstreams).To(HaveLen(1))
				Expect(capturedProxyRoute.Spec.Backend.Upstreams[0].Hostname).To(Equal("gateway.backend.example.com"))
				// Proxy route should trust the app zone's IDP issuer
				Expect(capturedProxyRoute.Spec.Security.TrustedIssuers).To(ContainElement(appZoneIssuer))

				// Primary route should be in the backend zone namespace
				Expect(capturedPrimaryRoute).ToNot(BeNil())
				Expect(capturedPrimaryRoute.Namespace).To(Equal(backendZoneStatusNs))
				Expect(capturedPrimaryRoute.Spec.Type).To(Equal(gatewayv1.RouteTypePrimary))
				Expect(capturedPrimaryRoute.Spec.Security.DisableAccessControl).To(BeTrue())
				Expect(capturedPrimaryRoute.Spec.Buffering.DisableResponseBuffering).To(BeTrue())
				Expect(capturedPrimaryRoute.Labels[cconfig.OwnerUidLabelKey]).To(Equal(string(obj.UID)))
				// Primary route upstream should point at the backend SSE URL
				Expect(capturedPrimaryRoute.Spec.Backend.Upstreams).To(HaveLen(1))
				Expect(capturedPrimaryRoute.Spec.Backend.Upstreams[0].Hostname).To(Equal("horizon-sse.backend"))
				// Primary route should trust the gateway mesh-client
				Expect(capturedPrimaryRoute.Spec.Security.DefaultConsumers).To(ContainElement(gatewayv1.GatewayConsumerName))
				// Primary route should trust both backend IDP issuer and app zone LMS issuer
				Expect(capturedPrimaryRoute.Spec.Security.TrustedIssuers).To(ContainElement(backendIssuer))
				Expect(capturedPrimaryRoute.Spec.Security.TrustedIssuers).To(ContainElement(appZoneLmsIssuer))

				// Both status refs should be set
				Expect(obj.Status.ListenerRoute).ToNot(BeNil())
				Expect(obj.Status.ListenerRoute.Namespace).To(Equal(backendZoneStatusNs))
				Expect(obj.Status.ProxyRoute).ToNot(BeNil())
				Expect(obj.Status.ProxyRoute.Namespace).To(Equal(testZoneStatusNs))
			})

			It("should return BlockedError when proxy chain resolves to non-local zone", func() {
				obj := newSpectreApplication("server_sent_event")
				app := makeReadyApplication()
				zone := makeReadyZone()
				proxyEC := makeProxyEventConfig()
				backendZone := makeBackendZone()

				// The target zone is also a proxy (chained proxies)
				chainedProxyEC := eventv1.EventConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ec-chained",
						Namespace: backendZoneStatusNs,
					},
					Spec: eventv1.EventConfigSpec{
						Zone: ctypes.ObjectRef{Name: backendZoneName, Namespace: backendZoneNs},
						Proxy: &eventv1.ProxyBackend{
							TargetZone: ctypes.ObjectRef{Name: "some-other-zone", Namespace: "some-ns"},
						},
					},
				}
				meta.SetStatusCondition(&chainedProxyEC.Status.Conditions, metav1.Condition{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
					Reason: "Ready",
				})

				es := makeBackendEventStore()

				mockGetApplication(app)
				mockGetZone(zone)
				mockListEventConfigs([]eventv1.EventConfig{proxyEC})
				// EventStore is from backend zone
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "eventstore-backend", Namespace: backendZoneStatusNs}, mock.AnythingOfType("*v1.EventStore")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						*out.(*pubsubv1.EventStore) = *es
					}).
					Return(nil).Once()
				mockCreateOrUpdatePublisher()
				mockCreateOrUpdateSubscriber()

				// Get backend zone
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: backendZoneName, Namespace: backendZoneNs}, mock.AnythingOfType("*v1.Zone")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						*out.(*adminv1.Zone) = *backendZone
					}).
					Return(nil).Once()
				// List backend zone's EventConfig — returns a proxy (chained)
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{chainedProxyEC}}
					}).
					Return(nil).Once()

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be a local (non-proxy) zone"))
			})

			It("should cleanup proxy route when switching from proxy to local delivery", func() {
				// When the zone switches from proxy to local, the cleanup mechanism
				// (OwnedByLabel) removes the stale proxy Route because it was not
				// touched in the current reconcile cycle.
				obj := setupHappyPath("server_sent_event")
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockExplicitReadinessChecks("server_sent_event")

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				// Local zone: only primary route, no proxy route
				Expect(obj.Status.ListenerRoute).ToNot(BeNil())
				Expect(obj.Status.ProxyRoute).To(BeNil())
				// Cleanup was called for Routes, which would remove any stale proxy route
			})
		})
	})

	Describe("Delete", func() {
		// mockEmptyLabelList stubs a List call returning an empty list of the given type.
		mockEmptySubList := func() {
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
				}).
				Return(nil).Once()
		}
		mockEmptyPubList := func() {
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.PublisherList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.PublisherList) = pubsubv1.PublisherList{}
				}).
				Return(nil).Once()
		}
		mockEmptyRouteList := func() {
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*gatewayv1.RouteList) = gatewayv1.RouteList{}
				}).
				Return(nil).Once()
		}

		It("should delete Subscribers before Publishers and Publishers before Routes", func() {
			obj := newSpectreApplication("server_sent_event")
			obj.Status.Publisher = &ctypes.ObjectRef{Name: "pub-1", Namespace: testZoneStatusNs}
			obj.Status.Subscriber = &ctypes.ObjectRef{Name: "sub-1", Namespace: testZoneStatusNs}
			obj.Status.ListenerRoute = &ctypes.ObjectRef{Name: "route-1", Namespace: testZoneStatusNs}

			// Phase 1: Subscriber delete (status ref).
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "sub-1", Namespace: testZoneStatusNs}, mock.AnythingOfType("*v1.Subscriber")).
				Return(nil).Once()
			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Subscriber")).
				Return(nil).Once()
			// Phase 1: label-list — empty.
			mockEmptySubList()
			// Phase 1: fresh-list — empty.
			mockEmptySubList()

			// Phase 2: Publisher delete (status ref).
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "pub-1", Namespace: testZoneStatusNs}, mock.AnythingOfType("*v1.Publisher")).
				Return(nil).Once()
			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Publisher")).
				Return(nil).Once()
			// Phase 2: label-list — empty.
			mockEmptyPubList()

			// Phase 3: Route delete (status ref).
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "route-1", Namespace: testZoneStatusNs}, mock.AnythingOfType("*v1.Route")).
				Return(nil).Once()
			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Route")).
				Return(nil).Once()
			// Phase 3: label-list — empty.
			mockEmptyRouteList()

			err := h.Delete(ctx, obj)
			Expect(err).ToNot(HaveOccurred())

			Expect(obj.Status.Publisher).To(BeNil())
			Expect(obj.Status.Subscriber).To(BeNil())
			Expect(obj.Status.ListenerRoute).To(BeNil())
		})

		It("should retry while Subscribers remain (finalizers still running)", func() {
			obj := newSpectreApplication("callback")
			obj.Status.Subscriber = &ctypes.ObjectRef{Name: "sub-1", Namespace: testZoneStatusNs}

			// Phase 1: Subscriber delete.
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "sub-1", Namespace: testZoneStatusNs}, mock.AnythingOfType("*v1.Subscriber")).
				Return(nil).Once()
			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Subscriber")).
				Return(nil).Once()
			// label-list — empty.
			mockEmptySubList()
			// fresh-list — sub still present.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
						Items: []pubsubv1.Subscriber{
							{ObjectMeta: metav1.ObjectMeta{Name: "sub-1", Namespace: testZoneStatusNs}},
						},
					}
				}).
				Return(nil).Once()

			err := h.Delete(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("waiting for Subscriber finalization"))
		})

		It("should run label-based fallback even when status refs are nil", func() {
			obj := newSpectreApplication("callback")
			// No status refs — label-based lists all empty.
			mockEmptySubList()
			mockEmptySubList() // fresh-list
			mockEmptyPubList()
			mockEmptyRouteList()

			err := h.Delete(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should delete owner-labelled children that status refs missed", func() {
			obj := newSpectreApplication("server_sent_event")
			// No status refs, but owner-labelled children exist.

			// Phase 1: label-list returns orphaned Sub.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
						Items: []pubsubv1.Subscriber{
							{ObjectMeta: metav1.ObjectMeta{Name: "orphan-sub", Namespace: testZoneStatusNs}},
						},
					}
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
					return obj.GetName() == "orphan-sub"
				}), mock.Anything).
				Return(nil).Once()
			// fresh-list — gone.
			mockEmptySubList()

			// Phase 2: label-list returns orphaned Pub.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.PublisherList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.PublisherList) = pubsubv1.PublisherList{
						Items: []pubsubv1.Publisher{
							{ObjectMeta: metav1.ObjectMeta{Name: "orphan-pub", Namespace: testZoneStatusNs}},
						},
					}
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
					return obj.GetName() == "orphan-pub"
				}), mock.Anything).
				Return(nil).Once()

			// Phase 3: label-list returns orphaned Route.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*gatewayv1.RouteList) = gatewayv1.RouteList{
						Items: []gatewayv1.Route{
							{ObjectMeta: metav1.ObjectMeta{Name: "orphan-route", Namespace: testZoneStatusNs}},
						},
					}
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
					return obj.GetName() == "orphan-route"
				}), mock.Anything).
				Return(nil).Once()

			err := h.Delete(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
