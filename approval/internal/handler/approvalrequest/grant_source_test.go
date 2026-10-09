// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = approvalv1.AddToScheme(s)
	return s
}

func baseTarget() ctypes.TypedObjectRef {
	return ctypes.TypedObjectRef{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "testgroup.cp.ei.telekom.de/v1",
			Kind:       "TestResource",
		},
		ObjectRef: ctypes.ObjectRef{
			Name:      "my-resource",
			Namespace: "default",
			UID:       "target-uid-1",
		},
	}
}

func makeAR(name string, uid ktypes.UID, key string, state approvalv1.ApprovalState, target *ctypes.TypedObjectRef) *approvalv1.ApprovalRequest {
	return &approvalv1.ApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       uid,
		},
		Spec: approvalv1.ApprovalRequestSpec{
			Target:      *target,
			ApprovalKey: key,
			State:       state,
			Strategy:    approvalv1.ApprovalStrategySimple,
			Action:      "subscribe",
			Requester: approvalv1.Requester{
				TeamName:  "test--requester",
				TeamEmail: "test@requester.com",
			},
			Decider: approvalv1.Decider{
				TeamName:  "test--decider",
				TeamEmail: "test@decider.com",
			},
		},
	}
}

func fakeReader(objs ...client.Object) client.Reader {
	return fake.NewClientBuilder().
		WithScheme(newScheme()).
		WithObjects(objs...).
		Build()
}

