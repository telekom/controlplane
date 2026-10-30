// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	arhandler "github.com/telekom/controlplane/approval/internal/handler/approvalrequest"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The ApprovalRequest handler writes a lifted revocation through the API
// server, so the Approval webhook validates the write.
var _ = Describe("Lifting a scoped revocation through the API server", func() {
	const (
		env = "test"
		key = "provider"
	)
	now := time.Now().Truncate(time.Second)
	before := metav1.NewTime(now.Add(-3 * time.Hour))
	revokedAt := metav1.NewTime(now.Add(-2 * time.Hour))
	after := metav1.NewTime(now.Add(-time.Hour))
	labels := map[string]string{config.EnvironmentLabelKey: env}
	decision := func(name string, state approvalv1.ApprovalState, at *metav1.Time) approvalv1.Decision {
		return approvalv1.Decision{Name: name, Email: name + "@example.com", ResultingState: state, Timestamp: at}
	}

	var target ctypes.TypedObjectRef
	BeforeEach(func() {
		target = ctypes.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{APIVersion: "testgroup.cp.ei.telekom.de/v1", Kind: "TestResource"},
			ObjectRef: ctypes.ObjectRef{
				Name: "listener-" + rand.String(6), Namespace: "default", UID: ktypes.UID(rand.String(12)),
			},
		}
	})

	// revokedApproval creates the gate's Approval granted for an older request
	// and moves it to the given revoked states with decisions.
	revokedApproval := func(strategy approvalv1.ApprovalStrategy, revocations ...approvalv1.Decision) *approvalv1.Approval {
		name, err := approvalv1.ScopedApprovalName(target, key)
		Expect(err).NotTo(HaveOccurred())
		approval := &approvalv1.Approval{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: "default", Labels: labels,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: target.APIVersion, Kind: target.Kind, Name: target.Name, UID: target.UID,
					Controller: ptr.To(true), BlockOwnerDeletion: ptr.To(true),
				}},
			},
			Spec: approvalv1.ApprovalSpec{
				Target:          target,
				ApprovalKey:     key,
				Action:          "listen-provider",
				Strategy:        strategy,
				State:           approvalv1.ApprovalStateGranted,
				Requester:       approvalv1.Requester{TeamName: "observer", TeamEmail: "observer@example.com", Reason: "listen"},
				ApprovedRequest: &ctypes.ObjectRef{Name: "old-request", Namespace: "default", UID: "old-uid"},
				Decisions:       []approvalv1.Decision{decision("alice", approvalv1.ApprovalStateGranted, &before)},
			},
		}
		Expect(k8sClient.Create(ctx, approval)).To(Succeed())
		for _, d := range revocations {
			approval.Spec.State = d.ResultingState
			approval.Spec.Decisions = append(approval.Spec.Decisions, d)
			Expect(k8sClient.Update(ctx, approval)).To(Succeed())
		}
		return approval
	}

	// grantedRequest creates the newer request for the gate, granted with decisions.
	grantedRequest := func(strategy approvalv1.ApprovalStrategy, decisions ...approvalv1.Decision) *approvalv1.ApprovalRequest {
		ar := &approvalv1.ApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "request-" + rand.String(6), Namespace: "default", Labels: labels},
			Spec: approvalv1.ApprovalRequestSpec{
				Target:      target,
				ApprovalKey: key,
				Action:      "listen-provider",
				Strategy:    strategy,
				State:       approvalv1.ApprovalStateGranted,
				Requester:   approvalv1.Requester{TeamName: "observer", TeamEmail: "observer@example.com", Reason: "listen"},
				Decisions:   decisions,
			},
		}
		Expect(k8sClient.Create(ctx, ar)).To(Succeed())
		// The state change has been notified already.
		ar.Status.LastState = ar.Spec.State
		return ar
	}

	handle := func(ar *approvalv1.ApprovalRequest) error {
		hctx := contextutil.WithRecorder(contextutil.WithEnv(ctx, env), record.NewFakeRecorder(10))
		hctx = cclient.WithClient(hctx, cclient.NewJanitorClient(cclient.NewScopedClient(k8sClient, env)))
		return (&arhandler.ApprovalRequestHandler{Reader: k8sClient}).CreateOrUpdate(hctx, ar)
	}
	current := func(approval *approvalv1.Approval) *approvalv1.Approval {
		live := &approvalv1.Approval{}
		Expect(k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(approval), live)).To(Succeed())
		return live
	}

	It("grants a Rejected FourEyes Approval for a newer request both approvals of which follow the revocation", func() {
		approval := revokedApproval(approvalv1.ApprovalStrategyFourEyes, decision("alice", approvalv1.ApprovalStateRejected, &revokedAt))
		ar := grantedRequest(approvalv1.ApprovalStrategyFourEyes,
			decision("bob", approvalv1.ApprovalStateSemigranted, &after),
			decision("carol", approvalv1.ApprovalStateGranted, &after))

		Expect(handle(ar)).To(Succeed())

		live := current(approval)
		Expect(live.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
		Expect(live.Spec.ApprovedRequest.UID).To(Equal(ar.UID))
	})

	It("keeps a Rejected FourEyes Approval when the newer request's first approval predates the revocation", func() {
		approval := revokedApproval(approvalv1.ApprovalStrategyFourEyes, decision("alice", approvalv1.ApprovalStateRejected, &revokedAt))
		ar := grantedRequest(approvalv1.ApprovalStrategyFourEyes,
			decision("bob", approvalv1.ApprovalStateSemigranted, &before),
			decision("carol", approvalv1.ApprovalStateGranted, &after))

		Expect(handle(ar)).To(Succeed())

		Expect(current(approval).Spec.State).To(Equal(approvalv1.ApprovalStateRejected))
	})

	// boundApproval grants the gate's Approval for a newer FourEyes request.
	boundApproval := func() (*approvalv1.Approval, *approvalv1.ApprovalRequest) {
		approval := revokedApproval(approvalv1.ApprovalStrategyFourEyes, decision("alice", approvalv1.ApprovalStateRejected, &revokedAt))
		ar := grantedRequest(approvalv1.ApprovalStrategyFourEyes,
			decision("bob", approvalv1.ApprovalStateSemigranted, &after),
			decision("carol", approvalv1.ApprovalStateGranted, &after))
		Expect(handle(ar)).To(Succeed())
		approval = current(approval)
		Expect(approval.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
		return approval, ar
	}

	Context("a revoked Approval whose request reference is edited", func() {
		// revokedAgain rejects the Approval after it was granted for ar.
		revokedAgain := func() (*approvalv1.Approval, *approvalv1.ApprovalRequest) {
			approval, ar := boundApproval()
			approval.Spec.State = approvalv1.ApprovalStateRejected
			approval.Spec.Decisions = append(approval.Spec.Decisions,
				decision("alice", approvalv1.ApprovalStateRejected, ptr.To(metav1.NewTime(now))))
			Expect(k8sClient.Update(ctx, approval)).To(Succeed())
			return approval, ar
		}

		It("stays revoked when the reference is cleared", func() {
			approval, ar := revokedAgain()

			approval.Spec.ApprovedRequest = nil
			Expect(k8sClient.Update(ctx, approval)).To(MatchError(ContainSubstring("must not be removed")))

			Expect(handle(ar)).To(Succeed())
			Expect(current(approval).Spec.State).To(Equal(approvalv1.ApprovalStateRejected))
		})

		It("stays revoked when the reference is cleared and set again with a grant", func() {
			approval, ar := revokedAgain()
			ref := approval.Spec.ApprovedRequest

			approval.Spec.ApprovedRequest = nil
			Expect(k8sClient.Update(ctx, approval)).To(MatchError(ContainSubstring("must not be removed")))

			approval = current(approval)
			approval.Spec.ApprovedRequest = ref
			approval.Spec.State = approvalv1.ApprovalStateGranted
			// Drop the oldest decision so the defaulter does not trim the list.
			approval.Spec.Decisions = append(approval.Spec.Decisions[1:],
				decision("mallory", approvalv1.ApprovalStateGranted, ptr.To(metav1.NewTime(now))))
			Expect(k8sClient.Update(ctx, approval)).To(MatchError(ContainSubstring("Invalid state transition")))

			live := current(approval)
			Expect(live.Spec.State).To(Equal(approvalv1.ApprovalStateRejected))
			Expect(live.Spec.ApprovedRequest.UID).To(Equal(ar.UID))
		})

		It("rejects another reference without a grant", func() {
			approval, _ := revokedAgain()
			newer := grantedRequest(approvalv1.ApprovalStrategyFourEyes,
				decision("bob", approvalv1.ApprovalStateSemigranted, ptr.To(metav1.NewTime(now))),
				decision("carol", approvalv1.ApprovalStateGranted, ptr.To(metav1.NewTime(now))))

			approval.Spec.ApprovedRequest = ctypes.ObjectRefFromObject(newer)
			Expect(k8sClient.Update(ctx, approval)).To(MatchError(ContainSubstring("must be Granted")))
		})
	})

	Context("a Granted Approval rebound without a state change", func() {
		It("is rejected for a request that is not granted", func() {
			approval, _ := boundApproval()
			newer := &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "request-" + rand.String(6), Namespace: "default", Labels: labels},
				Spec: approvalv1.ApprovalRequestSpec{
					Target:      target,
					ApprovalKey: key,
					Action:      "listen-provider",
					Strategy:    approvalv1.ApprovalStrategyFourEyes,
					State:       approvalv1.ApprovalStateSemigranted,
					Requester:   approvalv1.Requester{TeamName: "observer", TeamEmail: "observer@example.com", Reason: "listen"},
					Decisions:   []approvalv1.Decision{decision("bob", approvalv1.ApprovalStateSemigranted, &after)},
				},
			}
			Expect(k8sClient.Create(ctx, newer)).To(Succeed())

			approval.Spec.ApprovedRequest = ctypes.ObjectRefFromObject(newer)
			Expect(k8sClient.Update(ctx, approval)).To(MatchError(ContainSubstring("not granted")))
		})

		It("is allowed for a newer request granted by two people", func() {
			approval, _ := boundApproval()
			newer := grantedRequest(approvalv1.ApprovalStrategyFourEyes,
				decision("dave", approvalv1.ApprovalStateSemigranted, &after),
				decision("erin", approvalv1.ApprovalStateGranted, &after))

			approval.Spec.ApprovedRequest = ctypes.ObjectRefFromObject(newer)
			Expect(k8sClient.Update(ctx, approval)).To(Succeed())

			live := current(approval)
			Expect(live.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
			Expect(live.Spec.ApprovedRequest.UID).To(Equal(newer.UID))
		})
	})

	Context("an Approval half re-granted after its revocation", func() {
		halfRegranted := func() *approvalv1.Approval {
			return revokedApproval(approvalv1.ApprovalStrategyFourEyes,
				decision("alice", approvalv1.ApprovalStateRejected, &before),
				decision("bob", approvalv1.ApprovalStateSemigranted, &revokedAt))
		}

		It("is granted for a newer request a person granted afterwards", func() {
			approval := halfRegranted()
			ar := grantedRequest(approvalv1.ApprovalStrategySimple, decision("carol", approvalv1.ApprovalStateGranted, &after))

			Expect(handle(ar)).To(Succeed())

			live := current(approval)
			Expect(live.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
			Expect(live.Spec.ApprovedRequest.UID).To(Equal(ar.UID))
		})

		It("stays half re-granted for an automatic grant", func() {
			approval := halfRegranted()
			ar := grantedRequest(approvalv1.ApprovalStrategyAuto)

			Expect(handle(ar)).To(Succeed())

			Expect(current(approval).Spec.State).To(Equal(approvalv1.ApprovalStateSemigranted))
		})
	})
})
