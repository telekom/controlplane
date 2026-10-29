// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
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

// A newly granted ApprovalRequest overwrites the Approval of its target through
// the API server, so the Approval webhook validates the rebind.
var _ = Describe("Granting a new ApprovalRequest for a target that has an Approval", func() {
	const env = "test"
	labels := map[string]string{config.EnvironmentLabelKey: env}
	requester := approvalv1.Requester{
		TeamName: "subscriber", TeamEmail: "subscriber@example.com", Reason: "subscribe",
		Properties: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	decision := func(name string, state approvalv1.ApprovalState) approvalv1.Decision {
		return approvalv1.Decision{Name: name, Email: name + "@example.com", ResultingState: state}
	}
	fourEyesGrant := func() []approvalv1.Decision {
		return []approvalv1.Decision{
			decision("bob", approvalv1.ApprovalStateSemigranted),
			decision("carol", approvalv1.ApprovalStateGranted),
		}
	}

	var target ctypes.TypedObjectRef
	BeforeEach(func() {
		target = ctypes.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{APIVersion: "testgroup.cp.ei.telekom.de/v1", Kind: "TestResource"},
			ObjectRef: ctypes.ObjectRef{
				Name: "subscription-" + rand.String(6), Namespace: "default", UID: ktypes.UID(rand.String(12)),
			},
		}
	})

	// existingApproval creates the target's Approval granted for an older
	// request and moves it through the given decisions.
	existingApproval := func(strategy approvalv1.ApprovalStrategy, decisions ...approvalv1.Decision) *approvalv1.Approval {
		approval := &approvalv1.Approval{
			ObjectMeta: metav1.ObjectMeta{
				Name: approvalv1.ApprovalName(target.Kind, target.Name), Namespace: "default", Labels: labels,
			},
			Spec: approvalv1.ApprovalSpec{
				Target:          target,
				Action:          "subscribe",
				Strategy:        strategy,
				State:           approvalv1.ApprovalStateGranted,
				Requester:       requester,
				ApprovedRequest: &ctypes.ObjectRef{Name: "old-request", Namespace: "default", UID: "old-uid"},
				Decisions:       []approvalv1.Decision{decision("alice", approvalv1.ApprovalStateGranted)},
			},
		}
		Expect(k8sClient.Create(ctx, approval)).To(Succeed())
		for _, d := range decisions {
			approval.Spec.State = d.ResultingState
			approval.Spec.Decisions = append(approval.Spec.Decisions, d)
			Expect(k8sClient.Update(ctx, approval)).To(Succeed())
		}
		return approval
	}

	newRequest := func(strategy approvalv1.ApprovalStrategy, state approvalv1.ApprovalState, decisions ...approvalv1.Decision) *approvalv1.ApprovalRequest {
		return &approvalv1.ApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "request-" + rand.String(6), Namespace: "default", Labels: labels},
			Spec: approvalv1.ApprovalRequestSpec{
				Target:    target,
				Action:    "subscribe",
				Strategy:  strategy,
				State:     state,
				Requester: requester,
				Decisions: decisions,
			},
		}
	}

	// grantedRequest creates a new request for the target, granted with decisions.
	grantedRequest := func(strategy approvalv1.ApprovalStrategy, decisions ...approvalv1.Decision) *approvalv1.ApprovalRequest {
		ar := newRequest(strategy, approvalv1.ApprovalStateGranted, decisions...)
		Expect(k8sClient.Create(ctx, ar)).To(Succeed())
		// The state change has been notified already.
		ar.Status.LastState = ar.Spec.State
		return ar
	}

	handle := func(ar *approvalv1.ApprovalRequest) error {
		hctx := contextutil.WithRecorder(contextutil.WithEnv(ctx, env), record.NewFakeRecorder(10))
		hctx = cclient.WithClient(hctx, cclient.NewJanitorClient(cclient.NewScopedClient(k8sClient, env)))
		return (&arhandler.ApprovalRequestHandler{}).CreateOrUpdate(hctx, ar)
	}
	current := func(approval *approvalv1.Approval) *approvalv1.Approval {
		live := &approvalv1.Approval{}
		Expect(k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(approval), live)).To(Succeed())
		return live
	}

	DescribeTable("grants the Approval for the new request",
		func(strategy approvalv1.ApprovalStrategy, history []approvalv1.Decision,
			requestStrategy approvalv1.ApprovalStrategy, requestDecisions []approvalv1.Decision,
		) {
			approval := existingApproval(strategy, history...)
			ar := grantedRequest(requestStrategy, requestDecisions...)

			Expect(handle(ar)).To(Succeed())

			live := current(approval)
			Expect(live.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
			Expect(live.Spec.Strategy).To(Equal(requestStrategy))
			Expect(live.Spec.ApprovedRequest.UID).To(Equal(ar.UID))
		},
		Entry("Rejected FourEyes, new FourEyes request granted by two people",
			approvalv1.ApprovalStrategyFourEyes, []approvalv1.Decision{decision("alice", approvalv1.ApprovalStateRejected)},
			approvalv1.ApprovalStrategyFourEyes, fourEyesGrant()),
		Entry("Semigranted FourEyes, new Simple request",
			approvalv1.ApprovalStrategyFourEyes, []approvalv1.Decision{
				decision("alice", approvalv1.ApprovalStateRejected), decision("bob", approvalv1.ApprovalStateSemigranted),
			},
			approvalv1.ApprovalStrategySimple, []approvalv1.Decision{decision("carol", approvalv1.ApprovalStateGranted)}),
		Entry("Semigranted FourEyes, new Auto request",
			approvalv1.ApprovalStrategyFourEyes, []approvalv1.Decision{
				decision("alice", approvalv1.ApprovalStateRejected), decision("bob", approvalv1.ApprovalStateSemigranted),
			},
			approvalv1.ApprovalStrategyAuto, nil),
		// Worked before the fix.
		Entry("Rejected Simple, new Simple request",
			approvalv1.ApprovalStrategySimple, []approvalv1.Decision{decision("alice", approvalv1.ApprovalStateRejected)},
			approvalv1.ApprovalStrategySimple, []approvalv1.Decision{decision("bob", approvalv1.ApprovalStateGranted)}),
		Entry("Rejected FourEyes, new Simple request",
			approvalv1.ApprovalStrategyFourEyes, []approvalv1.Decision{decision("alice", approvalv1.ApprovalStateRejected)},
			approvalv1.ApprovalStrategySimple, []approvalv1.Decision{decision("bob", approvalv1.ApprovalStateGranted)}),
		Entry("Suspended FourEyes, new FourEyes request",
			approvalv1.ApprovalStrategyFourEyes, []approvalv1.Decision{decision("alice", approvalv1.ApprovalStateSuspended)},
			approvalv1.ApprovalStrategyFourEyes, fourEyesGrant()),
		Entry("Granted FourEyes, new FourEyes request",
			approvalv1.ApprovalStrategyFourEyes, nil,
			approvalv1.ApprovalStrategyFourEyes, fourEyesGrant()),
		Entry("Rejected Auto, new Auto request",
			approvalv1.ApprovalStrategyAuto, []approvalv1.Decision{decision("alice", approvalv1.ApprovalStateRejected)},
			approvalv1.ApprovalStrategyAuto, nil),
		Entry("Rejected FourEyes, new Auto request",
			approvalv1.ApprovalStrategyFourEyes, []approvalv1.Decision{decision("alice", approvalv1.ApprovalStateRejected)},
			approvalv1.ApprovalStrategyAuto, nil),
	)

	It("keeps a Rejected FourEyes Approval for a FourEyes request granted by one person", func() {
		approval := existingApproval(approvalv1.ApprovalStrategyFourEyes, decision("alice", approvalv1.ApprovalStateRejected))
		ar := grantedRequest(approvalv1.ApprovalStrategyFourEyes,
			decision("bob", approvalv1.ApprovalStateSemigranted), decision("bob", approvalv1.ApprovalStateGranted))

		Expect(handle(ar)).To(MatchError(ContainSubstring("two distinct deciders")))

		Expect(current(approval).Spec.State).To(Equal(approvalv1.ApprovalStateRejected))
	})

	It("keeps the FSM for a grant of the same request", func() {
		approval := existingApproval(approvalv1.ApprovalStrategyFourEyes, decision("alice", approvalv1.ApprovalStateRejected))
		approval.Spec.State = approvalv1.ApprovalStateGranted
		approval.Spec.Decisions = fourEyesGrant()

		Expect(k8sClient.Update(ctx, approval)).To(MatchError(ContainSubstring("Invalid state transition")))
	})

	DescribeTable("rejects a rebind the referenced request does not back",
		func(before func(*approvalv1.ApprovalRequest), after func(*approvalv1.ApprovalRequest, *ctypes.ObjectRef), message string) {
			approval := existingApproval(approvalv1.ApprovalStrategyFourEyes, decision("alice", approvalv1.ApprovalStateRejected))
			ar := newRequest(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateGranted, fourEyesGrant()...)
			if before != nil {
				before(ar)
			}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())
			ref := ctypes.ObjectRefFromObject(ar)
			if after != nil {
				after(ar, ref)
			}

			approval.Spec.State = approvalv1.ApprovalStateGranted
			approval.Spec.Decisions = fourEyesGrant()
			approval.Spec.ApprovedRequest = ref
			Expect(k8sClient.Update(ctx, approval)).To(MatchError(ContainSubstring(message)))
		},
		Entry("request not granted", func(ar *approvalv1.ApprovalRequest) {
			ar.Spec.State = approvalv1.ApprovalStateSemigranted
			ar.Spec.Decisions = fourEyesGrant()[:1]
		}, nil, "not granted"),
		Entry("request missing", nil, func(_ *approvalv1.ApprovalRequest, ref *ctypes.ObjectRef) {
			ref.Name = "missing-request"
		}, "not found"),
		Entry("request recreated", nil, func(_ *approvalv1.ApprovalRequest, ref *ctypes.ObjectRef) {
			ref.UID = "other-uid"
		}, "UID"),
		Entry("request terminating", func(ar *approvalv1.ApprovalRequest) {
			ar.Finalizers = []string{"test.cp.ei.telekom.de/hold"}
		}, func(ar *approvalv1.ApprovalRequest, _ *ctypes.ObjectRef) {
			Expect(k8sClient.Delete(ctx, ar)).To(Succeed())
			DeferCleanup(func() {
				Expect(k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(ar), ar)).To(Succeed())
				ar.Finalizers = nil
				Expect(k8sClient.Update(ctx, ar)).To(Succeed())
			})
		}, "not granted"),
		Entry("request in another namespace", nil, func(_ *approvalv1.ApprovalRequest, ref *ctypes.ObjectRef) {
			ref.Namespace = "kube-system"
		}, "namespace"),
		Entry("request for another target", func(ar *approvalv1.ApprovalRequest) {
			ar.Spec.Target.Name = "other-subscription"
		}, nil, "another target"),
		Entry("request of another strategy", func(ar *approvalv1.ApprovalRequest) {
			ar.Spec.Strategy = approvalv1.ApprovalStrategySimple
			ar.Spec.Decisions = fourEyesGrant()[1:]
		}, nil, "strategy"),
		Entry("FourEyes request granted by one person", func(ar *approvalv1.ApprovalRequest) {
			ar.Spec.Decisions[1].Email = ar.Spec.Decisions[0].Email
		}, nil, "two distinct deciders"),
	)
})
