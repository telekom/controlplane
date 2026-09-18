// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
// Copyright 2026.
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Watch-driven integration tests verify that controller watches wake up the
// reconciler within seconds when a dependency changes, instead of waiting for
// the 30-minute periodic requeue.
//
// These tests use the real controllers registered with the manager in
// suite_test.go. They create/modify real K8s objects and verify reconciliation
// happens via Eventually, without ever calling Reconcile() directly.

const (
	watchTimeout  = 30 * time.Second
	watchInterval = 500 * time.Millisecond

	watchEnv = "watch-env"
	watchNs  = "watch-ns"
	watchZNs = "watch-env--aws"
)

var _ = Describe("Watch-Driven Integration", Ordered, func() {
	var (
		watchConsumerName string
		watchProviderName string
		watchConsumerCID  string
		watchProviderCID  string
		watchSAName       string
	)

	BeforeAll(func() {
		watchConsumerName = "watch-consumer"
		watchProviderName = "watch-provider"
		watchConsumerCID = "team-watch--watch-consumer"
		watchProviderCID = "team-watch-prov--watch-provider"
		watchSAName = "watch-sa"

		// Create namespaces.
		for _, ns := range []string{watchNs, watchZNs} {
			nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
			Expect(client.IgnoreAlreadyExists(directClient.Create(ctx, nsObj))).To(Succeed())
		}

		// Zone.
		zone := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name: "aws", Namespace: watchNs,
				Labels: map[string]string{envLabelKey: watchEnv},
			},
			Spec: adminv1.ZoneSpec{
				IdentityProvider: adminv1.IdentityProviderConfig{
					Url:   "http://identity.local/auth",
					Admin: adminv1.IdentityProviderAdminConfig{ClientId: "admin", UserName: "admin", Password: "pass"},
				},
				Gateway: adminv1.GatewayConfig{
					Admin: adminv1.GatewayAdminConfig{Url: "http://gw-admin.local"},
					Presets: []adminv1.GatewayConfigPreset{{
						Name: "default", Default: true,
						Urls: []adminv1.UrlConfig{{Hostname: "gw.watch.example.com", BasePath: "/gateway"}},
					}},
				},
				Visibility: adminv1.ZoneVisibilityWorld,
			},
		}
		Expect(directClient.Create(ctx, zone)).To(Succeed())
		zone.Status = adminv1.ZoneStatus{
			Namespace:     watchZNs,
			Gateway:       &ctypes.ObjectRef{Name: "gw-aws", Namespace: watchZNs},
			IdentityRealm: &ctypes.ObjectRef{Name: "watch-realm", Namespace: watchNs},
			Conditions:    readyConditions(),
			Links: adminv1.Links{
				Url:       "http://gw.watch.example.com",
				Issuer:    "http://identity.local/auth/realms/watch-env",
				LmsIssuer: "http://identity.local/auth/realms/watch-env-lms",
			},
		}
		Expect(directClient.Status().Update(ctx, zone)).To(Succeed())

		// Realm.
		realm := &identityv1.Realm{
			ObjectMeta: metav1.ObjectMeta{
				Name: "watch-realm", Namespace: watchNs,
				Labels: map[string]string{envLabelKey: watchEnv},
			},
			Spec: identityv1.RealmSpec{
				IdentityProvider: &ctypes.ObjectRef{Name: "idp", Namespace: watchNs},
			},
		}
		Expect(directClient.Create(ctx, realm)).To(Succeed())
		realm.Status = identityv1.RealmStatus{IssuerUrl: "https://iris.example.com/auth/realms/watch"}
		Expect(directClient.Status().Update(ctx, realm)).To(Succeed())

		// EventConfig.
		ec := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ec-aws", Namespace: watchZNs,
				Labels: map[string]string{envLabelKey: watchEnv},
			},
			Spec: eventv1.EventConfigSpec{
				Zone: ctypes.ObjectRef{Name: "aws", Namespace: watchNs},
				Local: &eventv1.LocalBackend{
					Admin:              eventv1.AdminConfig{Url: "http://admin.local"},
					ServerSendEventUrl: "https://sse.local:443/api/v1/sse",
					PublishEventUrl:    "http://publish.local",
				},
			},
		}
		Expect(directClient.Create(ctx, ec)).To(Succeed())
		ec.Status = eventv1.EventConfigStatus{
			CallbackURL: "https://callback.gw.example.com/callback",
			Conditions:  readyConditions(),
			EventStore:  &ctypes.ObjectRef{Name: "es-aws", Namespace: watchZNs},
		}
		Expect(directClient.Status().Update(ctx, ec)).To(Succeed())

		// EventStore.
		es := &pubsubv1.EventStore{
			ObjectMeta: metav1.ObjectMeta{
				Name: "es-aws", Namespace: watchZNs,
				Labels: map[string]string{envLabelKey: watchEnv},
			},
			Spec: pubsubv1.EventStoreSpec{
				Url: "http://admin.local", TokenUrl: "http://token.local",
				ClientId: "cid", ClientSecret: "csec",
			},
		}
		Expect(directClient.Create(ctx, es)).To(Succeed())
		es.Status = pubsubv1.EventStoreStatus{Conditions: readyConditions()}
		Expect(directClient.Status().Update(ctx, es)).To(Succeed())

		// Consumer Application.
		consApp := &applicationv1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name: watchConsumerName, Namespace: watchNs,
				Labels: map[string]string{envLabelKey: watchEnv},
			},
			Spec: applicationv1.ApplicationSpec{
				Team: "team-watch", TeamEmail: "w@test.com", Secret: "sec",
				Zone:     ctypes.ObjectRef{Name: "aws", Namespace: watchNs},
				Failover: applicationv1.Failover{Enabled: false},
			},
		}
		Expect(directClient.Create(ctx, consApp)).To(Succeed())
		consApp.Status = applicationv1.ApplicationStatus{ClientId: watchConsumerCID, Conditions: readyConditions()}
		Expect(directClient.Status().Update(ctx, consApp)).To(Succeed())

		// Provider Application (different team for cross-team approval).
		provApp := &applicationv1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name: watchProviderName, Namespace: watchNs,
				Labels: map[string]string{envLabelKey: watchEnv},
			},
			Spec: applicationv1.ApplicationSpec{
				Team: "team-watch-prov", TeamEmail: "wp@test.com", Secret: "sec",
				Zone:     ctypes.ObjectRef{Name: "aws", Namespace: watchNs},
				Failover: applicationv1.Failover{Enabled: false},
			},
		}
		Expect(directClient.Create(ctx, provApp)).To(Succeed())
		provApp.Status = applicationv1.ApplicationStatus{ClientId: watchProviderCID, Conditions: readyConditions()}
		Expect(directClient.Status().Update(ctx, provApp)).To(Succeed())

		// Gateway Route for the listener's apiBasePath.
		gwRoute := &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: "api-v1-watch", Namespace: watchZNs,
				Labels: map[string]string{envLabelKey: watchEnv},
			},
			Spec: gatewayv1.RouteSpec{
				GatewayRef: ctypes.ObjectRef{Name: "gw-aws", Namespace: watchZNs},
				Type:       gatewayv1.RouteTypePrimary,
				Paths:      []string{"/gateway/api/v1/watch"},
				Backend: gatewayv1.Backend{
					Upstreams: []gatewayv1.Upstream{
						{Scheme: "https", Hostname: "api.watch.example.com", Port: 443, Path: "/api/v1/watch"},
					},
				},
			},
		}
		Expect(directClient.Create(ctx, gwRoute)).To(Succeed())
	})

	// -----------------------------------------------------------------------
	// Scenario 5: SpectreApplication watches — EventStore becoming Ready
	// requeues blocked SpectreApplication.
	// -----------------------------------------------------------------------
	Describe("Scenario 5: EventStore readiness requeues SpectreApplication", func() {
		const (
			s5Env    = "s5-env"
			s5Ns     = "s5-ns"
			s5ZoneNs = "s5-env--aws"
		)

		BeforeAll(func() {
			for _, ns := range []string{s5Ns, s5ZoneNs} {
				nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
				Expect(client.IgnoreAlreadyExists(directClient.Create(ctx, nsObj))).To(Succeed())
			}

			// Zone with status.
			zone := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name: "aws", Namespace: s5Ns,
					Labels: map[string]string{envLabelKey: s5Env},
				},
				Spec: adminv1.ZoneSpec{
					IdentityProvider: adminv1.IdentityProviderConfig{
						Url: "http://id.local", Admin: adminv1.IdentityProviderAdminConfig{ClientId: "a", UserName: "a", Password: "a"},
					},
					Gateway: adminv1.GatewayConfig{
						Admin: adminv1.GatewayAdminConfig{Url: "http://gw.local"},
						Presets: []adminv1.GatewayConfigPreset{{Name: "default", Default: true,
							Urls: []adminv1.UrlConfig{{Hostname: "gw.s5.example.com", BasePath: "/gateway"}}}},
					},
					Visibility: adminv1.ZoneVisibilityWorld,
				},
			}
			Expect(directClient.Create(ctx, zone)).To(Succeed())
			zone.Status = adminv1.ZoneStatus{
				Namespace: s5ZoneNs, Gateway: &ctypes.ObjectRef{Name: "gw-aws", Namespace: s5ZoneNs},
				IdentityRealm: &ctypes.ObjectRef{Name: "s5-realm", Namespace: s5Ns},
				Conditions:    readyConditions(),
				Links:         adminv1.Links{Url: "http://gw.s5.example.com", Issuer: "http://id.local/realms/s5", LmsIssuer: "http://id.local/realms/s5-lms"},
			}
			Expect(directClient.Status().Update(ctx, zone)).To(Succeed())

			// Realm.
			realm := &identityv1.Realm{
				ObjectMeta: metav1.ObjectMeta{Name: "s5-realm", Namespace: s5Ns, Labels: map[string]string{envLabelKey: s5Env}},
				Spec:       identityv1.RealmSpec{IdentityProvider: &ctypes.ObjectRef{Name: "idp", Namespace: s5Ns}},
			}
			Expect(directClient.Create(ctx, realm)).To(Succeed())
			realm.Status = identityv1.RealmStatus{IssuerUrl: "https://iris.example.com/auth/realms/s5"}
			Expect(directClient.Status().Update(ctx, realm)).To(Succeed())

			// EventConfig — references an EventStore that does NOT exist yet.
			ec := &eventv1.EventConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ec-aws", Namespace: s5ZoneNs,
					Labels: map[string]string{envLabelKey: s5Env},
				},
				Spec: eventv1.EventConfigSpec{
					Zone: ctypes.ObjectRef{Name: "aws", Namespace: s5Ns},
					Local: &eventv1.LocalBackend{
						Admin:              eventv1.AdminConfig{Url: "http://admin.local"},
						ServerSendEventUrl: "https://sse.local:443/api/v1/sse",
						PublishEventUrl:    "http://publish.local",
					},
				},
			}
			Expect(directClient.Create(ctx, ec)).To(Succeed())
			ec.Status = eventv1.EventConfigStatus{
				CallbackURL: "https://callback.s5.example.com/callback",
				Conditions:  readyConditions(),
				EventStore:  &ctypes.ObjectRef{Name: "s5-es", Namespace: s5ZoneNs},
			}
			Expect(directClient.Status().Update(ctx, ec)).To(Succeed())

			// Application.
			app := &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s5-app", Namespace: s5Ns,
					Labels: map[string]string{envLabelKey: s5Env},
				},
				Spec: applicationv1.ApplicationSpec{
					Team: "team-s5", TeamEmail: "s5@test.com", Secret: "sec",
					Zone:     ctypes.ObjectRef{Name: "aws", Namespace: s5Ns},
					Failover: applicationv1.Failover{Enabled: false},
				},
			}
			Expect(directClient.Create(ctx, app)).To(Succeed())
			app.Status = applicationv1.ApplicationStatus{ClientId: "team-s5--s5-app", Conditions: readyConditions()}
			Expect(directClient.Status().Update(ctx, app)).To(Succeed())
		})

		It("should reconcile SpectreApplication when EventStore is created and becomes Ready", func() {
			By("Creating SpectreApplication — it will be blocked because EventStore does not exist")
			sa := &spectrev1.SpectreApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s5-sa", Namespace: s5Ns,
					Labels: map[string]string{envLabelKey: s5Env},
				},
				Spec: spectrev1.SpectreApplicationSpec{
					Application: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: "s5-app", Namespace: s5Ns},
					},
					DeliveryType: "server_sent_event",
				},
			}
			Expect(directClient.Create(ctx, sa)).To(Succeed())

			By("Verifying the SA starts without Publisher children (blocked on missing EventStore)")
			// Give the controller a few seconds to try reconciling and getting blocked.
			Consistently(func(g Gomega) {
				pubList := &pubsubv1.PublisherList{}
				g.Expect(directClient.List(ctx, pubList, client.InNamespace(s5ZoneNs),
					client.MatchingLabels{envLabelKey: s5Env})).To(Succeed())
				for _, p := range pubList.Items {
					// No publisher should exist for this SA's event type yet.
					g.Expect(p.Spec.EventType).NotTo(Equal(util.BuildListenerEventType("team-s5--s5-app")))
				}
			}, 3*time.Second, 500*time.Millisecond).Should(Succeed())

			By("Creating the EventStore with Ready status — this should trigger the watch")
			es := &pubsubv1.EventStore{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s5-es", Namespace: s5ZoneNs,
					Labels: map[string]string{envLabelKey: s5Env},
				},
				Spec: pubsubv1.EventStoreSpec{
					Url: "http://admin.local", TokenUrl: "http://token.local",
					ClientId: "cid", ClientSecret: "csec",
				},
			}
			Expect(directClient.Create(ctx, es)).To(Succeed())
			es.Status = pubsubv1.EventStoreStatus{Conditions: readyConditions()}
			Expect(directClient.Status().Update(ctx, es)).To(Succeed())

			By("Verifying the SA reconciles and creates its Publisher child")
			expectedET := util.BuildListenerEventType("team-s5--s5-app")
			pubName := util.MakePublisherName(expectedET)
			Eventually(func(g Gomega) {
				pub := &pubsubv1.Publisher{}
				g.Expect(directClient.Get(ctx, types.NamespacedName{Name: pubName, Namespace: s5ZoneNs}, pub)).To(Succeed())
				g.Expect(pub.Spec.EventType).To(Equal(expectedET))
			}, watchTimeout, watchInterval).Should(Succeed())
		})
	})

	// -----------------------------------------------------------------------
	// Scenario 6: Publisher/Subscriber/RouteListener readiness changes update
	// parent readiness.
	// -----------------------------------------------------------------------
	Describe("Scenario 6: child readiness updates parent", func() {
		It("should update SpectreApplication conditions when child Publisher becomes Ready", func() {
			By("Creating a SpectreApplication and waiting for its Publisher to be created")
			sa := &spectrev1.SpectreApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s6-sa", Namespace: watchNs,
					Labels: map[string]string{envLabelKey: watchEnv},
				},
				Spec: spectrev1.SpectreApplicationSpec{
					Application: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: watchConsumerName, Namespace: watchNs},
					},
					DeliveryType: "server_sent_event",
				},
			}
			Expect(directClient.Create(ctx, sa)).To(Succeed())

			expectedET := util.BuildListenerEventType(watchConsumerCID)
			pubName := util.MakePublisherName(expectedET)

			Eventually(func(g Gomega) {
				pub := &pubsubv1.Publisher{}
				g.Expect(directClient.Get(ctx, types.NamespacedName{Name: pubName, Namespace: watchZNs}, pub)).To(Succeed())
			}, watchTimeout, watchInterval).Should(Succeed())

			By("Making the Publisher Ready — the watch should requeue the parent SA")
			Eventually(func(g Gomega) {
				pub := &pubsubv1.Publisher{}
				g.Expect(directClient.Get(ctx, types.NamespacedName{Name: pubName, Namespace: watchZNs}, pub)).To(Succeed())
				pub.Status = pubsubv1.PublisherStatus{Conditions: readyConditions()}
				g.Expect(directClient.Status().Update(ctx, pub)).To(Succeed())
			}, watchTimeout, watchInterval).Should(Succeed())

			// Also make the Subscriber and SSE route Ready to give the SA all children ready.
			subName := util.MakeSubscriberName(watchConsumerCID)
			Eventually(func(g Gomega) {
				sub := &pubsubv1.Subscriber{}
				g.Expect(directClient.Get(ctx, types.NamespacedName{Name: subName, Namespace: watchZNs}, sub)).To(Succeed())
				sub.Status = pubsubv1.SubscriberStatus{Conditions: readyConditions()}
				g.Expect(directClient.Status().Update(ctx, sub)).To(Succeed())
			}, watchTimeout, watchInterval).Should(Succeed())

			By("Verifying the SA's status.id is populated (controller reconciled)")
			Eventually(func(g Gomega) {
				updated := &spectrev1.SpectreApplication{}
				g.Expect(directClient.Get(ctx, types.NamespacedName{Name: "s6-sa", Namespace: watchNs}, updated)).To(Succeed())
				g.Expect(updated.Status.Id).To(Equal(watchConsumerCID))
				g.Expect(updated.Status.Publisher).NotTo(BeNil())
			}, watchTimeout, watchInterval).Should(Succeed())
		})
	})

	// -----------------------------------------------------------------------
	// Scenario 7: SpectreApplication status.Id changes requeue Listeners.
	// -----------------------------------------------------------------------
	Describe("Scenario 7: SpectreApplication status.Id requeues Listeners", func() {
		It("should reconcile a Listener when SpectreApplication gets status.Id", func() {
			By("Creating a SpectreApplication WITHOUT status.Id")
			sa := &spectrev1.SpectreApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name: watchSAName, Namespace: watchNs,
					Labels: map[string]string{envLabelKey: watchEnv},
				},
				Spec: spectrev1.SpectreApplicationSpec{
					Application: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: watchConsumerName, Namespace: watchNs},
					},
					DeliveryType: "server_sent_event",
				},
			}
			// The SA may already exist from scenario 6, or the SA controller may have
			// already reconciled it and set status.Id. In that case, let it be — the
			// Listener test below still proves the watch fires on SA status updates.
			Expect(client.IgnoreAlreadyExists(directClient.Create(ctx, sa))).To(Succeed())

			// Wait for the SA to have status.Id set (either by our controller or manually).
			Eventually(func(g Gomega) {
				fetched := &spectrev1.SpectreApplication{}
				g.Expect(directClient.Get(ctx, types.NamespacedName{Name: watchSAName, Namespace: watchNs}, fetched)).To(Succeed())
				g.Expect(fetched.Status.Id).NotTo(BeEmpty())
			}, watchTimeout, watchInterval).Should(Succeed())

			By("Creating a Listener that references this SpectreApplication")
			listener := &spectrev1.Listener{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s7-listener", Namespace: watchNs,
					Labels: map[string]string{envLabelKey: watchEnv},
				},
				Spec: spectrev1.ListenerSpec{
					Consumer: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: watchConsumerName, Namespace: watchNs},
					},
					Provider: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: watchProviderName, Namespace: watchNs},
					},
					Application: ctypes.ObjectRef{Name: watchSAName, Namespace: watchNs},
					ApiListener: &spectrev1.ApiListener{ApiBasePath: "/api/v1/watch"},
				},
			}
			Expect(directClient.Create(ctx, listener)).To(Succeed())

			By("Verifying the Listener controller reconciles (creates an ApprovalRequest)")
			// The Listener needs an Approval before creating children. The controller
			// will create an ApprovalRequest because the teams differ.
			Eventually(func(g Gomega) {
				arList := &approvalv1.ApprovalRequestList{}
				g.Expect(directClient.List(ctx, arList, client.InNamespace(watchNs))).To(Succeed())
				found := false
				for i := range arList.Items {
					for _, ref := range arList.Items[i].OwnerReferences {
						if ref.Name == "s7-listener" {
							found = true
						}
					}
				}
				g.Expect(found).To(BeTrue(), "ApprovalRequest for s7-listener should exist")
			}, watchTimeout, watchInterval).Should(Succeed())
		})
	})

	// -----------------------------------------------------------------------
	// Scenario 1: Approval grant provisions children without polling.
	// -----------------------------------------------------------------------
	Describe("Scenario 1: Approval grant provisions children", func() {
		It("should create RouteListener and Subscribers when Approval is granted", func() {
			By("Granting the Approval for the s7-listener")
			// Find the ApprovalRequest and grant it.
			var templateAR *approvalv1.ApprovalRequest
			Eventually(func(g Gomega) {
				arList := &approvalv1.ApprovalRequestList{}
				g.Expect(directClient.List(ctx, arList, client.InNamespace(watchNs))).To(Succeed())
				for i := range arList.Items {
					for _, ref := range arList.Items[i].OwnerReferences {
						if ref.Name == "s7-listener" {
							templateAR = &arList.Items[i]
						}
					}
				}
				g.Expect(templateAR).NotTo(BeNil())
			}, watchTimeout, watchInterval).Should(Succeed())

			approval := &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name: "listener--s7-listener", Namespace: watchNs,
					Labels: map[string]string{envLabelKey: watchEnv},
				},
				Spec: approvalv1.ApprovalSpec{
					Action:    templateAR.Spec.Action,
					Target:    templateAR.Spec.Target,
					Requester: templateAR.Spec.Requester,
					Decider:   templateAR.Spec.Decider,
					Strategy:  templateAR.Spec.Strategy,
					State:     approvalv1.ApprovalStateGranted,
					Decisions: []approvalv1.Decision{{
						Name: "System", Comment: "Auto-approved in watch test",
						ResultingState: approvalv1.ApprovalStateGranted,
					}},
					ApprovedRequest: &ctypes.ObjectRef{
						Name: templateAR.Name, Namespace: templateAR.Namespace,
					},
				},
			}
			Expect(directClient.Create(ctx, approval)).To(Succeed())

			By("Verifying RouteListener is created within a few seconds (watch-driven, no polling)")
			rlName := util.MakeRouteListenerName(watchConsumerCID, "/api/v1/watch", watchConsumerCID, watchProviderCID)
			Eventually(func(g Gomega) {
				rl := &gatewayv1.RouteListener{}
				g.Expect(directClient.Get(ctx, types.NamespacedName{Name: rlName, Namespace: watchZNs}, rl)).To(Succeed())
				g.Expect(rl.Spec.Consumer).To(Equal(watchConsumerCID))
				g.Expect(rl.Spec.ServiceOwner).To(Equal(watchProviderCID))
			}, watchTimeout, watchInterval).Should(Succeed())

			By("Verifying bridge Subscribers are created")
			rqSubId := util.MakeBridgeSubscriberId(watchConsumerCID, watchConsumerCID, "/api/v1/watch", "rq")
			Eventually(func(g Gomega) {
				sub := &pubsubv1.Subscriber{}
				g.Expect(directClient.Get(ctx, types.NamespacedName{
					Name: util.MakeSubscriberName(rqSubId), Namespace: watchZNs,
				}, sub)).To(Succeed())
			}, watchTimeout, watchInterval).Should(Succeed())

			rpSubId := util.MakeBridgeSubscriberId(watchConsumerCID, watchConsumerCID, "/api/v1/watch", "rp")
			Eventually(func(g Gomega) {
				sub := &pubsubv1.Subscriber{}
				g.Expect(directClient.Get(ctx, types.NamespacedName{
					Name: util.MakeSubscriberName(rpSubId), Namespace: watchZNs,
				}, sub)).To(Succeed())
			}, watchTimeout, watchInterval).Should(Succeed())
		})
	})

	// -----------------------------------------------------------------------
	// Scenario 2: Approval suspension removes capture promptly.
	// -----------------------------------------------------------------------
	Describe("Scenario 2: Approval suspension removes capture", func() {
		It("should delete RouteListener and Subscribers when Approval is Suspended", func() {
			rlName := util.MakeRouteListenerName(watchConsumerCID, "/api/v1/watch", watchConsumerCID, watchProviderCID)

			By("Confirming children exist from scenario 1")
			Eventually(func(g Gomega) {
				rl := &gatewayv1.RouteListener{}
				g.Expect(directClient.Get(ctx, types.NamespacedName{Name: rlName, Namespace: watchZNs}, rl)).To(Succeed())
			}, watchTimeout, watchInterval).Should(Succeed())

			By("Suspending the Approval")
			approval := &approvalv1.Approval{}
			Expect(directClient.Get(ctx, types.NamespacedName{Name: "listener--s7-listener", Namespace: watchNs}, approval)).To(Succeed())
			approval.Spec.State = approvalv1.ApprovalStateSuspended
			Expect(directClient.Update(ctx, approval)).To(Succeed())

			By("Verifying RouteListener is deleted by the watch-driven reconcile")
			Eventually(func(g Gomega) {
				rl := &gatewayv1.RouteListener{}
				err := directClient.Get(ctx, types.NamespacedName{Name: rlName, Namespace: watchZNs}, rl)
				g.Expect(err).To(HaveOccurred(), "RouteListener should be gone after approval suspension")
			}, watchTimeout, watchInterval).Should(Succeed())
		})
	})

	// -----------------------------------------------------------------------
	// Scenario 3: Route creation after Listener block requeues the Listener.
	// -----------------------------------------------------------------------
	Describe("Scenario 3: Route creation unblocks Listener", func() {
		const s3BasePath = "/api/v1/s3missing"

		It("should reconcile Listener when a missing Route is created", func() {
			By("Creating a Listener targeting a non-existent Route")
			// Use the same-team approach so approval is auto-granted.
			// Temporarily make provider same team.
			provApp := &applicationv1.Application{}
			Expect(directClient.Get(ctx, types.NamespacedName{Name: watchProviderName, Namespace: watchNs}, provApp)).To(Succeed())
			origTeam := provApp.Spec.Team
			provApp.Spec.Team = "team-watch"
			Expect(directClient.Update(ctx, provApp)).To(Succeed())

			listener := &spectrev1.Listener{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s3-listener", Namespace: watchNs,
					Labels: map[string]string{envLabelKey: watchEnv},
				},
				Spec: spectrev1.ListenerSpec{
					Consumer: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: watchConsumerName, Namespace: watchNs},
					},
					Provider: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: watchProviderName, Namespace: watchNs},
					},
					Application: ctypes.ObjectRef{Name: watchSAName, Namespace: watchNs},
					ApiListener: &spectrev1.ApiListener{ApiBasePath: s3BasePath},
				},
			}
			Expect(directClient.Create(ctx, listener)).To(Succeed())

			// Pre-create approval for same-team.
			approval := &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name: "listener--s3-listener", Namespace: watchNs,
					Labels: map[string]string{envLabelKey: watchEnv},
				},
				Spec: approvalv1.ApprovalSpec{
					Action: "listen-provider",
					Target: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Listener", APIVersion: "spectre.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: "s3-listener", Namespace: watchNs},
					},
					Requester: approvalv1.Requester{TeamName: "team-watch", TeamEmail: "w@test.com"},
					Decider:   approvalv1.Decider{TeamName: "team-watch", TeamEmail: "w@test.com"},
					Strategy:  approvalv1.ApprovalStrategyAuto,
					State:     approvalv1.ApprovalStateGranted,
					Decisions: []approvalv1.Decision{{
						Name: "System", Comment: "Auto", ResultingState: approvalv1.ApprovalStateGranted,
					}},
				},
			}
			Expect(client.IgnoreAlreadyExists(directClient.Create(ctx, approval))).To(Succeed())

			By("Verifying no RouteListener exists yet (Route is missing, Listener is blocked)")
			rlName := util.MakeRouteListenerName(watchConsumerCID, s3BasePath, watchConsumerCID, watchProviderCID)
			Consistently(func(g Gomega) {
				rl := &gatewayv1.RouteListener{}
				err := directClient.Get(ctx, types.NamespacedName{Name: rlName, Namespace: watchZNs}, rl)
				g.Expect(err).To(HaveOccurred(), "RouteListener should not exist while Route is missing")
			}, 3*time.Second, 500*time.Millisecond).Should(Succeed())

			By("Creating the missing Route — this should trigger the watch and unblock the Listener")
			route := &gatewayv1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name: util.MakeRouteName(s3BasePath), Namespace: watchZNs,
					Labels: map[string]string{envLabelKey: watchEnv},
				},
				Spec: gatewayv1.RouteSpec{
					GatewayRef: ctypes.ObjectRef{Name: "gw-aws", Namespace: watchZNs},
					Type:       gatewayv1.RouteTypePrimary,
					Paths:      []string{"/gateway" + s3BasePath},
					Backend: gatewayv1.Backend{
						Upstreams: []gatewayv1.Upstream{
							{Scheme: "https", Hostname: "api.s3.example.com", Port: 443, Path: s3BasePath},
						},
					},
				},
			}
			Expect(directClient.Create(ctx, route)).To(Succeed())

			By("Verifying RouteListener is created (Listener unblocked by Route watch)")
			Eventually(func(g Gomega) {
				rl := &gatewayv1.RouteListener{}
				g.Expect(directClient.Get(ctx, types.NamespacedName{Name: rlName, Namespace: watchZNs}, rl)).To(Succeed())
			}, watchTimeout, watchInterval).Should(Succeed())

			// Restore provider team.
			Expect(directClient.Get(ctx, types.NamespacedName{Name: watchProviderName, Namespace: watchNs}, provApp)).To(Succeed())
			provApp.Spec.Team = origTeam
			Expect(directClient.Update(ctx, provApp)).To(Succeed())
		})
	})

	// -----------------------------------------------------------------------
	// Scenario 4: Route transition to pass-through deprovisions capture.
	// -----------------------------------------------------------------------
	Describe("Scenario 4: Route pass-through deprovisions capture", func() {
		It("should remove RouteListener when Route switches to PassThrough", func() {
			rlName := util.MakeRouteListenerName(watchConsumerCID, "/api/v1/s3missing", watchConsumerCID, watchProviderCID)

			By("Confirming RouteListener exists from scenario 3")
			Eventually(func(g Gomega) {
				rl := &gatewayv1.RouteListener{}
				g.Expect(directClient.Get(ctx, types.NamespacedName{Name: rlName, Namespace: watchZNs}, rl)).To(Succeed())
			}, watchTimeout, watchInterval).Should(Succeed())

			By("Setting the Route to PassThrough")
			route := &gatewayv1.Route{}
			Expect(directClient.Get(ctx, types.NamespacedName{
				Name: util.MakeRouteName("/api/v1/s3missing"), Namespace: watchZNs,
			}, route)).To(Succeed())
			route.Spec.PassThrough = true
			Expect(directClient.Update(ctx, route)).To(Succeed())

			By("Verifying RouteListener is removed by the watch-driven reconcile")
			Eventually(func(g Gomega) {
				rl := &gatewayv1.RouteListener{}
				err := directClient.Get(ctx, types.NamespacedName{Name: rlName, Namespace: watchZNs}, rl)
				g.Expect(err).To(HaveOccurred(), "RouteListener should be deleted when Route is pass-through")
			}, watchTimeout, watchInterval).Should(Succeed())
		})
	})

	// -----------------------------------------------------------------------
	// Scenario 8 note: Rover→Spectre chain requires both Rover and Spectre
	// controllers running together with FeatureSpectre enabled. This is outside
	// the Spectre controller's own domain and would need a separate test suite
	// in rover/internal/controller/. Omitted here with a note.
	// -----------------------------------------------------------------------
	Describe("Scenario 8: Rover→Spectre chain (noted)", func() {
		It("is documented as requiring a separate rover test suite", func() {
			Skip(fmt.Sprintf(
				"Scenario 8 (Rover child status → Rover reconcile) requires the Rover " +
					"controller with FeatureSpectre enabled. This belongs in " +
					"rover/internal/controller/ with a shared envtest that registers both " +
					"Rover and Spectre controllers."))
		})
	})
})
