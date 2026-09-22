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
			Return(nil)
	}

	mockLegacyRequestExists := func(ar *approvalv1.ApprovalRequest) {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: ar.Name, Namespace: ar.Namespace},
				mock.AnythingOfType("*v1.ApprovalRequest")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*approvalv1.ApprovalRequest) = *ar
			}).
			Return(nil)
	}

	mockDeleteApprovalRequest := func(name string) {
		fakeClient.EXPECT().
			Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
				return obj.GetName() == name
			})).
			Return(nil).Once()
	}

	mockDeleteApproval := func(name string) {
		fakeClient.EXPECT().
			Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
				return obj.GetName() == name
			})).
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

		It("should return true when no legacy Approval exists", func() {
			listener := newMigrationListener()
			mockLegacyApprovalNotFound()

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

		It("should enter v2 directly when no legacy evidence exists", func() {
			mockLegacyApprovalNotFound()

			done, err := h.AdvanceMigration(ctx, listener, &intent, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeTrue())
			Expect(listener.Status.AuthorizationPolicyVersion).To(Equal("v2"))
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

			It("should advance past AwaitingScoped when dual-gate is granted", func() {
				// Discovery: Approval + ApprovalRequest.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// Retirement (full path through retirement).
				mockLegacyRequestExists(request)
				mockDeleteApprovalRequest(request.Name)
				mockLegacyApprovalExists(approval)
				mockDeleteApproval(approval.Name)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeTrue())
				Expect(listener.Status.AuthorizationPolicyVersion).To(Equal("v2"))
			})

			It("should complete migration through all phases with proper mocks", func() {
				// Full path: discover -> record -> awaitingScoped -> draining -> retire requests -> retire approval -> v2.
				// The approval Get is called twice: once in discoverLegacyEvidence, once in retireLegacyApproval.
				// The request Get is called twice: once in discoverLegacyEvidence, once in retireLegacyRequests.

				// discoverLegacyEvidence: Get legacy Approval.
				mockLegacyApprovalExists(approval)
				// discoverLegacyEvidence: Get legacy ApprovalRequest.
				mockLegacyRequestExists(request)
				// retireLegacyRequests: Get legacy ApprovalRequest (fresh read).
				mockLegacyRequestExists(request)
				// retireLegacyRequests: Delete.
				mockDeleteApprovalRequest(request.Name)
				// retireLegacyApproval: Get legacy Approval (fresh read).
				mockLegacyApprovalExists(approval)
				// retireLegacyApproval: Delete.
				mockDeleteApproval(approval.Name)

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

				// Discovery.
				mockLegacyApprovalExists(approval)
				mockLegacyRequestExists(request)
				// Retirement (triggered by dual granted).
				mockLegacyRequestExists(request)
				mockDeleteApprovalRequest(request.Name)
				mockLegacyApprovalExists(approval)
				mockDeleteApproval(approval.Name)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeTrue())
				Expect(listener.Status.AuthorizationPolicyVersion).To(Equal("v2"))
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
				// Retirement of approval (fresh read + delete).
				mockLegacyApprovalExists(approval)
				mockDeleteApproval(approval.Name)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeTrue())
			})
		})

		Context("retirement conflict", func() {
			It("should abort when Approval UID changes during retirement", func() {
				listener := newMigrationListener()
				approval := makeLegacyApproval(listener, approvalv1.ApprovalStateGranted)
				request := makeLegacyRequest(listener)

				// Simulate: at RetiringApproval phase.
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
				// Retirement: fresh read returns the same approval (UID mismatch with recorded).
				mockLegacyApprovalExists(approval)

				done, err := h.AdvanceMigration(ctx, listener, &intent, makeDualGranted())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("UID mismatch"))
				Expect(done).To(BeFalse())
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