var _ = Describe("loadSoleLiveScopedGrantSource", func() {
	ctx := context.Background()
	target := baseTarget()

	Context("Test A: two live Granted requests (ambiguity)", func() {
		It("rejects with ambiguity error when two Granted ARs target same scope", func() {
			r1 := makeAR("ar-r1", "uid-r1", "provider", approvalv1.ApprovalStateGranted, &target)
			r2 := makeAR("ar-r2", "uid-r2", "provider", approvalv1.ApprovalStateGranted, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader(r1, r2)}

			_, err := h.loadSoleLiveScopedGrantSource(ctx, r1)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ambiguous"))
			Expect(err.Error()).To(ContainSubstring("2 live requests"))
		})

		It("converges when the competing request is removed", func() {
			r2 := makeAR("ar-r2", "uid-r2", "provider", approvalv1.ApprovalStateGranted, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader(r2)}

			result, err := h.loadSoleLiveScopedGrantSource(ctx, r2)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.UID).To(Equal(ktypes.UID("uid-r2")))
			Expect(result.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
		})
	})

	Context("Test B: pending competitor counts in partition", func() {
		It("rejects when old Granted request has a Pending competitor", func() {
			old := makeAR("ar-old", "uid-old", "provider", approvalv1.ApprovalStateGranted, &target)
			pending := makeAR("ar-new", "uid-new", "provider", approvalv1.ApprovalStatePending, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader(old, pending)}

			_, err := h.loadSoleLiveScopedGrantSource(ctx, old)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ambiguous"))
			Expect(err.Error()).To(ContainSubstring("2 live requests"))
		})
	})

	Context("Test C: gate/target isolation", func() {
		It("does not count requests with a different approvalKey", func() {
			provider := makeAR("ar-provider", "uid-prov", "provider", approvalv1.ApprovalStateGranted, &target)
			consumer := makeAR("ar-consumer", "uid-cons", "consumer", approvalv1.ApprovalStateGranted, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader(provider, consumer)}

			result, err := h.loadSoleLiveScopedGrantSource(ctx, provider)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.UID).To(Equal(ktypes.UID("uid-prov")))
		})

		It("does not count unscoped requests", func() {
			scoped := makeAR("ar-scoped", "uid-scoped", "provider", approvalv1.ApprovalStateGranted, &target)
			unscoped := makeAR("ar-unscoped", "uid-unscoped", "", approvalv1.ApprovalStateGranted, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader(scoped, unscoped)}

			result, err := h.loadSoleLiveScopedGrantSource(ctx, scoped)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.UID).To(Equal(ktypes.UID("uid-scoped")))
		})

		It("does not count requests with different target UID", func() {
			ar1 := makeAR("ar-1", "uid-1", "provider", approvalv1.ApprovalStateGranted, &target)
			differentTarget := baseTarget()
			differentTarget.UID = "target-uid-different"
			ar2 := makeAR("ar-2", "uid-2", "provider", approvalv1.ApprovalStateGranted, &differentTarget)

			h := &ApprovalRequestHandler{Reader: fakeReader(ar1, ar2)}

			result, err := h.loadSoleLiveScopedGrantSource(ctx, ar1)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.UID).To(Equal(ktypes.UID("uid-1")))
		})
	})

	Context("Test D: source lifecycle failures", func() {
		It("fails when expected source is absent", func() {
			expected := makeAR("ar-ghost", "uid-ghost", "provider", approvalv1.ApprovalStateGranted, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader() /* empty */}

			_, err := h.loadSoleLiveScopedGrantSource(ctx, expected)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found in live partition"))
		})

		It("fails when expected source has wrong UID (recreated)", func() {
			actual := makeAR("ar-same-name", "uid-actual", "provider", approvalv1.ApprovalStateGranted, &target)
			expected := makeAR("ar-same-name", "uid-expected", "provider", approvalv1.ApprovalStateGranted, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader(actual)}

			_, err := h.loadSoleLiveScopedGrantSource(ctx, expected)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found in live partition"))
		})

		It("fails when expected source is terminating (DeletionTimestamp set)", func() {
			now := metav1.Now()
			terminating := makeAR("ar-term", "uid-term", "provider", approvalv1.ApprovalStateGranted, &target)
			terminating.DeletionTimestamp = &now
			terminating.Finalizers = []string{"test-finalizer"}

			h := &ApprovalRequestHandler{Reader: fakeReader(terminating)}

			_, err := h.loadSoleLiveScopedGrantSource(ctx, terminating)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found in live partition"))
		})

		It("fails when sole live request is not Granted", func() {
			pending := makeAR("ar-pending", "uid-pending", "provider", approvalv1.ApprovalStatePending, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader(pending)}

			_, err := h.loadSoleLiveScopedGrantSource(ctx, pending)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not granted"))
		})

		It("fails when Reader is nil", func() {
			expected := makeAR("ar-nil", "uid-nil", "provider", approvalv1.ApprovalStateGranted, &target)

			h := &ApprovalRequestHandler{Reader: nil}

			_, err := h.loadSoleLiveScopedGrantSource(ctx, expected)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("uncached reader is nil"))
		})
	})

	Context("Test H: existing behavior intact", func() {
		It("succeeds for sole live Granted request", func() {
			sole := makeAR("ar-sole", "uid-sole", "provider", approvalv1.ApprovalStateGranted, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader(sole)}

			result, err := h.loadSoleLiveScopedGrantSource(ctx, sole)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.UID).To(Equal(ktypes.UID("uid-sole")))
			Expect(result.Name).To(Equal("ar-sole"))
			Expect(result.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
		})

		It("handles two different gates on the same owner without ambiguity", func() {
			provider := makeAR("ar-prov", "uid-prov", "provider", approvalv1.ApprovalStateGranted, &target)
			consumer := makeAR("ar-cons", "uid-cons", "consumer", approvalv1.ApprovalStateGranted, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader(provider, consumer)}

			resultP, err := h.loadSoleLiveScopedGrantSource(ctx, provider)
			Expect(err).NotTo(HaveOccurred())
			Expect(resultP.UID).To(Equal(ktypes.UID("uid-prov")))

			resultC, err := h.loadSoleLiveScopedGrantSource(ctx, consumer)
			Expect(err).NotTo(HaveOccurred())
			Expect(resultC.UID).To(Equal(ktypes.UID("uid-cons")))
		})

		It("returns a deep copy that does not alias the original", func() {
			sole := makeAR("ar-copy", "uid-copy", "provider", approvalv1.ApprovalStateGranted, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader(sole)}

			result, err := h.loadSoleLiveScopedGrantSource(ctx, sole)
			Expect(err).NotTo(HaveOccurred())

			result.Spec.ApprovalKey = "mutated"
			Expect(sole.Spec.ApprovalKey).To(Equal("provider"))
		})
	})

	Context("partition edge cases", func() {
		It("includes Rejected requests in partition", func() {
			granted := makeAR("ar-granted", "uid-granted", "provider", approvalv1.ApprovalStateGranted, &target)
			rejected := makeAR("ar-rejected", "uid-rejected", "provider", approvalv1.ApprovalStateRejected, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader(granted, rejected)}

			_, err := h.loadSoleLiveScopedGrantSource(ctx, granted)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ambiguous"))
		})

		It("includes Semigranted requests in partition", func() {
			granted := makeAR("ar-granted", "uid-granted", "provider", approvalv1.ApprovalStateGranted, &target)
			semi := makeAR("ar-semi", "uid-semi", "provider", approvalv1.ApprovalStateSemigranted, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader(granted, semi)}

			_, err := h.loadSoleLiveScopedGrantSource(ctx, granted)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ambiguous"))
		})

		It("handles empty List result when source is absent", func() {
			expected := makeAR("ar-missing", "uid-missing", "provider", approvalv1.ApprovalStateGranted, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader()}

			_, err := h.loadSoleLiveScopedGrantSource(ctx, expected)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found in live partition"))
		})

		It("handles List error gracefully", func() {
			expected := makeAR("ar-err", "uid-err", "provider", approvalv1.ApprovalStateGranted, &target)

			h := &ApprovalRequestHandler{Reader: &errorReader{}}

			_, err := h.loadSoleLiveScopedGrantSource(ctx, expected)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("listing approval requests"))
		})
	})

	Context("Test I: fresh source fields used for Approval spec", func() {
		It("builds the Approval from the API-server version, not the stale in-memory copy", func() {
			sole := makeAR("ar-fresh", "uid-fresh", "provider", approvalv1.ApprovalStateGranted, &target)
			sole.Spec.Requester.TeamName = "fresh--requester"
			sole.Spec.Requester.TeamEmail = "fresh@requester.com"
			sole.Spec.Decider.TeamEmail = "fresh@decider.com"

			h := &ApprovalRequestHandler{Reader: fakeReader(sole)}

			result, err := h.loadSoleLiveScopedGrantSource(ctx, sole)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Spec.Requester.TeamName).To(Equal("fresh--requester"))
			Expect(result.Spec.Requester.TeamEmail).To(Equal("fresh@requester.com"))
			Expect(result.Spec.Decider.TeamEmail).To(Equal("fresh@decider.com"))

			// Mutate the original to confirm the result is a deep copy.
			sole.Spec.Requester.TeamName = "stale--requester"
			Expect(result.Spec.Requester.TeamName).To(Equal("fresh--requester"))
		})
	})

	Context("Test J: already-bound request still detects ambiguity", func() {
		It("returns ambiguity error even when the Approval already points to the source", func() {
			r1 := makeAR("ar-r1", "uid-r1", "provider", approvalv1.ApprovalStateGranted, &target)
			r2 := makeAR("ar-r2", "uid-r2", "provider", approvalv1.ApprovalStateGranted, &target)

			h := &ApprovalRequestHandler{Reader: fakeReader(r1, r2)}

			// Processing r2 (the "already-bound" one) still sees r1 in the partition.
			_, err := h.loadSoleLiveScopedGrantSource(ctx, r2)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ambiguous"))
			Expect(err.Error()).To(ContainSubstring("2 live requests"))

			// After r1 is removed, processing r2 succeeds.
			h2 := &ApprovalRequestHandler{Reader: fakeReader(r2)}
			result, err := h2.loadSoleLiveScopedGrantSource(ctx, r2)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.UID).To(Equal(ktypes.UID("uid-r2")))
			Expect(result.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
		})
	})
})

// errorReader is a client.Reader that always returns an error on List.
type errorReader struct{}

func (e *errorReader) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return fmt.Errorf("injected error")
}

func (e *errorReader) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return fmt.Errorf("injected list error")
}
