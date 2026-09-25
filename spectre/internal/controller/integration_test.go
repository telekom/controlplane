// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
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

const (
	testTimeout  = 15 * time.Second
	testInterval = 250 * time.Millisecond

	envName       = "test-env"
	envLabelKey   = "cp.ei.telekom.de/environment"
	testNamespace = "default"

	// Zone
	zoneName      = "aws"
	zoneNamespace = testNamespace
	zoneStatusNs  = "test-env--aws"

	// Applications
	consumerName     = "consumer-app"
	providerName     = "provider-app"
	consumerTeamName = "team-alpha"
	providerTeamName = "team-beta"
	consumerClientID = "team-alpha--consumer-app"
	providerClientID = "team-beta--provider-app"

	// SpectreApplication
	spectreAppName = "sa-consumer-app"
	appId          = consumerClientID

	// Listener
	integrationListenerName = "test-listener"
	testBasePath            = "/api/v1/orders"
	callbackURL             = "https://callback.gateway.example.com/callback"
)

// newDrainedRecorder returns a FakeRecorder whose event channel is continuously
// drained. This is required because reconcileUntilReady calls Reconcile in a
// polling loop and each pass emits an event, so a plain FakeRecorder's buffered
// channel fills up and the next Event() call blocks forever, hanging the suite.
// The drainer stops when the channel is closed at the end of the suite.
func newDrainedRecorder(bufferSize int) *record.FakeRecorder {
	recorder := record.NewFakeRecorder(bufferSize)
	go func() {
		for range recorder.Events {
		}
	}()
	return recorder
}

// reconcileUntilReady reconciles the object in a loop until the specified check
// passes. This accounts for cache lag and multi-pass reconciliation (finalizer
// addition, blocked requeues, conflicts).
func reconcileUntilReady(
	ctx context.Context,
	r reconcile.Reconciler,
	nn types.NamespacedName,
	check func(g Gomega),
) {
	Eventually(func(g Gomega) {
		_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		check(g)
	}, testTimeout, testInterval).Should(Succeed())
}

