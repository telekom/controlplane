// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// --- Migration test constants ---

const (
	migListenerName = "mig-listener"
	migNamespace    = "team-ns"
	migListenerUID  = k8stypes.UID("mig-listener-uid-001")
)

// legacyApprovalName returns the legacy naming convention for a Listener.
func legacyApprovalName(listenerName string) string {
	return "listener--" + listenerName
}

// newMigrationListener creates a Listener fixture for migration tests.
func newMigrationListener() *spectrev1.Listener {
	return &spectrev1.Listener{
		ObjectMeta: metav1.ObjectMeta{
			Name:      migListenerName,
			Namespace: migNamespace,
			UID:       migListenerUID,
		},
		Spec: spectrev1.ListenerSpec{
			Consumer: ctypes.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
				ObjectRef: ctypes.ObjectRef{
					Name:      consumerAppName,
					Namespace: migNamespace,
				},
			},
			Provider: ctypes.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
				ObjectRef: ctypes.ObjectRef{
					Name:      providerAppName,
					Namespace: migNamespace,
				},
			},
			Application: ctypes.ObjectRef{
				Name:      spectreAppName,
				Namespace: migNamespace,
			},
			ApiListener: &spectrev1.ApiListener{
				ApiBasePath: testApiBasePath,
			},
		},
	}
}

// makeLegacyApproval creates a legacy unscoped Approval for the given Listener.
func makeLegacyApproval(listener *spectrev1.Listener, state approvalv1.ApprovalState) *approvalv1.Approval {
	isController := true
	return &approvalv1.Approval{
		ObjectMeta: metav1.ObjectMeta{
			Name:            legacyApprovalName(listener.Name),
			Namespace:       listener.Namespace,
			UID:             "legacy-approval-uid-001",
			ResourceVersion: "100",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "spectre.cp.ei.telekom.de/v1",
					Kind:               "Listener",
					Name:               listener.Name,
					UID:                listener.UID,
					Controller:         &isController,
					BlockOwnerDeletion: &isController,
				},
			},
		},
		Spec: approvalv1.ApprovalSpec{
			Action: "listen-provider",
			Target: ctypes.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Listener",
					APIVersion: "spectre.cp.ei.telekom.de/v1",
				},
				ObjectRef: ctypes.ObjectRef{
					Name:      listener.Name,
					Namespace: listener.Namespace,
					UID:       listener.UID,
				},
			},
			Requester: approvalv1.Requester{
				TeamName:  consumerTeam,
				TeamEmail: consumerEmail,
			},
			Decider: approvalv1.Decider{
				TeamName:  providerTeam,
				TeamEmail: providerEmail,
			},
			Strategy: approvalv1.ApprovalStrategyAuto,
			State:    state,
			Decisions: []approvalv1.Decision{
				{
					Name:           approvalv1.SystemDecisionName,
					Comment:        approvalv1.AutoApprovedComment,
					ResultingState: approvalv1.ApprovalStateGranted,
				},
			},
			ApprovedRequest: &ctypes.ObjectRef{
				Name:      migListenerName + "--legacy-hash",
				Namespace: migNamespace,
			},
			// No ApprovalKey — legacy unscoped.
		},
		Status: approvalv1.ApprovalStatus{
			LastState: approvalv1.ApprovalStatePending,
		},
	}
}

// makeLegacyRequest creates a legacy ApprovalRequest bound to the Approval.
func makeLegacyRequest(listener *spectrev1.Listener) *approvalv1.ApprovalRequest {
	isController := true
	return &approvalv1.ApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:            migListenerName + "--legacy-hash",
			Namespace:       migNamespace,
			UID:             "legacy-ar-uid-001",
			ResourceVersion: "50",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "spectre.cp.ei.telekom.de/v1",
					Kind:               "Listener",
					Name:               listener.Name,
					UID:                listener.UID,
					Controller:         &isController,
					BlockOwnerDeletion: &isController,
				},
			},
		},
		Spec: approvalv1.ApprovalRequestSpec{
			Action: "listen-provider",
			Target: ctypes.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Listener",
					APIVersion: "spectre.cp.ei.telekom.de/v1",
				},
				ObjectRef: ctypes.ObjectRef{
					Name:      listener.Name,
					Namespace: listener.Namespace,
					UID:       listener.UID,
				},
			},
			Strategy: approvalv1.ApprovalStrategyAuto,
			State:    approvalv1.ApprovalStateGranted,
		},
		Status: approvalv1.ApprovalRequestStatus{
			Approval: ctypes.ObjectRef{
				Name:      legacyApprovalName(listener.Name),
				Namespace: listener.Namespace,
			},
		},
	}
}

// oldCaptureRef is a RouteListener of the old capture still recorded in status.
func oldCaptureRef() *ctypes.ObjectRef {
	return &ctypes.ObjectRef{Name: "old-rl", Namespace: migNamespace, UID: "old-rl-uid"}
}

// makeDualGranted returns a dualApprovalResult with both gates granted.
func makeDualGranted() *handler.DualApprovalResult {
	return handler.NewDualApprovalResult(handler.OutcomeGranted, nil)
}

// makeDualPending returns a dualApprovalResult with pending outcome.
func makeDualPending() *handler.DualApprovalResult {
	return handler.NewDualApprovalResult(handler.OutcomePending, nil)
}

