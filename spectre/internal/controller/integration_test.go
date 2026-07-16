// SPDX-FileCopyrightText: 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"
	"github.com/telekom/controlplane/spectre/internal/handler/util"
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
	appId          = "consumer-app"

	// Listener
	integrationListenerName = "test-listener"
	testBasePath            = "/api/v1/orders"
	callbackURL             = "https://callback.gateway.example.com/callback"
)

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
								{Hostname: "gateway.test.example.com", BasePath: "/"},
							},
						},
					},
				},
				Visibility: adminv1.ZoneVisibilityWorld,
			},
		}
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())

		// Set Zone status (simulates the admin controller).
		zone.Status = adminv1.ZoneStatus{
			Namespace: zoneStatusNs,
			Gateway: &ctypes.ObjectRef{
				Name:      "gateway-aws",
				Namespace: zoneStatusNs,
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
		recorder := record.NewFakeRecorder(100)
		saReconciler = &SpectreApplicationReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: recorder,
		}
		saReconciler.Controller = cc.NewController(&handler.SpectreApplicationHandler{}, k8sClient, recorder)

		listenerRecorder := record.NewFakeRecorder(100)
		listenerReconciler = &ListenerReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: listenerRecorder,
		}
		listenerReconciler.Controller = cc.NewController(&handler.ListenerHandler{}, k8sClient, listenerRecorder)

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

				// Find the SSE route for our app.
				var found *gatewayv1.Route
				for i := range routeList.Items {
					r := &routeList.Items[i]
					if r.Spec.GatewayRef.Name == "gateway-aws" {
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

			By("Pre-creating Approval CR (simulates approval controller granting before reconcile)")
			// Pre-create the Approval so the builder's Get finds it immediately as Granted.
			// This bypasses the ApprovalRequest creation + cleanup race in envtest.
			approval := &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "listener--" + integrationListenerName,
					Namespace: testNamespace,
					Labels:    map[string]string{envLabelKey: envName},
				},
				Spec: approvalv1.ApprovalSpec{
					Action: "listen-provider",
					Target: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Listener", APIVersion: "spectre.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: integrationListenerName, Namespace: testNamespace},
					},
					Requester: approvalv1.Requester{TeamName: consumerTeamName, TeamEmail: "alpha@test.com"},
					Decider:   approvalv1.Decider{TeamName: consumerTeamName, TeamEmail: "alpha@test.com"},
					Strategy:  approvalv1.ApprovalStrategyAuto,
					State:     approvalv1.ApprovalStateGranted,
					Decisions: []approvalv1.Decision{{
						Name: "System", Comment: "Auto-approved", ResultingState: approvalv1.ApprovalStateGranted,
					}},
				},
			}
			Expect(k8sClient.Create(ctx, approval)).To(Succeed())

			By("Reconciling until generic Publisher exists (approval pre-granted)")
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
				g.Expect(rqSub.Spec.Delivery.Callback).To(Equal(util.BuildBridgeCallbackURL(callbackURL, appId)))
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

			By("Verifying Listener status is updated")
			updatedListener := &spectrev1.Listener{}
			Expect(directClient.Get(ctx, listenerNN, updatedListener)).To(Succeed())
			Expect(updatedListener.Status.RouteListener).NotTo(BeNil())
			Expect(updatedListener.Status.EventSubscriptions).To(HaveLen(2))
			Expect(updatedListener.Status.ProviderApproval).NotTo(BeNil())
			Expect(updatedListener.Status.ConsumerApproval).NotTo(BeNil())
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
					ApiListener: &spectrev1.ApiListener{
						ApiBasePath: "/api/v1/blocked",
					},
				},
			}
			Expect(k8sClient.Create(ctx, blockedListener)).To(Succeed())

			blockedNN := types.NamespacedName{Name: "blocked-listener", Namespace: testNamespace}

			By("Reconciling — should not error but should not create downstream resources")
			// First reconcile adds finalizer.
			_, _ = listenerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: blockedNN})
			// Second reconcile hits the blocked path.
			_, err := listenerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: blockedNN})
			// Blocked errors are handled internally by the controller (no returned error).
			Expect(err).NotTo(HaveOccurred())

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

			By("Pre-creating Approval CR for cross-team Listener")
			crossApproval := &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "listener--cross-team-listener",
					Namespace: testNamespace,
					Labels:    map[string]string{envLabelKey: envName},
				},
				Spec: approvalv1.ApprovalSpec{
					Action: "listen-provider",
					Target: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Listener", APIVersion: "spectre.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: "cross-team-listener", Namespace: testNamespace},
					},
					Requester: approvalv1.Requester{TeamName: consumerTeamName, TeamEmail: "alpha@test.com"},
					Decider:   approvalv1.Decider{TeamName: providerTeamName, TeamEmail: "beta@test.com"},
					Strategy:  approvalv1.ApprovalStrategySimple,
					State:     approvalv1.ApprovalStateGranted,
					Decisions: []approvalv1.Decision{{
						Name: "admin", Comment: "Granted in test", ResultingState: approvalv1.ApprovalStateGranted,
					}},
				},
			}
			Expect(k8sClient.Create(ctx, crossApproval)).To(Succeed())

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

// grantApprovalsForListener simulates the approval controller by creating the single
// Approval CR for a Listener. The approval builder looks for an Approval named
// "listener--<listenerName>" (lowercase kind + "--" + owner name).
func grantApprovalsForListener(ctx context.Context, listenerName string) {
	approvalName := "listener--" + listenerName

	// Find any ApprovalRequest owned by this listener to copy fields from.
	arList := &approvalv1.ApprovalRequestList{}
	Expect(directClient.List(ctx, arList, client.InNamespace(testNamespace))).To(Succeed())

	var templateAR *approvalv1.ApprovalRequest
	for i := range arList.Items {
		ar := &arList.Items[i]
		for _, ref := range ar.OwnerReferences {
			if ref.Name == listenerName {
				templateAR = ar
				break
			}
		}
		if templateAR != nil {
			break
		}
	}
	Expect(templateAR).NotTo(BeNil(), "No ApprovalRequest found for listener %q", listenerName)

	approval := &approvalv1.Approval{
		ObjectMeta: metav1.ObjectMeta{
			Name:      approvalName,
			Namespace: testNamespace,
			Labels:    templateAR.Labels,
		},
		Spec: approvalv1.ApprovalSpec{
			Action:    templateAR.Spec.Action,
			Target:    templateAR.Spec.Target,
			Requester: templateAR.Spec.Requester,
			Decider:   templateAR.Spec.Decider,
			Strategy:  templateAR.Spec.Strategy,
			State:     approvalv1.ApprovalStateGranted,
			Decisions: []approvalv1.Decision{
				{
					Name:           "System",
					Comment:        "Auto-approved in test",
					ResultingState: approvalv1.ApprovalStateGranted,
				},
			},
			ApprovedRequest: &ctypes.ObjectRef{
				Name:      templateAR.Name,
				Namespace: templateAR.Namespace,
			},
		},
	}
	Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, approval))).To(Succeed())
}