var _ = Describe("Integration: Two-Tier Reconcile Cycle", Ordered, func() {
	var (
		ctx context.Context

		saReconciler       *SpectreApplicationReconciler
		listenerReconciler *ListenerReconciler
	)

	BeforeAll(func() {
		ctx = context.Background()

		// Create the zone status namespace (must exist for child resources).
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: zoneStatusNs}}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())

		// --- Prerequisite CRs ---

		By("Creating the Zone CR")
		zone := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      zoneName,
				Namespace: zoneNamespace,
				Labels:    map[string]string{envLabelKey: envName},
			},
			Spec: adminv1.ZoneSpec{
				IdentityProvider: adminv1.IdentityProviderConfig{
					Url: "http://identity.local/auth",
					Admin: adminv1.IdentityProviderAdminConfig{
						ClientId: "admin-client",
						UserName: "admin",
						Password: "admin-pass",
					},
				},
				Gateway: adminv1.GatewayConfig{
					Admin: adminv1.GatewayAdminConfig{
						Url: "http://gateway-admin.local",
					},
					Presets: []adminv1.GatewayConfigPreset{
						{
							Name:    "default",
							Default: true,
							Urls: []adminv1.UrlConfig{
								{Hostname: "gateway.test.example.com", BasePath: "/gateway"},
							},
						},
					},
				},
				Visibility: adminv1.ZoneVisibilityWorld,
			},
		}
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())

		// Create the identity Realm (prerequisite for resolveGatewayCredentials).
		By("Creating the identity Realm CR")
		realm := &identityv1.Realm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-realm",
				Namespace: zoneNamespace,
				Labels:    map[string]string{envLabelKey: envName},
			},
			Spec: identityv1.RealmSpec{
				IdentityProvider: &ctypes.ObjectRef{Name: "idp", Namespace: zoneNamespace},
			},
		}
		Expect(k8sClient.Create(ctx, realm)).To(Succeed())

		realm.Status = identityv1.RealmStatus{
			IssuerUrl: "https://iris.example.com/auth/realms/test",
		}
		Expect(k8sClient.Status().Update(ctx, realm)).To(Succeed())

		// Set Zone status (simulates the admin controller).
		zone.Status = adminv1.ZoneStatus{
			Namespace: zoneStatusNs,
			Gateway: &ctypes.ObjectRef{
				Name:      "gateway-aws",
				Namespace: zoneStatusNs,
			},
			IdentityRealm: &ctypes.ObjectRef{
				Name:      "test-realm",
				Namespace: zoneNamespace,
			},
			Conditions: readyConditions(),
			Links: adminv1.Links{
				Url:       "http://gateway.test.example.com",
				Issuer:    "http://identity.local/auth/realms/test-env",
				LmsIssuer: "http://identity.local/auth/realms/test-env-lms",
			},
		}
		Expect(k8sClient.Status().Update(ctx, zone)).To(Succeed())

		By("Creating the EventConfig CR")
		eventConfig := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ec-aws",
				Namespace: zoneStatusNs,
				Labels:    map[string]string{envLabelKey: envName},
			},
			Spec: eventv1.EventConfigSpec{
				Zone: ctypes.ObjectRef{Name: zoneName, Namespace: zoneNamespace},
				Local: &eventv1.LocalBackend{
					Admin:              eventv1.AdminConfig{Url: "http://admin.local"},
					ServerSendEventUrl: "https://horizon-sse.internal:443/api/v1/sse",
					PublishEventUrl:    "http://publish.local",
				},
			},
		}
		Expect(k8sClient.Create(ctx, eventConfig)).To(Succeed())

		// Set EventConfig status.
		eventConfig.Status = eventv1.EventConfigStatus{
			CallbackURL: callbackURL,
			Conditions:  readyConditions(),
			EventStore: &ctypes.ObjectRef{
				Name:      "eventstore-aws",
				Namespace: zoneStatusNs,
			},
		}
		Expect(k8sClient.Status().Update(ctx, eventConfig)).To(Succeed())

		By("Creating the EventStore CR")
		eventStore := &pubsubv1.EventStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "eventstore-aws",
				Namespace: zoneStatusNs,
				Labels:    map[string]string{envLabelKey: envName},
			},
			Spec: pubsubv1.EventStoreSpec{
				Url:          "http://admin.local",
				TokenUrl:     "http://token.local",
				ClientId:     "client-id",
				ClientSecret: "client-secret",
			},
		}
		Expect(k8sClient.Create(ctx, eventStore)).To(Succeed())

		eventStore.Status = pubsubv1.EventStoreStatus{
			Conditions: readyConditions(),
		}
		Expect(k8sClient.Status().Update(ctx, eventStore)).To(Succeed())

		By("Creating ApiExposure CRs (provider binding verification)")
		// ApiExposures live in the team namespace (same as the provider
		// Application), not in the zone namespace where Routes live.
		ordersExposure := &apiv1.ApiExposure{
			ObjectMeta: metav1.ObjectMeta{
				Name:      providerName + "--api-v1-orders",
				Namespace: testNamespace,
				Labels: map[string]string{
					envLabelKey:                          envName,
					cconfig.BuildLabelKey("application"): providerName,
				},
			},
			Spec: apiv1.ApiExposureSpec{
				ApiBasePath: testBasePath,
				Upstreams:   []apiv1.Upstream{{Url: "https://api.provider.example.com"}},
				Visibility:  apiv1.VisibilityZone,
				Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
				Zone:        ctypes.ObjectRef{Name: zoneName, Namespace: zoneNamespace},
			},
		}
		Expect(k8sClient.Create(ctx, ordersExposure)).To(Succeed())
		ordersExposure.Status = apiv1.ApiExposureStatus{
			Active: true,
			Route:  &ctypes.ObjectRef{Name: "api-v1-orders", Namespace: zoneStatusNs},
		}
		Expect(k8sClient.Status().Update(ctx, ordersExposure)).To(Succeed())

		crossExposure := &apiv1.ApiExposure{
			ObjectMeta: metav1.ObjectMeta{
				Name:      providerName + "--api-v1-cross",
				Namespace: testNamespace,
				Labels: map[string]string{
					envLabelKey:                          envName,
					cconfig.BuildLabelKey("application"): providerName,
				},
			},
			Spec: apiv1.ApiExposureSpec{
				ApiBasePath: "/api/v1/cross",
				Upstreams:   []apiv1.Upstream{{Url: "https://api.cross.example.com"}},
				Visibility:  apiv1.VisibilityZone,
				Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
				Zone:        ctypes.ObjectRef{Name: zoneName, Namespace: zoneNamespace},
			},
		}
		Expect(k8sClient.Create(ctx, crossExposure)).To(Succeed())
		crossExposure.Status = apiv1.ApiExposureStatus{
			Active: true,
			Route:  &ctypes.ObjectRef{Name: "api-v1-cross", Namespace: zoneStatusNs},
		}
		Expect(k8sClient.Status().Update(ctx, crossExposure)).To(Succeed())

		By("Creating gateway Route CRs (prerequisites for RouteListener path resolution)")
		gatewayRoute := &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-v1-orders",
				Namespace: zoneStatusNs,
				Labels: map[string]string{
					envLabelKey:              envName,
					cconfig.OwnerUidLabelKey: string(ordersExposure.UID),
				},
			},
			Spec: gatewayv1.RouteSpec{
				GatewayRef: ctypes.ObjectRef{Name: "gateway-aws", Namespace: zoneStatusNs},
				Type:       gatewayv1.RouteTypePrimary,
				Paths:      []string{"/gateway" + testBasePath},
				Backend: gatewayv1.Backend{
					Upstreams: []gatewayv1.Upstream{
						{Scheme: "https", Hostname: "api.provider.example.com", Port: 443, Path: testBasePath},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, gatewayRoute)).To(Succeed())

		crossRoute := &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-v1-cross",
				Namespace: zoneStatusNs,
				Labels: map[string]string{
					envLabelKey:              envName,
					cconfig.OwnerUidLabelKey: string(crossExposure.UID),
				},
			},
			Spec: gatewayv1.RouteSpec{
				GatewayRef: ctypes.ObjectRef{Name: "gateway-aws", Namespace: zoneStatusNs},
				Type:       gatewayv1.RouteTypePrimary,
				Paths:      []string{"/gateway/api/v1/cross"},
				Backend: gatewayv1.Backend{
					Upstreams: []gatewayv1.Upstream{
						{Scheme: "https", Hostname: "api.cross.example.com", Port: 443, Path: "/api/v1/cross"},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, crossRoute)).To(Succeed())

		By("Creating the consumer Application CR")
		consumerApp := &applicationv1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      consumerName,
				Namespace: testNamespace,
				Labels:    map[string]string{envLabelKey: envName},
			},
			Spec: applicationv1.ApplicationSpec{
				Team:      consumerTeamName,
				TeamEmail: "alpha@test.com",
				Secret:    "consumer-secret",
				Zone:      ctypes.ObjectRef{Name: zoneName, Namespace: zoneNamespace},
				Failover:  applicationv1.Failover{Enabled: false},
			},
		}
		Expect(k8sClient.Create(ctx, consumerApp)).To(Succeed())

		consumerApp.Status = applicationv1.ApplicationStatus{
			ClientId:   consumerClientID,
			Conditions: readyConditions(),
		}
		Expect(k8sClient.Status().Update(ctx, consumerApp)).To(Succeed())

		By("Creating the provider Application CR")
		providerApp := &applicationv1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      providerName,
				Namespace: testNamespace,
				Labels:    map[string]string{envLabelKey: envName},
			},
			Spec: applicationv1.ApplicationSpec{
				Team:      providerTeamName,
				TeamEmail: "beta@test.com",
				Secret:    "provider-secret",
				Zone:      ctypes.ObjectRef{Name: zoneName, Namespace: zoneNamespace},
				Failover:  applicationv1.Failover{Enabled: false},
			},
		}
		Expect(k8sClient.Create(ctx, providerApp)).To(Succeed())

		providerApp.Status = applicationv1.ApplicationStatus{
			ClientId:   providerClientID,
			Conditions: readyConditions(),
		}
		Expect(k8sClient.Status().Update(ctx, providerApp)).To(Succeed())

		// --- Reconcilers ---
		recorder := newDrainedRecorder(100)
		saReconciler = &SpectreApplicationReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: recorder,
		}
		saReconciler.Controller = cc.NewController(&handler.SpectreApplicationHandler{}, k8sClient, recorder)

		listenerRecorder := newDrainedRecorder(100)
		listenerReconciler = &ListenerReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: listenerRecorder,
		}
		listenerReconciler.Controller = cc.NewController(&handler.ListenerHandler{Reader: testMgr.GetAPIReader()}, k8sClient, listenerRecorder)

		// --- SpectreApplication (prerequisite for Listener tests) ---
		By("Creating the SpectreApplication CR")
		sa := &spectrev1.SpectreApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name:      spectreAppName,
				Namespace: testNamespace,
				Labels:    map[string]string{envLabelKey: envName},
			},
			Spec: spectrev1.SpectreApplicationSpec{
				Application: ctypes.TypedObjectRef{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Application",
						APIVersion: "application.cp.ei.telekom.de/v1",
					},
					ObjectRef: ctypes.ObjectRef{
						Name:      consumerName,
						Namespace: testNamespace,
					},
				},
				DeliveryType: "server_sent_event",
			},
		}
		Expect(k8sClient.Create(ctx, sa)).To(Succeed())

		saNN := types.NamespacedName{Name: spectreAppName, Namespace: testNamespace}
		expectedEventType := util.BuildListenerEventType(appId)
		publisherName := util.MakePublisherName(expectedEventType)

		By("Reconciling SpectreApplication until per-app Publisher exists")
		reconcileUntilReady(ctx, saReconciler, saNN, func(g Gomega) {
			publisher := &pubsubv1.Publisher{}
			err := directClient.Get(ctx, types.NamespacedName{
				Name: publisherName, Namespace: zoneStatusNs,
			}, publisher)
			g.Expect(err).NotTo(HaveOccurred())
		})

		By("Waiting for SpectreApplication status to be visible in cache")
		Eventually(func(g Gomega) {
			cachedSA := &spectrev1.SpectreApplication{}
			g.Expect(directClient.Get(ctx, saNN, cachedSA)).To(Succeed())
			g.Expect(cachedSA.Status.Id).NotTo(BeEmpty())
		}, testTimeout, testInterval).Should(Succeed())
	})

	Describe("SpectreApplication reconcile", func() {
		It("should have created per-app Publisher, Subscriber, and SSE Route", func() {
			expectedEventType := util.BuildListenerEventType(appId)
			publisherName := util.MakePublisherName(expectedEventType)

			By("Verifying per-app Publisher fields")
			publisher := &pubsubv1.Publisher{}
			Expect(directClient.Get(ctx, types.NamespacedName{
				Name: publisherName, Namespace: zoneStatusNs,
			}, publisher)).To(Succeed())
			Expect(publisher.Spec.EventType).To(Equal(expectedEventType))
			Expect(publisher.Spec.PublisherId).To(Equal(util.PublisherID))
			Expect(publisher.Spec.EventStore.Name).To(Equal("eventstore-aws"))

			By("Verifying Subscriber exists")
			subscriberName := util.MakeSubscriberName(appId)
			subscriber := &pubsubv1.Subscriber{}
			Eventually(func(g Gomega) {
				err := directClient.Get(ctx, types.NamespacedName{
					Name: subscriberName, Namespace: zoneStatusNs,
				}, subscriber)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(subscriber.Spec.Publisher.Name).To(Equal(publisherName))
				g.Expect(subscriber.Spec.SubscriberId).To(Equal(appId))
				g.Expect(subscriber.Spec.Delivery.Type).To(Equal(pubsubv1.DeliveryTypeServerSentEvent))
			}, testTimeout, testInterval).Should(Succeed())

			By("Verifying SSE Route exists")
			routeList := &gatewayv1.RouteList{}
			Eventually(func(g Gomega) {
				err := directClient.List(ctx, routeList, client.InNamespace(zoneStatusNs))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(routeList.Items).NotTo(BeEmpty())

				// Find the SSE route for our app (must have DisableAccessControl set).
				var found *gatewayv1.Route
				for i := range routeList.Items {
					r := &routeList.Items[i]
					if r.Spec.GatewayRef.Name == "gateway-aws" && r.Spec.Security.DisableAccessControl {
						found = r
						break
					}
				}
				g.Expect(found).NotTo(BeNil(), "SSE Route not found")
				g.Expect(found.Spec.Security.DisableAccessControl).To(BeTrue())
				g.Expect(found.Spec.Buffering.DisableResponseBuffering).To(BeTrue())
			}, testTimeout, testInterval).Should(Succeed())

			By("Verifying SpectreApplication status is updated")
			saNN := types.NamespacedName{Name: spectreAppName, Namespace: testNamespace}
			updatedSA := &spectrev1.SpectreApplication{}
			Expect(directClient.Get(ctx, saNN, updatedSA)).To(Succeed())
			Expect(updatedSA.Status.Id).To(Equal(appId))
			Expect(updatedSA.Status.Publisher).NotTo(BeNil())
			Expect(updatedSA.Status.Subscriber).NotTo(BeNil())
		})
	})

	Describe("Listener reconcile with same-team approval (auto-grant)", func() {
		It("should create RouteListener, generic Publisher, and bridge Subscribers when consumer team owns the listener", func() {
			By("Creating a Listener with same-team consumer (auto-approved)")
			// For same-team approval (auto), consumer and provider belong to the same team.
			// Override provider app to be same team.
			sameTeamProvider := &applicationv1.Application{}
			Expect(directClient.Get(ctx, types.NamespacedName{Name: providerName, Namespace: testNamespace}, sameTeamProvider)).To(Succeed())
			sameTeamProvider.Spec.Team = consumerTeamName
			sameTeamProvider.Spec.TeamEmail = "alpha@test.com"
			Expect(k8sClient.Update(ctx, sameTeamProvider)).To(Succeed())

			listener := &spectrev1.Listener{
				ObjectMeta: metav1.ObjectMeta{
					Name:      integrationListenerName,
					Namespace: testNamespace,
					Labels:    map[string]string{envLabelKey: envName},
				},
				Spec: spectrev1.ListenerSpec{
					Consumer: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: consumerName, Namespace: testNamespace},
					},
					Provider: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: providerName, Namespace: testNamespace},
					},
					Application: ctypes.ObjectRef{Name: spectreAppName, Namespace: testNamespace},
					ApiListener: &spectrev1.ApiListener{
						ApiBasePath: testBasePath,
					},
				},
			}
			Expect(k8sClient.Create(ctx, listener)).To(Succeed())

			listenerNN := types.NamespacedName{Name: integrationListenerName, Namespace: testNamespace}

			By("Waiting for Listener to be visible in cache")
			Eventually(func() error {
				return k8sClient.Get(ctx, listenerNN, &spectrev1.Listener{})
			}, testTimeout, testInterval).Should(Succeed())

			By("Reconciling until ApprovalRequests exist, then granting approvals")
			// The dual-gate path creates scoped ApprovalRequests. Let the
			// reconciler create them, then grant both gates before proceeding.
			Eventually(func(g Gomega) {
				_, _ = listenerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: listenerNN})
				arList := &approvalv1.ApprovalRequestList{}
				g.Expect(directClient.List(ctx, arList, client.InNamespace(testNamespace))).To(Succeed())
				count := 0
				for i := range arList.Items {
					for _, ref := range arList.Items[i].OwnerReferences {
						if ref.Name == integrationListenerName {
							count++
						}
					}
				}
				g.Expect(count).To(BeNumerically(">=", 2), "Both gate ApprovalRequests should exist")
			}, testTimeout, testInterval).Should(Succeed())
			grantApprovalsForListener(ctx, integrationListenerName, true)

			By("Reconciling until generic Publisher exists (approvals granted)")
			genericPublisherName := util.MakePublisherName(util.GenericEventType)
			reconcileUntilReady(ctx, listenerReconciler, listenerNN, func(g Gomega) {
				genericPub := &pubsubv1.Publisher{}
				err := directClient.Get(ctx, types.NamespacedName{
					Name: genericPublisherName, Namespace: zoneStatusNs,
				}, genericPub)
				g.Expect(err).NotTo(HaveOccurred())
			})

			By("Verifying generic Publisher fields")
			genericPub := &pubsubv1.Publisher{}
			Expect(directClient.Get(ctx, types.NamespacedName{
				Name: genericPublisherName, Namespace: zoneStatusNs,
			}, genericPub)).To(Succeed())
			Expect(genericPub.Spec.EventType).To(Equal(util.GenericEventType))
			Expect(genericPub.Spec.PublisherId).To(Equal(util.PublisherID))

			By("Verifying RouteListener exists with correct fields")
			rlName := util.MakeRouteListenerName(appId, testBasePath, consumerClientID, providerClientID)
			rl := &gatewayv1.RouteListener{}
			Eventually(func(g Gomega) {
				err := directClient.Get(ctx, types.NamespacedName{
					Name: rlName, Namespace: zoneStatusNs,
				}, rl)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rl.Spec.Consumer).To(Equal(consumerClientID))
				g.Expect(rl.Spec.ServiceOwner).To(Equal(providerClientID))
				g.Expect(rl.Spec.Issue).To(Equal(testBasePath))
				g.Expect(rl.Spec.Zone.Name).To(Equal(zoneName))
			}, testTimeout, testInterval).Should(Succeed())

			By("Verifying two bridge Subscribers exist with correct selection filters")
			rqSubId := util.MakeBridgeSubscriberId(consumerClientID, appId, testBasePath, "rq")
			rpSubId := util.MakeBridgeSubscriberId(consumerClientID, appId, testBasePath, "rp")

			rqSub := &pubsubv1.Subscriber{}
			Eventually(func(g Gomega) {
				err := directClient.Get(ctx, types.NamespacedName{
					Name: util.MakeSubscriberName(rqSubId), Namespace: zoneStatusNs,
				}, rqSub)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rqSub.Spec.Delivery.Type).To(Equal(pubsubv1.DeliveryTypeCallback))
				expectedCallback, cbErr := util.BuildBridgeCallbackURL(callbackURL, appId)
				g.Expect(cbErr).NotTo(HaveOccurred())
				g.Expect(rqSub.Spec.Delivery.Callback).To(Equal(expectedCallback))
				g.Expect(rqSub.Spec.Trigger).NotTo(BeNil())
				g.Expect(rqSub.Spec.Trigger.SelectionFilter).NotTo(BeNil())
				g.Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["issue"]).To(Equal(testBasePath))
				g.Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["consumer"]).To(Equal(consumerClientID))
				g.Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["provider"]).To(Equal(providerClientID))
				g.Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["kind"]).To(Equal("REQUEST"))
			}, testTimeout, testInterval).Should(Succeed())

			rpSub := &pubsubv1.Subscriber{}
			Eventually(func(g Gomega) {
				err := directClient.Get(ctx, types.NamespacedName{
					Name: util.MakeSubscriberName(rpSubId), Namespace: zoneStatusNs,
				}, rpSub)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rpSub.Spec.Trigger.SelectionFilter.Attributes["kind"]).To(Equal("RESPONSE"))
			}, testTimeout, testInterval).Should(Succeed())

			By("Setting children Ready (no downstream controllers in envtest)")
			// RouteListener
			rlList := &gatewayv1.RouteListenerList{}
			Eventually(func(g Gomega) {
				g.Expect(directClient.List(ctx, rlList, client.InNamespace(zoneStatusNs))).To(Succeed())
				g.Expect(rlList.Items).NotTo(BeEmpty())
			}, testTimeout, testInterval).Should(Succeed())
			for i := range rlList.Items {
				rl := &rlList.Items[i]
				rl.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "ready"))
				Expect(directClient.Status().Update(ctx, rl)).To(Succeed())
			}
			// Bridge Subscribers
			rqSub.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "ready"))
			Expect(directClient.Status().Update(ctx, rqSub)).To(Succeed())
			rpSub.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "ready"))
			Expect(directClient.Status().Update(ctx, rpSub)).To(Succeed())

			By("Verifying Listener status is updated")
			updatedListener := &spectrev1.Listener{}
			Eventually(func(g Gomega) {
				g.Expect(directClient.Get(ctx, listenerNN, updatedListener)).To(Succeed())
				g.Expect(updatedListener.Status.RouteListener).NotTo(BeNil())
				g.Expect(updatedListener.Status.EventSubscriptions).To(HaveLen(2))
				g.Expect(updatedListener.Status.ProviderApproval).NotTo(BeNil())
			}, testTimeout, testInterval).Should(Succeed())
		})
	})

	Describe("Listener reconcile requeue cycle", func() {
		It("should block when EventConfig or Zone prerequisites are missing", func() {
			By("Creating a Listener referencing a non-existent zone via a missing application")
			missingApp := &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-zone-app",
					Namespace: testNamespace,
					Labels:    map[string]string{envLabelKey: envName},
				},
				Spec: applicationv1.ApplicationSpec{
					Team:      "team-missing",
					TeamEmail: "missing@test.com",
					Secret:    "secret",
					Zone:      ctypes.ObjectRef{Name: "nonexistent-zone", Namespace: testNamespace},
					Failover:  applicationv1.Failover{Enabled: false},
				},
			}
			Expect(k8sClient.Create(ctx, missingApp)).To(Succeed())
			missingApp.Status = applicationv1.ApplicationStatus{
				ClientId:   "team-missing--missing-zone-app",
				Conditions: readyConditions(),
			}
			Expect(k8sClient.Status().Update(ctx, missingApp)).To(Succeed())

			blockedListener := &spectrev1.Listener{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "blocked-listener",
					Namespace: testNamespace,
					Labels:    map[string]string{envLabelKey: envName},
				},
				Spec: spectrev1.ListenerSpec{
					Consumer: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: "missing-zone-app", Namespace: testNamespace},
					},
					Provider: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: providerName, Namespace: testNamespace},
					},
					Application: ctypes.ObjectRef{Name: spectreAppName, Namespace: testNamespace},
					ApiListener: &spectrev1.ApiListener{
						ApiBasePath: "/api/v1/blocked",
					},
				},
			}
			Expect(k8sClient.Create(ctx, blockedListener)).To(Succeed())

			blockedNN := types.NamespacedName{Name: "blocked-listener", Namespace: testNamespace}

			By("Reconciling — should not error but should not create downstream resources")
			// The first reconcile only adds the finalizer via an Update and requeues,
			// so the reconciler's cached client may still serve the pre-Update version
			// on the next call and lose the status write with a conflict. Retry until
			// the cache has caught up and the blocked path is reached cleanly.
			// Blocked errors are handled internally by the controller (no returned error).
			Eventually(func(g Gomega) {
				_, err := listenerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: blockedNN})
				g.Expect(err).NotTo(HaveOccurred())
			}, testTimeout, testInterval).Should(Succeed())

			By("Verifying no RouteListener was created for the blocked Listener")
			rlList := &gatewayv1.RouteListenerList{}
			Expect(directClient.List(ctx, rlList, client.InNamespace(zoneStatusNs))).To(Succeed())
			for _, rl := range rlList.Items {
				Expect(rl.Spec.Issue).NotTo(Equal("/api/v1/blocked"),
					"RouteListener should not be created when zone is missing")
			}
		})
	})

	Describe("Listener reconcile with cross-team approval gate", func() {
		It("should block until approvals are granted, then create downstream resources", func() {
			By("Restoring provider to different team for cross-team scenario")
			providerApp := &applicationv1.Application{}
			Expect(directClient.Get(ctx, types.NamespacedName{Name: providerName, Namespace: testNamespace}, providerApp)).To(Succeed())
			providerApp.Spec.Team = providerTeamName
			providerApp.Spec.TeamEmail = "beta@test.com"
			Expect(k8sClient.Update(ctx, providerApp)).To(Succeed())

			By("Creating a cross-team Listener")
			crossTeamListener := &spectrev1.Listener{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cross-team-listener",
					Namespace: testNamespace,
					Labels:    map[string]string{envLabelKey: envName},
				},
				Spec: spectrev1.ListenerSpec{
					Consumer: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: consumerName, Namespace: testNamespace},
					},
					Provider: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: providerName, Namespace: testNamespace},
					},
					Application: ctypes.ObjectRef{Name: spectreAppName, Namespace: testNamespace},
					ApiListener: &spectrev1.ApiListener{
						ApiBasePath: "/api/v1/cross",
					},
				},
			}
			Expect(k8sClient.Create(ctx, crossTeamListener)).To(Succeed())

			crossNN := types.NamespacedName{Name: "cross-team-listener", Namespace: testNamespace}
			crossRL := util.MakeRouteListenerName(appId, "/api/v1/cross", consumerClientID, providerClientID)

			By("Waiting for cross-team Listener to be visible in cache")
			Eventually(func() error {
				return k8sClient.Get(ctx, crossNN, &spectrev1.Listener{})
			}, testTimeout, testInterval).Should(Succeed())

			By("Granting the provider approval the way the real approval controller does")
			// The first reconcile only adds the finalizer and returns early
			// (common/pkg/controller.FirstSetup), so reconcile until the
			// ApprovalRequest actually exists, then grant it with ApprovedRequest
			// set — exactly what the approval controller does.
			Eventually(func(g Gomega) {
				_, _ = listenerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: crossNN})
				arList := &approvalv1.ApprovalRequestList{}
				g.Expect(directClient.List(ctx, arList, client.InNamespace(testNamespace))).To(Succeed())
				found := false
				for i := range arList.Items {
					for _, ref := range arList.Items[i].OwnerReferences {
						if ref.Name == "cross-team-listener" {
							found = true
						}
					}
				}
				g.Expect(found).To(BeTrue(), "ApprovalRequest for cross-team-listener should exist")
			}, testTimeout, testInterval).Should(Succeed())

			By("Keeping capture blocked while the Granted Approvals have no controller owner")
			grantApprovalsForListener(ctx, "cross-team-listener", false)
			reconcileOwnerless := func() error {
				_, err := listenerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: crossNN})
				return err
			}
			// Wait for the cache to serve the ownerless Approvals to the builder.
			Eventually(reconcileOwnerless, testTimeout, testInterval).
				Should(MatchError(ContainSubstring("missing or foreign controller owner")))
			for range 3 {
				Expect(reconcileOwnerless()).To(MatchError(ContainSubstring("missing or foreign controller owner")))
				l := &spectrev1.Listener{}
				Expect(directClient.Get(ctx, crossNN, l)).To(Succeed())
				Expect(meta.IsStatusConditionTrue(l.Status.Conditions, condition.ConditionTypeReady)).To(BeFalse())
				Expect(l.Status.RouteListener).To(BeNil())
				Expect(apierrors.IsNotFound(directClient.Get(ctx,
					types.NamespacedName{Name: crossRL, Namespace: zoneStatusNs}, &gatewayv1.RouteListener{}))).To(BeTrue())
			}

			By("Replacing the ownerless Approvals with ones the approval controller would write")
			for _, ar := range listenerApprovalRequests(ctx, "cross-team-listener") {
				approvalName, err := approvalv1.ScopedApprovalName(ar.Spec.Target, ar.Spec.ApprovalKey)
				Expect(err).NotTo(HaveOccurred())
				key := types.NamespacedName{Name: approvalName, Namespace: testNamespace}
				Expect(directClient.Delete(ctx, &approvalv1.Approval{
					ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
				})).To(Succeed())
				Eventually(func() bool {
					return apierrors.IsNotFound(directClient.Get(ctx, key, &approvalv1.Approval{}))
				}, testTimeout, testInterval).Should(BeTrue())
			}
			grantApprovalsForListener(ctx, "cross-team-listener", true)

			By("Reconciling until RouteListener is created (approval pre-granted)")
			reconcileUntilReady(ctx, listenerReconciler, crossNN, func(g Gomega) {
				rlCheck := &gatewayv1.RouteListener{}
				err := directClient.Get(ctx, types.NamespacedName{Name: crossRL, Namespace: zoneStatusNs}, rlCheck)
				g.Expect(err).NotTo(HaveOccurred())
			})

			By("Verifying RouteListener fields")
			rl := &gatewayv1.RouteListener{}
			Eventually(func(g Gomega) {
				err := directClient.Get(ctx, types.NamespacedName{Name: crossRL, Namespace: zoneStatusNs}, rl)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rl.Spec.Consumer).To(Equal(consumerClientID))
				g.Expect(rl.Spec.ServiceOwner).To(Equal(providerClientID))
				g.Expect(rl.Spec.Issue).To(Equal("/api/v1/cross"))
			}, testTimeout, testInterval).Should(Succeed())

			By("Verifying bridge Subscribers exist for cross-team Listener")
			rqSubId := util.MakeBridgeSubscriberId(consumerClientID, appId, "/api/v1/cross", "rq")
			rqSub := &pubsubv1.Subscriber{}
			Eventually(func(g Gomega) {
				err := directClient.Get(ctx, types.NamespacedName{
					Name: util.MakeSubscriberName(rqSubId), Namespace: zoneStatusNs,
				}, rqSub)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["kind"]).To(Equal("REQUEST"))
			}, testTimeout, testInterval).Should(Succeed())

			rpSubId := util.MakeBridgeSubscriberId(consumerClientID, appId, "/api/v1/cross", "rp")
			rpSub := &pubsubv1.Subscriber{}
			Eventually(func(g Gomega) {
				err := directClient.Get(ctx, types.NamespacedName{
					Name: util.MakeSubscriberName(rpSubId), Namespace: zoneStatusNs,
				}, rpSub)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rpSub.Spec.Trigger.SelectionFilter.Attributes["kind"]).To(Equal("RESPONSE"))
			}, testTimeout, testInterval).Should(Succeed())
		})
	})

	Describe("Listener deletion", func() {
		It("should drain the RouteListener, then the bridge Subscribers, and release the finalizer only after the drain", func() {
			// The children live in the zone namespace while the Listener lives in
			// the team namespace, so Kubernetes owner references cannot garbage
			// collect them. The Delete handler must remove them explicitly.
			listenerNN := types.NamespacedName{Name: "cross-team-listener", Namespace: testNamespace}
			rlKey := types.NamespacedName{
				Name:      util.MakeRouteListenerName(appId, "/api/v1/cross", consumerClientID, providerClientID),
				Namespace: zoneStatusNs,
			}
			rqKey := types.NamespacedName{
				Name:      util.MakeSubscriberName(util.MakeBridgeSubscriberId(consumerClientID, appId, "/api/v1/cross", "rq")),
				Namespace: zoneStatusNs,
			}
			rpKey := types.NamespacedName{
				Name:      util.MakeSubscriberName(util.MakeBridgeSubscriberId(consumerClientID, appId, "/api/v1/cross", "rp")),
				Namespace: zoneStatusNs,
			}
			reconcileDeletion := func() {
				_, _ = listenerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: listenerNN})
			}

			By("Confirming the children exist before deletion")
			Expect(directClient.Get(ctx, rlKey, &gatewayv1.RouteListener{})).To(Succeed())
			Expect(directClient.Get(ctx, rpKey, &pubsubv1.Subscriber{})).To(Succeed())

			By("Holding the rq Subscriber behind a finalizer, as the pubsub controller does while it deregisters")
			const holdFinalizer = "test.cp.ei.telekom.de/hold"
			Eventually(func(g Gomega) {
				sub := &pubsubv1.Subscriber{}
				g.Expect(directClient.Get(ctx, rqKey, sub)).To(Succeed())
				controllerutil.AddFinalizer(sub, holdFinalizer)
				g.Expect(directClient.Update(ctx, sub)).To(Succeed())
			}, testTimeout, testInterval).Should(Succeed())

			By("Deleting the Listener")
			listener := &spectrev1.Listener{}
			Expect(directClient.Get(ctx, listenerNN, listener)).To(Succeed())
			Expect(directClient.Delete(ctx, listener)).To(Succeed())

			By("Waiting for the RouteListener to go while the persisted checkpoint drains the Subscribers")
			Eventually(func(g Gomega) {
				reconcileDeletion()
				g.Expect(apierrors.IsNotFound(directClient.Get(ctx, rlKey, &gatewayv1.RouteListener{}))).
					To(BeTrue(), "RouteListener should be deleted, not orphaned")
				g.Expect(apierrors.IsNotFound(directClient.Get(ctx, rpKey, &pubsubv1.Subscriber{}))).
					To(BeTrue(), "unheld bridge Subscriber should be deleted")
				held := &pubsubv1.Subscriber{}
				g.Expect(directClient.Get(ctx, rqKey, held)).To(Succeed())
				g.Expect(held.DeletionTimestamp).NotTo(BeNil(), "held bridge Subscriber should be terminating")
				current := &spectrev1.Listener{}
				g.Expect(directClient.Get(ctx, listenerNN, current)).To(Succeed())
				g.Expect(current.Status.Draining).NotTo(BeNil())
				g.Expect(current.Status.Draining.Phase).To(Equal(handler.DrainPhaseDrainingSubscribers))
				g.Expect(current.Status.RouteListener).To(BeNil())
				// Ready follows the drain; it does not freeze at the first phase.
				ready := meta.FindStatusCondition(current.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.Reason).To(Equal("Deleting"))
				g.Expect(ready.Message).To(ContainSubstring("phase " + handler.DrainPhaseDrainingSubscribers))
			}, testTimeout, testInterval).Should(Succeed())

			By("Keeping the Listener finalizer while the held Subscriber finalizes")
			Consistently(func(g Gomega) {
				reconcileDeletion()
				current := &spectrev1.Listener{}
				g.Expect(directClient.Get(ctx, listenerNN, current)).To(Succeed())
				g.Expect(current.Finalizers).To(ContainElement(cconfig.FinalizerName))
				g.Expect(current.Status.Draining).NotTo(BeNil())
			}, 2*time.Second, testInterval).Should(Succeed())

			By("Releasing the held Subscriber")
			Eventually(func(g Gomega) {
				sub := &pubsubv1.Subscriber{}
				g.Expect(directClient.Get(ctx, rqKey, sub)).To(Succeed())
				controllerutil.RemoveFinalizer(sub, holdFinalizer)
				g.Expect(directClient.Update(ctx, sub)).To(Succeed())
			}, testTimeout, testInterval).Should(Succeed())

			By("Waiting for the Listener to be gone with every child")
			Eventually(func(g Gomega) {
				reconcileDeletion()
				g.Expect(apierrors.IsNotFound(directClient.Get(ctx, listenerNN, &spectrev1.Listener{}))).
					To(BeTrue(), "Listener should be gone once the drain completes")
			}, testTimeout, testInterval).Should(Succeed())
			Expect(apierrors.IsNotFound(directClient.Get(ctx, rlKey, &gatewayv1.RouteListener{}))).To(BeTrue())
			for _, key := range []types.NamespacedName{rqKey, rpKey} {
				Expect(apierrors.IsNotFound(directClient.Get(ctx, key, &pubsubv1.Subscriber{}))).
					To(BeTrue(), "bridge Subscriber %q should be deleted, not orphaned", key.Name)
			}
		})
	})

	Describe("Listener reconcile when a current ApprovalRequest is rejected", func() {
		It("should drain the RouteListener and bridge Subscribers and keep them gone", func() {
			const (
				rejectedListenerName = "rejected-listener"
				rejectedBasePath     = "/api/v1/rejected"
				rejectedRouteName    = "api-v1-rejected"
			)

			By("Creating the ApiExposure and Route for the rejected Listener's API")
			exposure := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName + "--api-v1-rejected",
					Namespace: testNamespace,
					Labels: map[string]string{
						envLabelKey:                          envName,
						cconfig.BuildLabelKey("application"): providerName,
					},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: rejectedBasePath,
					Upstreams:   []apiv1.Upstream{{Url: "https://api.rejected.example.com"}},
					Visibility:  apiv1.VisibilityZone,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
					Zone:        ctypes.ObjectRef{Name: zoneName, Namespace: zoneNamespace},
				},
			}
			Expect(k8sClient.Create(ctx, exposure)).To(Succeed())
			exposure.Status = apiv1.ApiExposureStatus{
				Active: true,
				Route:  &ctypes.ObjectRef{Name: rejectedRouteName, Namespace: zoneStatusNs},
			}
			Expect(k8sClient.Status().Update(ctx, exposure)).To(Succeed())

			route := &gatewayv1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rejectedRouteName,
					Namespace: zoneStatusNs,
					Labels: map[string]string{
						envLabelKey:              envName,
						cconfig.OwnerUidLabelKey: string(exposure.UID),
					},
				},
				Spec: gatewayv1.RouteSpec{
					GatewayRef: ctypes.ObjectRef{Name: "gateway-aws", Namespace: zoneStatusNs},
					Type:       gatewayv1.RouteTypePrimary,
					Paths:      []string{"/gateway" + rejectedBasePath},
					Backend: gatewayv1.Backend{
						Upstreams: []gatewayv1.Upstream{
							{Scheme: "https", Hostname: "api.rejected.example.com", Port: 443, Path: rejectedBasePath},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, route)).To(Succeed())

			By("Creating a cross-team Listener")
			listener := &spectrev1.Listener{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rejectedListenerName,
					Namespace: testNamespace,
					Labels:    map[string]string{envLabelKey: envName},
				},
				Spec: spectrev1.ListenerSpec{
					Consumer: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: consumerName, Namespace: testNamespace},
					},
					Provider: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: providerName, Namespace: testNamespace},
					},
					Application: ctypes.ObjectRef{Name: spectreAppName, Namespace: testNamespace},
					ApiListener: &spectrev1.ApiListener{ApiBasePath: rejectedBasePath},
				},
			}
			Expect(k8sClient.Create(ctx, listener)).To(Succeed())

			nn := types.NamespacedName{Name: rejectedListenerName, Namespace: testNamespace}
			rlKey := types.NamespacedName{
				Name:      util.MakeRouteListenerName(appId, rejectedBasePath, consumerClientID, providerClientID),
				Namespace: zoneStatusNs,
			}
			subKeys := []types.NamespacedName{
				{Name: util.MakeSubscriberName(util.MakeBridgeSubscriberId(consumerClientID, appId, rejectedBasePath, "rq")), Namespace: zoneStatusNs},
				{Name: util.MakeSubscriberName(util.MakeBridgeSubscriberId(consumerClientID, appId, rejectedBasePath, "rp")), Namespace: zoneStatusNs},
			}

			// ownedRequest returns the Listener's current ApprovalRequest for key.
			ownedRequest := func(g Gomega, key string) *approvalv1.ApprovalRequest {
				arList := &approvalv1.ApprovalRequestList{}
				g.Expect(directClient.List(ctx, arList, client.InNamespace(testNamespace))).To(Succeed())
				var found *approvalv1.ApprovalRequest
				for i := range arList.Items {
					ar := &arList.Items[i]
					owner := metav1.GetControllerOf(ar)
					if ar.Spec.ApprovalKey == key && owner != nil && owner.Name == rejectedListenerName {
						found = ar
					}
				}
				g.Expect(found).NotTo(BeNil(), "no %s-gate ApprovalRequest owned by %s", key, rejectedListenerName)
				return found
			}

			By("Granting both gates and waiting for capture to be provisioned")
			Eventually(func(g Gomega) {
				_, _ = listenerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				ownedRequest(g, "provider")
				ownedRequest(g, "consumer")
			}, testTimeout, testInterval).Should(Succeed())
			grantApprovalsForListener(ctx, rejectedListenerName, true)
			reconcileUntilReady(ctx, listenerReconciler, nn, func(g Gomega) {
				g.Expect(directClient.Get(ctx, rlKey, &gatewayv1.RouteListener{})).To(Succeed())
				for _, key := range subKeys {
					g.Expect(directClient.Get(ctx, key, &pubsubv1.Subscriber{})).To(Succeed())
				}
			})

			By("Rejecting the provider gate's current ApprovalRequest")
			Eventually(func(g Gomega) {
				ar := ownedRequest(g, "provider")
				ar.Spec.State = approvalv1.ApprovalStateRejected
				g.Expect(k8sClient.Update(ctx, ar)).To(Succeed())
			}, testTimeout, testInterval).Should(Succeed())

			// expectDrained asserts the capture children are gone and the Listener
			// reports the rejection without a pending drain.
			expectDrained := func(g Gomega) {
				_, _ = listenerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				g.Expect(apierrors.IsNotFound(directClient.Get(ctx, rlKey, &gatewayv1.RouteListener{}))).
					To(BeTrue(), "RouteListener should be drained")
				for _, key := range subKeys {
					g.Expect(apierrors.IsNotFound(directClient.Get(ctx, key, &pubsubv1.Subscriber{}))).
						To(BeTrue(), "bridge Subscriber %q should be drained", key.Name)
				}
				current := &spectrev1.Listener{}
				g.Expect(directClient.Get(ctx, nn, current)).To(Succeed())
				g.Expect(current.Status.Draining).To(BeNil())
				ready := meta.FindStatusCondition(current.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.Reason).To(Equal(condition.ReasonAccessDenied))
				g.Expect(ready.Message).To(ContainSubstring("provider gate"))
			}

			By("Waiting for the capture to drain")
			Eventually(expectDrained, testTimeout, testInterval).Should(Succeed())

			By("Verifying the capture stays gone while the request remains rejected")
			Consistently(expectDrained, 2*time.Second, testInterval).Should(Succeed())
		})
	})
})