var _ = Describe("Authorization Migration", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *handler.ListenerHandler
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &handler.ListenerHandler{}

		scheme = runtime.NewScheme()
		_ = spectrev1.AddToScheme(scheme)
		_ = approvalv1.AddToScheme(scheme)
		_ = applicationv1.AddToScheme(scheme)
		_ = gatewayv1.AddToScheme(scheme)
		_ = pubsubv1.AddToScheme(scheme)
		fakeClient.EXPECT().Scheme().Return(scheme).Maybe()
	})

	// --- Mock helpers for migration ---

	mockLegacyApprovalNotFound := func() {
		legacyName := legacyApprovalName(migListenerName)
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: legacyName, Namespace: migNamespace},
				mock.AnythingOfType("*v1.Approval")).
			Return(errors.NewNotFound(
				schema.GroupResource{Group: "approval.cp.ei.telekom.de", Resource: "approvals"},
				legacyName)).
			Once()
	}

	mockLegacyApprovalExists := func(approval *approvalv1.Approval) {
		legacyName := legacyApprovalName(migListenerName)
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: legacyName, Namespace: migNamespace},
				mock.AnythingOfType("*v1.Approval")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*approvalv1.Approval) = *approval
			}).
			Return(nil).Once()
	}

	mockLegacyRequestExists := func(ar *approvalv1.ApprovalRequest) {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: ar.Name, Namespace: ar.Namespace},
				mock.AnythingOfType("*v1.ApprovalRequest")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*approvalv1.ApprovalRequest) = *ar
			}).
			Return(nil).Once()
	}

	mockLegacyRequestNotFound := func(name, ns string) {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: name, Namespace: ns},
				mock.AnythingOfType("*v1.ApprovalRequest")).
			Return(errors.NewNotFound(
				schema.GroupResource{Group: "approval.cp.ei.telekom.de", Resource: "approvalrequests"},
				name)).
			Once()
	}

	mockDeleteApprovalRequest := func(name string) {
		fakeClient.EXPECT().
			Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
				return obj.GetName() == name
			}), mock.Anything).
			Return(nil).Once()
	}

	mockDeleteApproval := func(name string) {
		fakeClient.EXPECT().
			Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
				return obj.GetName() == name
			}), mock.Anything).
			Return(nil).Once()
	}

	// mockStartDrainLists sets up List mocks for the startDrain snapshot.
	mockStartDrainLists := func() {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{}
			}).
			Return(nil).Once()
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
			}).
			Return(nil).Once()
	}

	Describe("IsFreshInstall", func() {
		It("should return true when AuthorizationPolicyVersion is v2", func() {
			listener := newMigrationListener()
			listener.Status.AuthorizationPolicyVersion = "v2"

			fresh, err := h.IsFreshInstall(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(fresh).To(BeTrue())
		})

		It("should return false when migration is in progress", func() {
			listener := newMigrationListener()
			listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
				Phase: "Recorded",
			}

			fresh, err := h.IsFreshInstall(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(fresh).To(BeFalse())
		})

		It("should return true when no legacy Approval exists and no old children", func() {
			listener := newMigrationListener()
			mockLegacyApprovalNotFound()

			// No children at all.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{}
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
				}).
				Return(nil).Once()

			fresh, err := h.IsFreshInstall(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(fresh).To(BeTrue())
		})

		It("should return false when legacy Approval exists", func() {
			listener := newMigrationListener()
			approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
			mockLegacyApprovalExists(approval)

			fresh, err := h.IsFreshInstall(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(fresh).To(BeFalse())
		})

		It("should return false when unlabelled RouteListener children exist", func() {
			listener := newMigrationListener()
			mockLegacyApprovalNotFound()

			// Return a RouteListener without the authorization fingerprint label.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{
						Items: []gatewayv1.RouteListener{
							{ObjectMeta: metav1.ObjectMeta{Name: "old-rl", Namespace: migNamespace}},
						},
					}
				}).
				Return(nil).Once()

			fresh, err := h.IsFreshInstall(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(fresh).To(BeFalse())
		})

		It("should return false when unlabelled Subscriber children exist", func() {
			listener := newMigrationListener()
			mockLegacyApprovalNotFound()

			// RouteListeners all have the fingerprint label.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{
						Items: []gatewayv1.RouteListener{
							{ObjectMeta: metav1.ObjectMeta{
								Name:      "labelled-rl",
								Namespace: migNamespace,
								Labels:    map[string]string{handler.AuthorizationFingerprintLabelKey: "fp-abc"},
							}},
						},
					}
				}).
				Return(nil).Once()

			// But a Subscriber lacks the label.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
						Items: []pubsubv1.Subscriber{
							{ObjectMeta: metav1.ObjectMeta{Name: "old-sub", Namespace: migNamespace}},
						},
					}
				}).
				Return(nil).Once()

			fresh, err := h.IsFreshInstall(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(fresh).To(BeFalse())
		})

		It("should return true when all children have fingerprint labels", func() {
			listener := newMigrationListener()
			mockLegacyApprovalNotFound()

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{
						Items: []gatewayv1.RouteListener{
							{ObjectMeta: metav1.ObjectMeta{
								Name:      "labelled-rl",
								Namespace: migNamespace,
								Labels:    map[string]string{handler.AuthorizationFingerprintLabelKey: "fp-abc"},
							}},
						},
					}
				}).
				Return(nil).Once()

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
						Items: []pubsubv1.Subscriber{
							{ObjectMeta: metav1.ObjectMeta{
								Name:      "labelled-sub",
								Namespace: migNamespace,
								Labels:    map[string]string{handler.AuthorizationFingerprintLabelKey: "fp-abc"},
							}},
						},
					}
				}).
				Return(nil).Once()

			fresh, err := h.IsFreshInstall(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(fresh).To(BeTrue())
		})
	})

	Describe("AdvanceMigration", func() {
		var (
			listener *spectrev1.Listener
			intent   handler.AuthorizationIntent
		)

		BeforeEach(func() {
			listener = newMigrationListener()
			intent = handler.NewTestIntent()
		})

		It("should return true immediately when already at v2", func() {
			listener.Status.AuthorizationPolicyVersion = "v2"

			done, err := h.AdvanceMigration(ctx, listener, &intent, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeTrue())
		})

		It("should block when no legacy evidence exists but isFreshInstall was false", func() {
			mockLegacyApprovalNotFound()

			done, err := h.AdvanceMigration(ctx, listener, &intent, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse())
			// Should NOT set v2 — block instead.
			Expect(listener.Status.AuthorizationPolicyVersion).ToNot(Equal("v2"))
		})

		Context("with granted legacy Approval", func() {
			var (
				approval *approvalv1.Approval
				request  *approvalv1.ApprovalRequest
			)

			BeforeEach(func() {
				approval = makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request = makeLegacyRequest(listener)
			})

			It("should record legacy evidence and wait for scoped grants", func() {
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)

				done, err := h.AdvanceMigration(ctx, listener, &intent, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.AuthorizationMigration).ToNot(BeNil())
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("AwaitingScoped"))
				Expect(listener.Status.AuthorizationMigration.LegacyApproval).ToNot(BeNil())
				Expect(listener.Status.AuthorizationMigration.LegacyApproval.Name).To(Equal(legacyApprovalName(migListenerName)))
			})

			It("should not complete when dual-gate is pending", func() {
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualPending())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("AwaitingScoped"))
			})

			It("should advance to Draining when dual-gate is granted", func() {
				listener.Status.RouteListener = oldCaptureRef()
				// Discovery: Approval + ApprovalRequest.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// The drain needs List mocks for its snapshot.
				mockStartDrainLists()

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("Draining"))
				Expect(listener.Status.Draining).ToNot(BeNil())
			})
		})

		Context("draining phase", func() {
			var (
				approval *approvalv1.Approval
				request  *approvalv1.ApprovalRequest
			)

			BeforeEach(func() {
				approval = makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request = makeLegacyRequest(listener)
			})

			It("should start the shared drain when Draining is nil and old capture remains", func() {
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "Draining",
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
				}
				listener.Status.RouteListener = oldCaptureRef()

				// Discovery.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// Drain snapshot List mocks.
				mockStartDrainLists()

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.Draining).ToNot(BeNil())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(listener.Status.Draining.Reason).To(Equal("legacy migration"))
				Expect(listener.Status.Draining.OldRouteListeners).To(Equal([]ctypes.ObjectRef{*oldCaptureRef()}))
				Expect(listener.Status.AuthorizationMigration.DrainStarted).To(BeTrue())
				fakeClient.AssertNumberOfCalls(GinkgoT(), "Delete", 0)
			})

			It("should advance to RetiringRequests without an empty drain when nothing is left", func() {
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "Draining",
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
				}

				// Discovery.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// Drain inventory: nothing owned, tracked or applied (the handler's
				// stale-child check already drained the prior-policy children).
				mockStartDrainLists()
				// Retirement only checkpoints the request's deletion.
				mockLegacyRequestExists(request)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.Draining).To(BeNil())
				migration := listener.Status.AuthorizationMigration
				Expect(migration.DrainStarted).To(BeFalse())
				Expect(migration.Phase).To(Equal("RetiringRequests"))
				Expect(migration.RetirementCheckpoint).ToNot(BeNil())
				Expect(migration.RetirementCheckpoint.PendingDeletions).To(HaveLen(1))
				Expect(migration.RetirementCheckpoint.PendingDeletions[0].Phase).To(Equal(handler.ExportPendingDeletionPhasePrepared))
				fakeClient.AssertNumberOfCalls(GinkgoT(), "Delete", 0)
			})

			It("should wait for continueDrain to complete", func() {
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "Draining",
					DrainStarted:        true,
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
				}
				// Simulate drain already started, in Stopping phase with a RouteListener.
				listener.Status.Draining = &spectrev1.ListenerDrainStatus{
					Phase:            handler.ExportDrainPhaseStopping,
					Reason:           "legacy migration",
					OldRouteListener: &ctypes.ObjectRef{Name: "old-rl", Namespace: migNamespace, UID: "old-rl-uid"},
				}

				// Discovery.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// continueDrain: Get old RouteListener — still exists.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "old-rl", Namespace: migNamespace},
						mock.AnythingOfType("*v1.RouteListener")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						rl := out.(*gatewayv1.RouteListener)
						rl.Name = "old-rl"
						rl.Namespace = migNamespace
						rl.UID = "old-rl-uid"
					}).
					Return(nil).Once()
				// continueDrain: Delete old RouteListener (with UID+RV preconditions).
				fakeClient.EXPECT().
					Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
						return obj.GetName() == "old-rl"
					}), mock.AnythingOfType("client.Preconditions")).
					Return(nil).Once()

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse()) // drain still in progress
			})

			It("should advance to RetiringRequests when drain completes", func() {
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "Draining",
					DrainStarted:        true,
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
				}
				// Simulate drain at CleaningPublisher with no source publisher.
				listener.Status.Draining = &spectrev1.ListenerDrainStatus{
					Phase:  handler.ExportDrainPhaseCleaningPublisher,
					Reason: "legacy migration",
				}

				// Discovery.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// continueDrain: CleaningPublisher with no SourcePublisher -> complete.
				// Retirement: records PendingDeletion for request, returns false.
				mockLegacyRequestExists(request)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("RetiringRequests"))
				Expect(listener.Status.Draining).To(BeNil()) // drain cleared
			})
		})

		Context("per-resource retirement checkpoints", func() {
			var (
				approval *approvalv1.Approval
				request  *approvalv1.ApprovalRequest
			)

			BeforeEach(func() {
				approval = makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request = makeLegacyRequest(listener)
			})

			It("should record PendingDeletion before deleting ApprovalRequest", func() {
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringRequests",
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
				}

				// Discovery.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// retireLegacyRequests: fresh read.
				mockLegacyRequestExists(request)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse()) // returned to persist checkpoint
				cp := listener.Status.AuthorizationMigration.RetirementCheckpoint
				Expect(cp).ToNot(BeNil())
				Expect(cp.PendingDeletions).To(HaveLen(1))
				Expect(cp.PendingDeletions[0].Kind).To(Equal("ApprovalRequest"))
				Expect(cp.PendingDeletions[0].Phase).To(Equal(handler.ExportPendingDeletionPhasePrepared))
				Expect(cp.PendingDeletions[0].UID).To(Equal(string(request.UID)))
				Expect(cp.RequestsRetired).To(BeFalse())
			})

			It("should delete ApprovalRequest after PendingDeletion is persisted", func() {
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringRequests",
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
					RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
						PendingDeletions: []spectrev1.PendingDeletion{
							{
								Kind:            "ApprovalRequest",
								Name:            request.Name,
								Namespace:       request.Namespace,
								UID:             string(request.UID),
								ResourceVersion: request.ResourceVersion,
								Phase:           handler.ExportPendingDeletionPhasePrepared,
							},
						},
					},
				}

				// Discovery.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// retireLegacyRequests: fresh read + delete (with UID precondition).
				mockLegacyRequestExists(request)
				mockDeleteApprovalRequest(request.Name)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse()) // returned to verify delete on next reconcile
				cp := listener.Status.AuthorizationMigration.RetirementCheckpoint
				// Delete was issued but not yet verified — requests not yet retired.
				Expect(cp.RequestsRetired).To(BeFalse())
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("RetiringRequests"))
			})

			It("should delete Approval after PendingDeletion is persisted", func() {
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringApproval",
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
					RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
						RequestsRetired: true,
						PendingDeletions: []spectrev1.PendingDeletion{
							{
								Kind:  "ApprovalRequest",
								Name:  request.Name,
								Phase: handler.ExportPendingDeletionPhaseObserved,
							},
							{
								Kind:            "Approval",
								Name:            approval.Name,
								Namespace:       approval.Namespace,
								UID:             string(approval.UID),
								ResourceVersion: approval.ResourceVersion,
								Phase:           handler.ExportPendingDeletionPhasePrepared,
							},
						},
					},
				}

				// Discovery.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// retireLegacyApproval: fresh read + delete (with UID precondition).
				mockLegacyApprovalExists(approval)
				mockDeleteApproval(approval.Name)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse()) // returned to verify delete on next reconcile
			})

			It("should complete migration after all PendingDeletions are observed", func() {
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringApproval",
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
					RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
						RequestsRetired: true,
						PendingDeletions: []spectrev1.PendingDeletion{
							{
								Kind:  "ApprovalRequest",
								Name:  request.Name,
								Phase: handler.ExportPendingDeletionPhaseObserved,
							},
							{
								Kind:      "Approval",
								Name:      approval.Name,
								Namespace: approval.Namespace,
								UID:       string(approval.UID),
								Phase:     handler.ExportPendingDeletionPhaseObserved,
							},
						},
					},
				}

				// Discovery.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeTrue())
				Expect(listener.Status.AuthorizationPolicyVersion).To(Equal("v2"))
				Expect(listener.Status.AuthorizationMigration).To(BeNil())
			})
		})

		Context("with rejected legacy Approval", func() {
			It("should block migration", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateRejected)
				request := makeLegacyRequest(listener)

				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)

				done, err := h.AdvanceMigration(ctx, listener, &intent, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.AuthorizationMigration).ToNot(BeNil())
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("Blocked"))
			})
		})

		Context("with suspended legacy Approval", func() {
			It("should block migration", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateSuspended)
				request := makeLegacyRequest(listener)

				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)

				done, err := h.AdvanceMigration(ctx, listener, &intent, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("Blocked"))
			})
		})

		Context("with expired legacy Approval", func() {
			It("should block when expired from Suspended", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateExpired)
				approval.Status.LastState = approvalv1.ApprovalStateSuspended
				request := makeLegacyRequest(listener)

				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)

				done, err := h.AdvanceMigration(ctx, listener, &intent, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("Blocked"))
			})

			It("should proceed when expired from Granted", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateExpired)
				approval.Status.LastState = approvalv1.ApprovalStateGranted
				request := makeLegacyRequest(listener)

				// Discovery phase.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)

				done, err := h.AdvanceMigration(ctx, listener, &intent, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				// Should advance to AwaitingScoped (not Blocked).
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("AwaitingScoped"))
			})
		})

		Context("with missing/recreated evidence", func() {
			It("should block when recorded evidence disappears", func() {
				listener := newMigrationListener()
				// Pre-set migration status as if evidence was previously recorded.
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "AwaitingScoped",
					LegacyApproval: &ctypes.ObjectRef{
						Name:      legacyApprovalName(migListenerName),
						Namespace: migNamespace,
						UID:       "legacy-approval-uid-001",
					},
				}

				// Legacy approval is gone now.
				mockLegacyApprovalNotFound()

				done, err := h.AdvanceMigration(ctx, listener, &intent, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("Blocked"))
			})

			It("should block when legacy Approval UID changes (deleted and recreated)", func() {
				listener := newMigrationListener()
				// Pre-set migration status with the originally recorded UID.
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "AwaitingScoped",
					LegacyApproval: &ctypes.ObjectRef{
						Name:      legacyApprovalName(migListenerName),
						Namespace: migNamespace,
						UID:       "old-uid",
					},
				}

				// Return a same-named Approval but with a different UID.
				recreatedApproval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				recreatedApproval.UID = "new-uid"
				mockLegacyApprovalExists(recreatedApproval)
				mockLegacyRequestExists(makeLegacyRequest(listener))

				done, err := h.AdvanceMigration(ctx, listener, &intent, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("Blocked"))
			})
		})

		Context("restart resilience", func() {
			It("should resume from persisted phase after restart", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request := makeLegacyRequest(listener)

				// Simulate: migration was at AwaitingScoped, controller restarted.
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "AwaitingScoped",
					LegacyApproval: &ctypes.ObjectRef{
						Name:      legacyApprovalName(migListenerName),
						Namespace: migNamespace,
						UID:       "legacy-approval-uid-001",
					},
					LegacyRequests: []ctypes.ObjectRef{
						{
							Name:      request.Name,
							Namespace: request.Namespace,
							UID:       request.UID,
						},
					},
				}

				listener.Status.RouteListener = oldCaptureRef()

				// Discovery.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// Draining: the drain snapshot.
				mockStartDrainLists()

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				// Should be in Draining now with drain started.
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("Draining"))
				Expect(listener.Status.Draining).ToNot(BeNil())
			})

			It("should not re-delete requests when checkpoint says they are retired", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request := makeLegacyRequest(listener)

				// Simulate: requests already retired, approval retirement pending.
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringApproval",
					LegacyApproval: &ctypes.ObjectRef{
						Name:      legacyApprovalName(migListenerName),
						Namespace: migNamespace,
						UID:       "legacy-approval-uid-001",
					},
					RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
						RequestsRetired: true,
					},
				}

				// Discovery: Approval Get + ApprovalRequest Get (from ApprovedRequest ref).
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// retireLegacyApproval: fresh read -> records PendingDeletion, returns false.
				mockLegacyApprovalExists(approval)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				// PendingDeletion recorded for approval.
				cp := listener.Status.AuthorizationMigration.RetirementCheckpoint
				Expect(cp.PendingDeletions).To(HaveLen(1))
				Expect(cp.PendingDeletions[0].Kind).To(Equal("Approval"))
				Expect(cp.PendingDeletions[0].Phase).To(Equal(handler.ExportPendingDeletionPhasePrepared))
			})
		})

		Context("phase-aware recovery", func() {
			It("should recover when ApprovalRequest gone after DeletePrepared", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request := makeLegacyRequest(listener)

				// Simulate: crash after delete before ack — PendingDeletion recorded
				// but the controller never persisted the DeleteObserved phase.
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringRequests",
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
					RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
						PendingDeletions: []spectrev1.PendingDeletion{
							{
								Kind:      "ApprovalRequest",
								Name:      request.Name,
								Namespace: request.Namespace,
								UID:       string(request.UID),
								Phase:     handler.ExportPendingDeletionPhasePrepared,
							},
						},
					},
				}

				// Discovery.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// retireLegacyRequests: fresh read returns NotFound.
				mockLegacyRequestNotFound(request.Name, request.Namespace)
				// retireLegacyApproval: fresh read -> records PendingDeletion.
				mockLegacyApprovalExists(approval)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse()) // approval PendingDeletion just recorded
				cp := listener.Status.AuthorizationMigration.RetirementCheckpoint
				Expect(cp.RequestsRetired).To(BeTrue())
				// The PendingDeletion for request should be marked DeleteObserved.
				Expect(cp.PendingDeletions[0].Phase).To(Equal(handler.ExportPendingDeletionPhaseObserved))
			})

			It("should recover when Approval gone after DeletePrepared", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request := makeLegacyRequest(listener)

				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringApproval",
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
					RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
						RequestsRetired: true,
						PendingDeletions: []spectrev1.PendingDeletion{
							{
								Kind:  "ApprovalRequest",
								Name:  request.Name,
								Phase: handler.ExportPendingDeletionPhaseObserved,
							},
							{
								Kind:      "Approval",
								Name:      approval.Name,
								Namespace: approval.Namespace,
								UID:       string(approval.UID),
								Phase:     handler.ExportPendingDeletionPhasePrepared,
							},
						},
					},
				}

				// Discovery: approval was deleted and is now gone.
				mockLegacyApprovalNotFound()
				// retireLegacyApproval: fresh read also returns NotFound.
				mockLegacyApprovalNotFound()

				// The pending deletion checkpoint tells the evidence guard that
				// the absence is expected — retirement observes the NotFound and
				// completes the migration.
				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeTrue())
				Expect(listener.Status.AuthorizationPolicyVersion).To(Equal("v2"))
				Expect(listener.Status.AuthorizationMigration).To(BeNil())
			})
		})

		Context("unexplained absence", func() {
			It("should block when ApprovalRequest missing without deletion checkpoint", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request := makeLegacyRequest(listener)

				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringRequests",
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
				}

				// Discovery.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// retireLegacyRequests: fresh read returns NotFound — no checkpoint.
				mockLegacyRequestNotFound(request.Name, request.Namespace)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexplained absence"))
				Expect(done).To(BeFalse())
			})
		})

		Context("Bug 1 regression: pending deletion bypasses evidence guard", func() {
			It("should not block when Approval absent but PendingDeletion exists", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request := makeLegacyRequest(listener)

				// State: RetiringApproval, Approval was successfully deleted (NotFound
				// on discovery), but a PendingDeletion with phase=DeletePrepared exists.
				// Bug 1 caused the evidence-missing guard to block here.
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringApproval",
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
					RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
						RequestsRetired: true,
						PendingDeletions: []spectrev1.PendingDeletion{
							{
								Kind:  "ApprovalRequest",
								Name:  request.Name,
								Phase: handler.ExportPendingDeletionPhaseObserved,
							},
							{
								Kind:      "Approval",
								Name:      approval.Name,
								Namespace: approval.Namespace,
								UID:       string(approval.UID),
								Phase:     handler.ExportPendingDeletionPhasePrepared,
							},
						},
					},
				}

				// Discovery: Approval is gone (NotFound).
				mockLegacyApprovalNotFound()
				// retireLegacyApproval: fresh read also returns NotFound.
				mockLegacyApprovalNotFound()

				// With the fix: evidence guard sees the pending deletion and skips
				// blocking. retireLegacyApproval observes the NotFound and completes.
				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeTrue())
				Expect(listener.Status.AuthorizationPolicyVersion).To(Equal("v2"))
				Expect(listener.Status.AuthorizationMigration).To(BeNil())
			})

			It("should still block when Approval absent without PendingDeletion", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)

				// No retirement checkpoint at all — truly unexplained absence.
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "AwaitingScoped",
					LegacyApproval: &ctypes.ObjectRef{
						Name:      legacyApprovalName(migListenerName),
						Namespace: migNamespace,
						UID:       k8stypes.UID(approval.UID),
					},
				}

				mockLegacyApprovalNotFound()

				done, err := h.AdvanceMigration(ctx, listener, &intent, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("Blocked"))
			})
		})

		Context("Bug 2 regression: RV preconditions and restrictive state re-check", func() {
			It("should block at retirement when Approval becomes Rejected after checkpoint", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request := makeLegacyRequest(listener)

				// Simulate: at RetiringApproval, PendingDeletion recorded when state
				// was Granted. Between reconciles, the Approval changed to Rejected.
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringApproval",
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
					RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
						RequestsRetired: true,
						PendingDeletions: []spectrev1.PendingDeletion{
							{
								Kind:            "Approval",
								Name:            approval.Name,
								Namespace:       approval.Namespace,
								UID:             string(approval.UID),
								ResourceVersion: "100",
								Phase:           handler.ExportPendingDeletionPhasePrepared,
							},
						},
					},
				}

				// Discovery: Approval is now Rejected (state changed between reconciles).
				// The step 2 isLegacyBlocked check catches this at the discovery level.
				rejectedApproval := makeLegacyApproval(listener, approvalv1.ApprovalStateRejected)
				rejectedApproval.ResourceVersion = "200"
				mockLegacyApprovalExists(rejectedApproval)
				mockLegacyRequestExists(request)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("Blocked"))
			})

			It("should also block via retirement re-check when Approval becomes Suspended after discovery", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request := makeLegacyRequest(listener)

				// Simulate: at RetiringApproval with no PendingDeletion yet.
				// Discovery sees Granted, but the retirement fresh read sees Suspended
				// (changed between the two Gets in the same reconcile).
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringApproval",
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
					RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
						RequestsRetired: true,
					},
				}

				// Discovery: Approval still Granted.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)

				// retireLegacyApproval fresh read: now Suspended (changed mid-reconcile).
				suspendedApproval := makeLegacyApproval(listener, approvalv1.ApprovalStateSuspended)
				suspendedApproval.ResourceVersion = "200"
				mockLegacyApprovalExists(suspendedApproval)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				// The retirement re-check catches the restrictive state.
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("Blocked"))
			})

			It("should re-validate when Approval ResourceVersion changes during retirement", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request := makeLegacyRequest(listener)

				// Simulate: at RetiringApproval, PendingDeletion recorded with old RV.
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringApproval",
					LegacyApproval:      ctypes.ObjectRefFromObject(approval),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
					RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
						RequestsRetired: true,
						PendingDeletions: []spectrev1.PendingDeletion{
							{
								Kind:            "Approval",
								Name:            approval.Name,
								Namespace:       approval.Namespace,
								UID:             string(approval.UID),
								ResourceVersion: "100", // original RV
								Phase:           handler.ExportPendingDeletionPhasePrepared,
							},
						},
					},
				}

				// Discovery: Approval still Granted but RV changed (e.g. metadata update).
				modifiedApproval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				modifiedApproval.ResourceVersion = "200" // different from checkpoint
				mockLegacyApprovalExists(modifiedApproval)
				mockLegacyRequestExists(request)
				// retireLegacyApproval: fresh read returns modified approval.
				mockLegacyApprovalExists(modifiedApproval)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				// Should NOT have deleted — should have updated checkpoint with new RV.
				cp := listener.Status.AuthorizationMigration.RetirementCheckpoint
				Expect(cp.PendingDeletions[0].ResourceVersion).To(Equal("200"))
			})
		})

		Context("retirement conflict", func() {
			It("should block when Approval UID changes between recorded and discovered", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request := makeLegacyRequest(listener)

				// Simulate: at RetiringApproval phase with a recorded UID that
				// differs from the live object — the approval was deleted and
				// recreated with the same name.
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringApproval",
					LegacyApproval: &ctypes.ObjectRef{
						Name:      legacyApprovalName(migListenerName),
						Namespace: migNamespace,
						UID:       "recorded-uid-different", // Different from actual.
					},
					RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
						RequestsRetired: true,
					},
				}

				// Discovery: Approval Get + ApprovalRequest Get (from ApprovedRequest ref).
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)

				// The UID cross-check fires before retirement, blocking gracefully.
				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("Blocked"))
			})

			It("should abort when ApprovalRequest UID changes during retirement", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request := makeLegacyRequest(listener)

				// Simulate: at RetiringRequests phase with recorded UID that differs.
				listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "RetiringRequests",
					LegacyApproval: &ctypes.ObjectRef{
						Name:      legacyApprovalName(migListenerName),
						Namespace: migNamespace,
						UID:       "legacy-approval-uid-001",
					},
					LegacyRequests: []ctypes.ObjectRef{
						{
							Name:      request.Name,
							Namespace: request.Namespace,
							UID:       "recorded-request-uid-different",
						},
					},
				}

				// Discovery.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// Retirement: fresh read returns request with actual UID (mismatch).
				mockLegacyRequestExists(request)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("UID mismatch"))
				Expect(done).To(BeFalse())
			})
		})
	})

	Describe("hasPendingDeletionFor", func() {
		It("should return true when matching PendingDeletion with DeletePrepared exists", func() {
			migration := &spectrev1.AuthorizationMigrationStatus{
				RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
					PendingDeletions: []spectrev1.PendingDeletion{
						{
							Kind:      "Approval",
							Name:      "test-approval",
							Namespace: migNamespace,
							UID:       "uid-123",
							Phase:     handler.ExportPendingDeletionPhasePrepared,
						},
					},
				},
			}
			ref := &ctypes.ObjectRef{
				Name:      "test-approval",
				Namespace: migNamespace,
				UID:       "uid-123",
			}
			Expect(handler.HasPendingDeletionFor(migration, "Approval", ref)).To(BeTrue())
		})

		It("should return false when PendingDeletion is already DeleteObserved", func() {
			migration := &spectrev1.AuthorizationMigrationStatus{
				RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
					PendingDeletions: []spectrev1.PendingDeletion{
						{
							Kind:      "Approval",
							Name:      "test-approval",
							Namespace: migNamespace,
							UID:       "uid-123",
							Phase:     handler.ExportPendingDeletionPhaseObserved,
						},
					},
				},
			}
			ref := &ctypes.ObjectRef{
				Name:      "test-approval",
				Namespace: migNamespace,
				UID:       "uid-123",
			}
			Expect(handler.HasPendingDeletionFor(migration, "Approval", ref)).To(BeFalse())
		})

		It("should return false when UID does not match", func() {
			migration := &spectrev1.AuthorizationMigrationStatus{
				RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
					PendingDeletions: []spectrev1.PendingDeletion{
						{
							Kind:      "Approval",
							Name:      "test-approval",
							Namespace: migNamespace,
							UID:       "uid-different",
							Phase:     handler.ExportPendingDeletionPhasePrepared,
						},
					},
				},
			}
			ref := &ctypes.ObjectRef{
				Name:      "test-approval",
				Namespace: migNamespace,
				UID:       "uid-123",
			}
			Expect(handler.HasPendingDeletionFor(migration, "Approval", ref)).To(BeFalse())
		})

		It("should return false when no RetirementCheckpoint exists", func() {
			migration := &spectrev1.AuthorizationMigrationStatus{}
			ref := &ctypes.ObjectRef{
				Name:      "test-approval",
				Namespace: migNamespace,
				UID:       "uid-123",
			}
			Expect(handler.HasPendingDeletionFor(migration, "Approval", ref)).To(BeFalse())
		})

		It("should return false for nil ref", func() {
			migration := &spectrev1.AuthorizationMigrationStatus{
				RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{},
			}
			Expect(handler.HasPendingDeletionFor(migration, "Approval", nil)).To(BeFalse())
		})
	})

	Describe("Finding 1 regression: multi-reconcile fresh-install escape", func() {
		It("should persist migration record so second reconcile does not misclassify as fresh", func() {
			listener := newMigrationListener()
			intent := handler.NewTestIntent()

			// Simulate the pre-condition: old unscoped ProviderApproval ref exists
			// (no "ag-v1-" prefix), causing isFreshInstall to return false.
			listener.Status.ProviderApproval = &ctypes.ObjectRef{
				Name:      legacyApprovalName(migListenerName),
				Namespace: migNamespace,
			}

			// --- Reconcile 1 ---
			// advanceMigration finds no legacy Approval by name. The bug was that
			// it set conditions but did NOT persist an AuthorizationMigration
			// record. With the fix, it persists Phase=Blocked.
			mockLegacyApprovalNotFound()

			done, err := h.AdvanceMigration(ctx, listener, &intent, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse())
			// Fix: migration record MUST be persisted.
			Expect(listener.Status.AuthorizationMigration).ToNot(BeNil(),
				"Reconcile 1 must persist a migration record to prevent fresh-install escape")
			Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("Blocked"))

			// --- Simulate ensureApprovals overwriting ProviderApproval with scoped ref ---
			listener.Status.ProviderApproval = &ctypes.ObjectRef{
				Name:      "ag-v1-provider-" + migListenerName,
				Namespace: migNamespace,
			}

			// --- Reconcile 2 ---
			// isFreshInstall would see only the scoped ref (ag-v1- prefix) and no
			// old unlabelled children. Without the fix, AuthorizationMigration is
			// nil, so isFreshInstall returns true. With the fix, isFreshInstall
			// sees AuthorizationMigration != nil and returns false.
			fresh, err := h.IsFreshInstall(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(fresh).To(BeFalse(),
				"Reconcile 2: isFreshInstall must return false because migration record exists")
			Expect(listener.Status.AuthorizationPolicyVersion).ToNot(Equal("v2"),
				"Listener must NOT escape to v2")
		})
	})

	Describe("Finding 2 regression: retirement re-authorization check", func() {
		It("should not delete ApprovalRequest from DeletePrepared when dual regresses to Pending", func() {
			listener := newMigrationListener()
			intent := handler.NewTestIntent()
			approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
			request := makeLegacyRequest(listener)

			// State: RetiringRequests, PendingDeletion was recorded while Granted.
			listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
				TargetPolicyVersion: "v2",
				Phase:               "RetiringRequests",
				LegacyApproval:      ctypes.ObjectRefFromObject(approval),
				LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
				RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
					PendingDeletions: []spectrev1.PendingDeletion{
						{
							Kind:            "ApprovalRequest",
							Name:            request.Name,
							Namespace:       request.Namespace,
							UID:             string(request.UID),
							ResourceVersion: request.ResourceVersion,
							Phase:           handler.ExportPendingDeletionPhasePrepared,
						},
					},
				},
			}

			// Discovery.
			mockLegacyApprovalExists(approval)
			mockLegacyRequestExists(request)
			// retireLegacyRequests: fresh read succeeds (object still alive).
			mockLegacyRequestExists(request)

			// Dual has regressed to Pending (was Granted when checkpoint was recorded).
			done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualPending())
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse())
			// ZERO delete calls should have been made — mockery strict mode
			// would fail if any Delete was called unexpectedly.
			// Phase should still be RetiringRequests (waiting for re-grant).
			Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("RetiringRequests"))
		})

		It("should not delete Approval from DeletePrepared when dual regresses to Pending", func() {
			listener := newMigrationListener()
			intent := handler.NewTestIntent()
			approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
			request := makeLegacyRequest(listener)

			// State: RetiringApproval, PendingDeletion was recorded while Granted.
			listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
				TargetPolicyVersion: "v2",
				Phase:               "RetiringApproval",
				LegacyApproval:      ctypes.ObjectRefFromObject(approval),
				LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
				RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
					RequestsRetired: true,
					PendingDeletions: []spectrev1.PendingDeletion{
						{
							Kind:  "ApprovalRequest",
							Name:  request.Name,
							Phase: handler.ExportPendingDeletionPhaseObserved,
						},
						{
							Kind:            "Approval",
							Name:            approval.Name,
							Namespace:       approval.Namespace,
							UID:             string(approval.UID),
							ResourceVersion: approval.ResourceVersion,
							Phase:           handler.ExportPendingDeletionPhasePrepared,
						},
					},
				},
			}

			// Discovery.
			mockLegacyApprovalExists(approval)
			mockLegacyRequestExists(request)
			// retireLegacyApproval: fresh read succeeds.
			mockLegacyApprovalExists(approval)

			// Dual has regressed to Pending.
			done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualPending())
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse())
			// ZERO delete calls — mockery strict mode catches any unexpected Delete.
			Expect(listener.Status.AuthorizationMigration.Phase).To(Equal("RetiringApproval"))
		})

		It("should resume retirement when dual re-grants after regression", func() {
			listener := newMigrationListener()
			intent := handler.NewTestIntent()
			approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
			request := makeLegacyRequest(listener)

			// State: same as above but now dual is Granted again.
			listener.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
				TargetPolicyVersion: "v2",
				Phase:               "RetiringRequests",
				LegacyApproval:      ctypes.ObjectRefFromObject(approval),
				LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(request)},
				RetirementCheckpoint: &spectrev1.MigrationRetirementCheckpoint{
					PendingDeletions: []spectrev1.PendingDeletion{
						{
							Kind:            "ApprovalRequest",
							Name:            request.Name,
							Namespace:       request.Namespace,
							UID:             string(request.UID),
							ResourceVersion: request.ResourceVersion,
							Phase:           handler.ExportPendingDeletionPhasePrepared,
						},
					},
				},
			}

			// Discovery.
			mockLegacyApprovalExists(approval)
			mockLegacyRequestExists(request)
			// retireLegacyRequests: fresh read + delete.
			mockLegacyRequestExists(request)
			mockDeleteApprovalRequest(request.Name)

			// Re-granted — delete should proceed.
			done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse()) // returned to verify delete on next reconcile
		})
	})

	Describe("isLegacyBlocked", func() {
		It("should block on Rejected", func() {
			approval := makeLegacyApproval(newMigrationListener(), approvalv1.ApprovalStateRejected)
			blocked, reason := handler.IsLegacyBlocked(approval)
			Expect(blocked).To(BeTrue())
			Expect(reason).To(ContainSubstring("Rejected"))
		})

		It("should block on Suspended", func() {
			approval := makeLegacyApproval(newMigrationListener(), approvalv1.ApprovalStateSuspended)
			blocked, reason := handler.IsLegacyBlocked(approval)
			Expect(blocked).To(BeTrue())
			Expect(reason).To(ContainSubstring("Suspended"))
		})

		It("should block on Pending", func() {
			approval := makeLegacyApproval(newMigrationListener(), approvalv1.ApprovalStatePending)
			blocked, reason := handler.IsLegacyBlocked(approval)
			Expect(blocked).To(BeTrue())
			Expect(reason).To(ContainSubstring("Pending"))
		})

		It("should not block on Granted", func() {
			approval := makeLegacyApproval(newMigrationListener(), approvalv1.ApprovalStateGranted)
			blocked, _ := handler.IsLegacyBlocked(approval)
			Expect(blocked).To(BeFalse())
		})

		It("should block on Expired from Suspended", func() {
			approval := makeLegacyApproval(newMigrationListener(), approvalv1.ApprovalStateExpired)
			approval.Status.LastState = approvalv1.ApprovalStateSuspended
			blocked, reason := handler.IsLegacyBlocked(approval)
			Expect(blocked).To(BeTrue())
			Expect(reason).To(ContainSubstring("expired from Suspended"))
		})

		It("should not block on Expired from Granted", func() {
			approval := makeLegacyApproval(newMigrationListener(), approvalv1.ApprovalStateExpired)
			approval.Status.LastState = approvalv1.ApprovalStateGranted
			blocked, _ := handler.IsLegacyBlocked(approval)
			Expect(blocked).To(BeFalse())
		})
	})
})
