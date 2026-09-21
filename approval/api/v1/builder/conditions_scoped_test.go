// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test"
	"github.com/telekom/controlplane/common/pkg/test/testutil"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestConditionTypeForKey is a stdlib test verifying the ConditionTypeForKey helper.
func TestConditionTypeForKey(t *testing.T) {
	cases := []struct {
		key      string
		expected string
	}{
		{"", ConditionTypeApprovalGranted},
		{"provider", ConditionTypeApprovalGranted + "-provider"},
		{"consumer", ConditionTypeApprovalGranted + "-consumer"},
		// max-length key (63 chars — DNS label limit)
		{strings.Repeat("a", 63), ConditionTypeApprovalGranted + "-" + strings.Repeat("a", 63)},
	}

	for _, tc := range cases {
		got := ConditionTypeForKey(tc.key)
		if got != tc.expected {
			t.Errorf("ConditionTypeForKey(%q) = %q; want %q", tc.key, got, tc.expected)
		}
	}
}

var _ = Describe("Scoped condition isolation", func() {

	var condIdx int

	uniqueOwnerName := func(base string) string {
		condIdx++
		return base + "-cond-" + string(rune('0'+condIdx))
	}

	createOwner := func(name string) *test.TestResource {
		owner := test.NewObject(name, testNamespace)
		owner.SetLabels(map[string]string{
			config.EnvironmentLabelKey: testEnvironment,
		})
		Expect(k8sClient.Create(ctx, owner)).To(Succeed())
		return owner
	}

	waitForCacheAR := func(name string) {
		Eventually(func(g Gomega) {
			ar := &approvalv1.ApprovalRequest{}
			g.Expect(k8sm.GetClient().Get(ctx, client.ObjectKey{Name: name, Namespace: testNamespace}, ar)).To(Succeed())
		}, timeout, interval).Should(Succeed())
	}

	waitForCacheApproval := func(name string) {
		Eventually(func(g Gomega) {
			appr := &approvalv1.Approval{}
			g.Expect(k8sm.GetClient().Get(ctx, client.ObjectKey{Name: name, Namespace: testNamespace}, appr)).To(Succeed())
		}, timeout, interval).Should(Succeed())
	}

	BeforeEach(func() {
		condIdx = 0
	})

	AfterEach(func() {
		_ = k8sClient.DeleteAllOf(ctx, &approvalv1.ApprovalRequest{}, client.InNamespace(testNamespace))
		_ = k8sClient.DeleteAllOf(ctx, &approvalv1.Approval{}, client.InNamespace(testNamespace))
		_ = k8sClient.DeleteAllOf(ctx, &test.TestResource{}, client.InNamespace(testNamespace))
	})

	// -----------------------------------------------------------------
	// Provider Granted + Consumer Pending produce two distinct conditions
	// and leave Ready untouched.
	// -----------------------------------------------------------------
	It("Scoped provider Granted + consumer Pending produce distinct conditions; Ready unchanged", func() {
		ownerName := uniqueOwnerName("dual")
		owner := createOwner(ownerName)

		// Set a sentinel Ready=True condition before running any gate.
		owner.SetCondition(newReadySentinel())
		readyBefore := meta.FindStatusCondition(owner.GetConditions(), "Ready")
		Expect(readyBefore).NotTo(BeNil())

		props := map[string]any{"path": "/dual"}
		requester := &approvalv1.Requester{TeamName: "TeamDual", TeamEmail: "dual@telekom.de", Reason: "dual"}
		Expect(requester.SetProperties(props)).To(Succeed())

		// --- provider gate: build + manufacture a bound Granted approval ---
		jcP := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bP := NewApprovalBuilder(jcP, owner)
		bP.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := bP.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		arName := bP.GetApprovalRequest().Name
		arUID := bP.GetApprovalRequest().UID
		approvalName := bP.GetApproval().Name
		waitForCacheAR(arName)

		correctRef := &ctypes.ObjectRef{Name: arName, Namespace: testNamespace, UID: arUID}
		createScopedApproval(approvalName, "provider", bP.GetApprovalRequest().Spec.Target, approvalv1.ApprovalStateGranted, correctRef)
		waitForCacheApproval(approvalName)

		jcP2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bP2 := NewApprovalBuilder(jcP2, owner)
		bP2.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		resP, err := bP2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resP).To(Equal(ApprovalResultGranted))

		// --- consumer gate: still pending ---
		jcC := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bC := NewApprovalBuilder(jcC, owner)
		bC.WithApprovalKey("consumer").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		resC, err := bC.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resC).To(Equal(ApprovalResultPending))

		// Check distinct conditions on the in-memory owner (bC's owner is the same object ref).
		// The owner passed to both builders is the same pointer; conditions accumulate.
		conditions := owner.GetConditions()

		providerCondType := ConditionTypeForKey("provider")
		consumerCondType := ConditionTypeForKey("consumer")
		Expect(providerCondType).NotTo(Equal(consumerCondType))

		providerCond := meta.FindStatusCondition(conditions, providerCondType)
		consumerCond := meta.FindStatusCondition(conditions, consumerCondType)
		unscopedCond := meta.FindStatusCondition(conditions, ConditionTypeApprovalGranted)

		Expect(providerCond).NotTo(BeNil(), "provider condition must exist")
		Expect(consumerCond).NotTo(BeNil(), "consumer condition must exist")
		// Unscoped condition must not be touched by either keyed builder
		Expect(unscopedCond).To(BeNil(), "keyed builders must not write the unscoped ApprovalGranted condition")

		testutil.ExpectConditionToBeTrue(NewGomegaWithT(GinkgoT()), providerCond, "Granted")
		testutil.ExpectConditionToBeFalse(NewGomegaWithT(GinkgoT()), consumerCond, "Pending")

		// Ready sentinel must be unchanged.
		readyAfter := meta.FindStatusCondition(owner.GetConditions(), "Ready")
		Expect(readyAfter).NotTo(BeNil())
		Expect(readyAfter.Status).To(Equal(readyBefore.Status))
		Expect(readyAfter.Reason).To(Equal(readyBefore.Reason))
	})

	// -----------------------------------------------------------------
	// Both gates Pending: two distinct Pending conditions, no overwrite.
	// -----------------------------------------------------------------
	It("Scoped provider Pending + consumer Pending: two distinct Pending conditions", func() {
		ownerName := uniqueOwnerName("twopend")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/twopend"}
		requester := &approvalv1.Requester{TeamName: "TeamPend", TeamEmail: "pend@telekom.de", Reason: "pend"}
		Expect(requester.SetProperties(props)).To(Succeed())

		jcP := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bP := NewApprovalBuilder(jcP, owner)
		bP.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		resP, err := bP.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resP).To(Equal(ApprovalResultPending))

		jcC := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bC := NewApprovalBuilder(jcC, owner)
		bC.WithApprovalKey("consumer").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		resC, err := bC.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resC).To(Equal(ApprovalResultPending))

		providerCond := meta.FindStatusCondition(owner.GetConditions(), ConditionTypeForKey("provider"))
		consumerCond := meta.FindStatusCondition(owner.GetConditions(), ConditionTypeForKey("consumer"))

		Expect(providerCond).NotTo(BeNil())
		Expect(consumerCond).NotTo(BeNil())
		testutil.ExpectConditionToBeFalse(NewGomegaWithT(GinkgoT()), providerCond, "Pending")
		testutil.ExpectConditionToBeFalse(NewGomegaWithT(GinkgoT()), consumerCond, "Pending")
	})

	// -----------------------------------------------------------------
	// Provider Denied (Rejected Approval): keyed Denied condition; consumer unaffected.
	// -----------------------------------------------------------------
	It("Scoped provider Denied does not overwrite consumer condition", func() {
		ownerName := uniqueOwnerName("deniedp")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/denied"}
		requester := &approvalv1.Requester{TeamName: "TeamDenied", TeamEmail: "denied@telekom.de", Reason: "deny"}
		Expect(requester.SetProperties(props)).To(Succeed())

		// First run both builders so both conditions exist.
		jcP := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bP := NewApprovalBuilder(jcP, owner)
		bP.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := bP.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		providerApprovalName := bP.GetApproval().Name
		waitForCacheAR(bP.GetApprovalRequest().Name)

		jcC := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bC := NewApprovalBuilder(jcC, owner)
		bC.WithApprovalKey("consumer").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err = bC.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Now reject the provider approval.
		createScopedApproval(providerApprovalName, "provider", bP.GetApprovalRequest().Spec.Target, approvalv1.ApprovalStateRejected, nil)
		waitForCacheApproval(providerApprovalName)

		// Re-run provider builder to get the Denied condition.
		jcP2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bP2 := NewApprovalBuilder(jcP2, owner)
		bP2.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		resP, err := bP2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resP).To(Equal(ApprovalResultDenied))

		providerCond := meta.FindStatusCondition(owner.GetConditions(), ConditionTypeForKey("provider"))
		consumerCond := meta.FindStatusCondition(owner.GetConditions(), ConditionTypeForKey("consumer"))

		Expect(providerCond).NotTo(BeNil())
		Expect(consumerCond).NotTo(BeNil())

		// Provider should be False/Rejected; consumer should still be False/Pending.
		testutil.ExpectConditionToBeFalse(NewGomegaWithT(GinkgoT()), providerCond, string(approvalv1.ApprovalStateRejected))
		testutil.ExpectConditionToBeFalse(NewGomegaWithT(GinkgoT()), consumerCond, "Pending")
	})

	// -----------------------------------------------------------------
	// Keyed builder does not overwrite the unscoped ApprovalGranted condition.
	// -----------------------------------------------------------------
	It("Scoped keyed condition does not overwrite unscoped ApprovalGranted", func() {
		ownerName := uniqueOwnerName("noxover")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/noxover"}
		requester := &approvalv1.Requester{TeamName: "TeamXOver", TeamEmail: "x@telekom.de", Reason: "xover"}
		Expect(requester.SetProperties(props)).To(Succeed())

		// Seed an unscoped condition manually.
		owner.SetCondition(newApprovalGrantedCondition(approvalv1.ApprovalStateGranted, "manual unscoped grant"))
		unscopedBefore := meta.FindStatusCondition(owner.GetConditions(), ConditionTypeApprovalGranted)
		Expect(unscopedBefore).NotTo(BeNil())

		// Run a keyed builder.
		jcP := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bP := NewApprovalBuilder(jcP, owner)
		bP.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := bP.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Unscoped condition must remain exactly as seeded.
		unscopedAfter := meta.FindStatusCondition(owner.GetConditions(), ConditionTypeApprovalGranted)
		Expect(unscopedAfter).NotTo(BeNil())
		Expect(unscopedAfter.Status).To(Equal(unscopedBefore.Status))
		Expect(unscopedAfter.Reason).To(Equal(unscopedBefore.Reason))
		Expect(unscopedAfter.Message).To(Equal(unscopedBefore.Message))

		// Keyed condition must exist separately.
		providerCond := meta.FindStatusCondition(owner.GetConditions(), ConditionTypeForKey("provider"))
		Expect(providerCond).NotTo(BeNil())
	})

	// -----------------------------------------------------------------
	// Repeated reconcile does not churn conditions (stable message/status).
	// -----------------------------------------------------------------
	It("Scoped repeated reconcile does not churn conditions", func() {
		ownerName := uniqueOwnerName("churn")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/churn"}
		requester := &approvalv1.Requester{TeamName: "TeamChurn", TeamEmail: "churn@telekom.de", Reason: "churn"}
		Expect(requester.SetProperties(props)).To(Succeed())

		var arName string

		jcP := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bP := NewApprovalBuilder(jcP, owner)
		bP.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := bP.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		arName = bP.GetApprovalRequest().Name
		waitForCacheAR(arName)

		cond1 := meta.FindStatusCondition(owner.GetConditions(), ConditionTypeForKey("provider"))
		Expect(cond1).NotTo(BeNil())

		// Second reconcile — same intent, same AR; expect OperationResultNone.
		jcP2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bP2 := NewApprovalBuilder(jcP2, owner)
		bP2.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err = bP2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		cond2 := meta.FindStatusCondition(owner.GetConditions(), ConditionTypeForKey("provider"))
		Expect(cond2).NotTo(BeNil())

		// Status and reason must be stable across reconciles.
		Expect(cond2.Status).To(Equal(cond1.Status))
		Expect(cond2.Reason).To(Equal(cond1.Reason))
	})
})

// newReadySentinel returns a Ready=True condition to use as a sentinel.
func newReadySentinel() metav1.Condition {
	return metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "TestSentinel",
		Message:            "sentinel",
		LastTransitionTime: metav1.Now(),
	}
}