// readyConditions returns a standard Ready=True condition slice for test fixtures.
func readyConditions() []metav1.Condition {
	return []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Ready",
			LastTransitionTime: metav1.Now(),
		},
	}
}

// targetControllerRef is the controller reference the approval controller
// writes on every scoped Approval it creates for target
// (approvalrequest handler setControllerReferenceForRef).
func targetControllerRef(t *ctypes.TypedObjectRef) []metav1.OwnerReference {
	isController := true
	return []metav1.OwnerReference{{
		APIVersion:         t.APIVersion,
		Kind:               t.Kind,
		Name:               t.Name,
		UID:                t.UID,
		Controller:         &isController,
		BlockOwnerDeletion: &isController,
	}}
}

// listenerApprovalRequests returns all ApprovalRequests owned by the Listener.
func listenerApprovalRequests(ctx context.Context, listenerName string) []*approvalv1.ApprovalRequest {
	arList := &approvalv1.ApprovalRequestList{}
	Expect(directClient.List(ctx, arList, client.InNamespace(testNamespace))).To(Succeed())

	var ownedARs []*approvalv1.ApprovalRequest
	for i := range arList.Items {
		ar := &arList.Items[i]
		for _, ref := range ar.OwnerReferences {
			if ref.Name == listenerName {
				ownedARs = append(ownedARs, ar)
				break
			}
		}
	}
	Expect(ownedARs).NotTo(BeEmpty(), "No ApprovalRequests found for listener %q", listenerName)
	return ownedARs
}

