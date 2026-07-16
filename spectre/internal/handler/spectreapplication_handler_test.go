// SPDX-FileCopyrightText: 2026 Deutsche Telekom IT GmbH
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
	testAppName      = "my-app"
	testAppNamespace = "team-ns"
	testZoneName     = "aws"
	testZoneNs       = "env-ns"
	testZoneStatusNs = "env-ns--aws"
	testSSEUrl       = "https://horizon-sse.internal:443/api/v1/sse"
)

func newSpectreApplication(deliveryType string) *spectrev1.SpectreApplication {
	app := &spectrev1.SpectreApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa-" + testAppName,
			Namespace: testAppNamespace,
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
	}
	meta.SetStatusCondition(&ec.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return ec
}

func makeEventStore() pubsubv1.EventStore {
	return pubsubv1.EventStore{
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

	mockListEventStores := func(items []pubsubv1.EventStore) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventStoreList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*pubsubv1.EventStoreList) = pubsubv1.EventStoreList{Items: items}
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

	setupHappyPath := func(deliveryType string) *spectrev1.SpectreApplication {
		obj := newSpectreApplication(deliveryType)
		app := makeReadyApplication()
		zone := makeReadyZone()
		ec := makeReadyEventConfig()
		es := makeEventStore()

		mockGetApplication(app)
		mockGetZone(zone)
		mockListEventConfigs([]eventv1.EventConfig{ec})
		mockListEventStores([]pubsubv1.EventStore{es})
		mockCreateOrUpdatePublisher()
		mockCreateOrUpdateSubscriber()

		if deliveryType == "server_sent_event" {
			mockCreateOrUpdateRoute()
		}

		return obj
	}

	Describe("CreateOrUpdate", func() {
		Context("with SSE delivery", func() {
			It("should create Publisher with correct fields", func() {
				obj := setupHappyPath("server_sent_event")
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				// Verify status refs are set
				Expect(obj.Status.Publisher).ToNot(BeNil())
				Expect(obj.Status.Publisher.Namespace).To(Equal(testZoneStatusNs))
				Expect(obj.Status.Id).To(Equal(testAppName))
			})

			It("should create Subscriber referencing Publisher", func() {
				obj := setupHappyPath("server_sent_event")
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(obj.Status.Subscriber).ToNot(BeNil())
				Expect(obj.Status.Subscriber.Namespace).To(Equal(testZoneStatusNs))
			})

			It("should create SSE Route when delivery is server_sent_event", func() {
				obj := setupHappyPath("server_sent_event")
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(obj.Status.ListenerRoute).ToNot(BeNil())
				Expect(obj.Status.ListenerRoute.Namespace).To(Equal(testZoneStatusNs))
			})

			It("should set Ready condition when all children are ready", func() {
				obj := setupHappyPath("server_sent_event")
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(obj.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(readyCond.Reason).To(Equal(condition.ReasonProvisioned))
			})

			It("should set NotReady when AllReady returns false", func() {
				obj := setupHappyPath("server_sent_event")
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
				fakeClient.EXPECT().AllReady().Return(true).Once()

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
				mockListEventStores([]pubsubv1.EventStore{es})

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
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedPublisher).ToNot(BeNil())
				Expect(capturedPublisher.Spec.EventType).To(Equal("de.telekom.ei.listener." + testAppName))
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
				mockListEventStores([]pubsubv1.EventStore{es})
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
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedSubscriber).ToNot(BeNil())
				Expect(capturedSubscriber.Spec.SubscriberId).To(Equal(testAppName))
				Expect(capturedSubscriber.Spec.Delivery.Type).To(Equal(pubsubv1.DeliveryTypeServerSentEvent))
				Expect(capturedSubscriber.Spec.Delivery.Callback).To(BeEmpty())
			})

			It("should create Subscriber with callback URL for callback delivery", func() {
				obj := newSpectreApplication("callback")
				app := makeReadyApplication()
				zone := makeReadyZone()
				ec := makeReadyEventConfig()
				es := makeEventStore()

				mockGetApplication(app)
				mockGetZone(zone)
				mockListEventConfigs([]eventv1.EventConfig{ec})
				mockListEventStores([]pubsubv1.EventStore{es})
				mockCreateOrUpdatePublisher()

				var capturedSubscriber *pubsubv1.Subscriber
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
						capturedSubscriber = obj.(*pubsubv1.Subscriber)
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()

				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedSubscriber).ToNot(BeNil())
				Expect(capturedSubscriber.Spec.Delivery.Type).To(Equal(pubsubv1.DeliveryTypeCallback))
				Expect(capturedSubscriber.Spec.Delivery.Callback).To(Equal("https://customer.example.com/callback"))
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
				mockListEventStores([]pubsubv1.EventStore{es})
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

				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedRoute).ToNot(BeNil())
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
				Expect(capturedRoute.Spec.Paths[0]).To(ContainSubstring("/sse/v1/de.telekom.ei.listener." + testAppName))
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

			It("should return error when no EventStore found in zone namespace", func() {
				obj := newSpectreApplication("server_sent_event")
				app := makeReadyApplication()
				zone := makeReadyZone()
				ec := makeReadyEventConfig()

				mockGetApplication(app)
				mockGetZone(zone)
				mockListEventConfigs([]eventv1.EventConfig{ec})
				// Empty EventStore list
				mockListEventStores([]pubsubv1.EventStore{})

				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no EventStore found"))
			})
		})
	})
})