// grantApprovalsForListener simulates the approval controller by finding all
// scoped ApprovalRequests owned by the Listener and creating the matching
// scoped Approvals. Each gate ("provider", "consumer") gets its own Approval
// with a name computed by ScopedApprovalName(target, key). withControllerOwner
// writes the controller reference the real approval controller sets; false
// creates ownerless Approvals that must not authorize capture.
func grantApprovalsForListener(ctx context.Context, listenerName string, withControllerOwner bool) {
	for _, ar := range listenerApprovalRequests(ctx, listenerName) {
		// Compute the scoped Approval name from the target and key.
		approvalName, err := approvalv1.ScopedApprovalName(ar.Spec.Target, ar.Spec.ApprovalKey)
		Expect(err).NotTo(HaveOccurred(), "failed to compute ScopedApprovalName for key %q", ar.Spec.ApprovalKey)

		var ownerRefs []metav1.OwnerReference
		if withControllerOwner {
			ownerRefs = targetControllerRef(&ar.Spec.Target)
		}
		approval := &approvalv1.Approval{
			ObjectMeta: metav1.ObjectMeta{
				Name:            approvalName,
				Namespace:       testNamespace,
				Labels:          ar.Labels,
				OwnerReferences: ownerRefs,
			},
			Spec: approvalv1.ApprovalSpec{
				Action:      ar.Spec.Action,
				Target:      ar.Spec.Target,
				Requester:   ar.Spec.Requester,
				Decider:     ar.Spec.Decider,
				Strategy:    ar.Spec.Strategy,
				ApprovalKey: ar.Spec.ApprovalKey,
				State:       approvalv1.ApprovalStateGranted,
				Decisions: []approvalv1.Decision{
					{
						Name:           "System",
						Comment:        "Auto-approved in test",
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
				ApprovedRequest: &ctypes.ObjectRef{
					Name:      ar.Name,
					Namespace: ar.Namespace,
					UID:       ar.UID,
				},
			},
		}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, approval))).To(Succeed())
	}
}
