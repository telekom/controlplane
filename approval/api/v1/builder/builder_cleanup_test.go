// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test"
	"github.com/telekom/controlplane/common/pkg/test/testutil"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Cleanup characterization", func() {
	// Each spec gets a unique owner name via a counter so there is no
	// cross-spec state even though we share the "default" namespace.
	var specIdx int

	uniqueOwnerName := func(base string) string {
		specIdx++
		return fmt.Sprintf("%s-%d", base, specIdx)
	}

	// createOwner creates a TestResource via the API so it gets a real UID,
	// then returns the object with the server-assigned metadata.
	createOwner := func(name string) *test.TestResource {
		owner := test.NewObject(name, testNamespace)
		owner.SetLabels(map[string]string{
			config.EnvironmentLabelKey: testEnvironment,
		})
		Expect(k8sClient.Create(ctx, owner)).To(Succeed())
		return owner
	}

	// waitForCacheAR waits until the manager cache can see the given AR.
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
		specIdx = 0
	})

	AfterEach(func() {
		_ = k8sClient.DeleteAllOf(ctx, &approvalv1.ApprovalRequest{}, client.InNamespace(testNamespace))
		_ = k8sClient.DeleteAllOf(ctx, &approvalv1.Approval{}, client.InNamespace(testNamespace))
		_ = k8sClient.DeleteAllOf(ctx, &test.TestResource{}, client.InNamespace(testNamespace))
	})

	// -------------------------------------------------------------------
	// Case 1: Two sequential unscoped builders, same owner, different hashes.
	// The first AR is removed by cleanup; the second survives; the durable
	// Approval name is unchanged across both calls.
	// -------------------------------------------------------------------
	It("replaces the first request when a second builder uses a different hash (same JanitorClient)", func() {
		ownerName := uniqueOwnerName("seqsame")
		owner := createOwner(ownerName)

		jclient := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))

		hashA := map[string]any{"path": "/a"}
		hashB := map[string]any{"path": "/b"}

		requester := &approvalv1.Requester{
			TeamName:  "TeamSeq",
			TeamEmail: "seq@telekom.de",
			Reason:    "first request",
		}
		Expect(requester.SetProperties(hashA)).To(Succeed())

		b1 := NewApprovalBuilder(jclient, owner)
		b1.WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		res1, err := b1.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res1).To(Equal(ApprovalResultPending))

		firstARName := b1.GetApprovalRequest().Name
		firstARUID := b1.GetApprovalRequest().UID
		approvalName := b1.GetApproval().Name

		// Wait for the cache to see the first AR before running the second builder.
		waitForCacheAR(firstARName)

		By("Running a second builder with a different hash on the same JanitorClient")
		requester2 := &approvalv1.Requester{
			TeamName:  "TeamSeq",
			TeamEmail: "seq@telekom.de",
			Reason:    "second request",
		}
		Expect(requester2.SetProperties(hashB)).To(Succeed())

		// Reset the janitor state so it only tracks the new AR.
		jclient.Reset()

		b2 := NewApprovalBuilder(jclient, owner)
		b2.WithHashValue(requester2.Properties).WithRequester(requester2).WithStrategy(approvalv1.ApprovalStrategySimple)
		res2, err := b2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res2).To(Equal(ApprovalResultPending))

		secondARName := b2.GetApprovalRequest().Name
		Expect(secondARName).NotTo(Equal(firstARName), "different hashes produce different AR names")
		Expect(b2.GetApproval().Name).To(Equal(approvalName), "Approval name is durable across hash changes")

		By("Verifying the first AR was deleted and the second survives")
		Eventually(func(g Gomega) {
			ar := &approvalv1.ApprovalRequest{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: firstARName, Namespace: testNamespace}, ar)
			g.Expect(err).To(HaveOccurred(), "first AR should be cleaned up")
		}, timeout, interval).Should(Succeed())

		ar2 := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: secondARName, Namespace: testNamespace}, ar2)).To(Succeed())
		Expect(ar2.UID).NotTo(Equal(firstARUID), "second AR is a distinct object")
	})

	// Variant: separate JanitorClient instances (new-reconcile pattern).
	It("replaces the first request when a fresh JanitorClient is used per reconcile", func() {
		ownerName := uniqueOwnerName("seqfresh")
		owner := createOwner(ownerName)

		hashA := map[string]any{"path": "/a"}
		hashB := map[string]any{"path": "/b"}

		requester := &approvalv1.Requester{
			TeamName:  "TeamFresh",
			TeamEmail: "fresh@telekom.de",
			Reason:    "first request",
		}
		Expect(requester.SetProperties(hashA)).To(Succeed())

		jc1 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b1 := NewApprovalBuilder(jc1, owner)
		b1.WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		res1, err := b1.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res1).To(Equal(ApprovalResultPending))

		firstARName := b1.GetApprovalRequest().Name
		waitForCacheAR(firstARName)

		By("Second reconcile with a fresh JanitorClient and different hash")
		requester2 := &approvalv1.Requester{
			TeamName:  "TeamFresh",
			TeamEmail: "fresh@telekom.de",
			Reason:    "second request",
		}
		Expect(requester2.SetProperties(hashB)).To(Succeed())

		jc2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b2 := NewApprovalBuilder(jc2, owner)
		b2.WithHashValue(requester2.Properties).WithRequester(requester2).WithStrategy(approvalv1.ApprovalStrategySimple)
		res2, err := b2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res2).To(Equal(ApprovalResultPending))

		secondARName := b2.GetApprovalRequest().Name

		Eventually(func(g Gomega) {
			ar := &approvalv1.ApprovalRequest{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: firstARName, Namespace: testNamespace}, ar)
			g.Expect(err).To(HaveOccurred(), "first AR should be cleaned up")
		}, timeout, interval).Should(Succeed())

		ar2 := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: secondARName, Namespace: testNamespace}, ar2)).To(Succeed())
	})

	// -------------------------------------------------------------------
	// Case 2: Same hash reapplied in a new reconcile — idempotent update,
	// not destructive recreation.
	// -------------------------------------------------------------------
	It("preserves the AR name, UID, and decisions when the same hash is reapplied", func() {
		ownerName := uniqueOwnerName("idem")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/stable"}
		requester := &approvalv1.Requester{
			TeamName:  "TeamIdem",
			TeamEmail: "idem@telekom.de",
			Reason:    "stable request",
		}
		Expect(requester.SetProperties(props)).To(Succeed())

		jc1 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b1 := NewApprovalBuilder(jc1, owner)
		b1.WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := b1.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		arName := b1.GetApprovalRequest().Name
		arUID := b1.GetApprovalRequest().UID

		By("Simulating a decider granting the AR")
		ar := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: arName, Namespace: testNamespace}, ar)).To(Succeed())
		ar.Spec.State = approvalv1.ApprovalStateGranted
		ar.Spec.Decisions = []approvalv1.Decision{
			{Name: "Decider", Email: "d@telekom.de", Comment: "ok", ResultingState: approvalv1.ApprovalStateGranted},
		}
		Expect(k8sClient.Update(ctx, ar)).To(Succeed())

		By("Re-running the builder with the same hash in a new reconcile")
		jc2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b2 := NewApprovalBuilder(jc2, owner)
		b2.WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err = b2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(b2.GetApprovalRequest().Name).To(Equal(arName), "AR name unchanged")
		Expect(b2.GetApprovalRequest().UID).To(Equal(arUID), "AR UID unchanged — no recreation")

		arAfter := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: arName, Namespace: testNamespace}, arAfter)).To(Succeed())
		Expect(arAfter.Spec.State).To(Equal(approvalv1.ApprovalStateGranted), "state preserved")
		Expect(arAfter.Spec.Decisions).To(HaveLen(1), "decisions preserved")
		Expect(arAfter.Spec.Decisions[0].Comment).To(Equal("ok"))
	})

	// -------------------------------------------------------------------
	// Case 3: Changed hash within the unscoped gate — only the stale AR
	// for this owner is removed, not another owner's ARs.
	// -------------------------------------------------------------------
	It("does not delete another owner's ARs when cleaning up stale requests", func() {
		ownerAName := uniqueOwnerName("ownera")
		ownerBName := uniqueOwnerName("ownerb")
		ownerA := createOwner(ownerAName)
		ownerB := createOwner(ownerBName)

		props := map[string]any{"path": "/shared"}
		requesterA := &approvalv1.Requester{TeamName: "TeamA", TeamEmail: "a@telekom.de", Reason: "a"}
		Expect(requesterA.SetProperties(props)).To(Succeed())
		requesterB := &approvalv1.Requester{TeamName: "TeamB", TeamEmail: "b@telekom.de", Reason: "b"}
		Expect(requesterB.SetProperties(props)).To(Succeed())

		By("Creating an AR for owner B")
		jcB := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bB := NewApprovalBuilder(jcB, ownerB)
		bB.WithHashValue(requesterB.Properties).WithRequester(requesterB).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := bB.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		ownerBARName := bB.GetApprovalRequest().Name

		By("Creating an AR for owner A with hash-1")
		jcA1 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bA1 := NewApprovalBuilder(jcA1, ownerA)
		bA1.WithHashValue(requesterA.Properties).WithRequester(requesterA).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err = bA1.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		ownerAFirstAR := bA1.GetApprovalRequest().Name

		waitForCacheAR(ownerAFirstAR)

		By("Running owner A again with hash-2, which triggers cleanup")
		propsNew := map[string]any{"path": "/changed"}
		requesterA2 := &approvalv1.Requester{TeamName: "TeamA", TeamEmail: "a@telekom.de", Reason: "a changed"}
		Expect(requesterA2.SetProperties(propsNew)).To(Succeed())

		jcA2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bA2 := NewApprovalBuilder(jcA2, ownerA)
		bA2.WithHashValue(requesterA2.Properties).WithRequester(requesterA2).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err = bA2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying owner A's old AR is gone")
		Eventually(func(g Gomega) {
			ar := &approvalv1.ApprovalRequest{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: ownerAFirstAR, Namespace: testNamespace}, ar)
			g.Expect(err).To(HaveOccurred())
		}, timeout, interval).Should(Succeed())

		By("Verifying owner B's AR is untouched")
		arB := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ownerBARName, Namespace: testNamespace}, arB)).To(Succeed())
	})

	// -------------------------------------------------------------------
	// Case 4: Granted request reconciled without a new decision —
	// existing decision, state, and owner conditions remain intact.
	// -------------------------------------------------------------------
	It("preserves granted state and owner conditions on re-reconcile", func() {
		ownerName := uniqueOwnerName("granted")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/keep"}
		requester := &approvalv1.Requester{
			TeamName:  "TeamKeep",
			TeamEmail: "keep@telekom.de",
			Reason:    "keep me",
		}
		Expect(requester.SetProperties(props)).To(Succeed())

		jc1 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b1 := NewApprovalBuilder(jc1, owner)
		b1.WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := b1.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		arName := b1.GetApprovalRequest().Name
		approvalName := b1.GetApproval().Name

		By("Creating a granted Approval that points to the AR")
		approval := CreateApproval(approvalName, &ctypes.ObjectRef{Name: arName, Namespace: testNamespace})
		ProgressApproval(approval, approvalv1.ApprovalStateGranted)

		By("Simulating a decider granting the AR")
		ar := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: arName, Namespace: testNamespace}, ar)).To(Succeed())
		ar.Spec.State = approvalv1.ApprovalStateGranted
		ar.Spec.Decisions = []approvalv1.Decision{
			{Name: "Boss", Email: "boss@telekom.de", Comment: "granted", ResultingState: approvalv1.ApprovalStateGranted},
		}
		Expect(k8sClient.Update(ctx, ar)).To(Succeed())

		By("Re-running the builder — no new decision submitted")
		jc2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b2 := NewApprovalBuilder(jc2, owner)
		b2.WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		res, err := b2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(ApprovalResultGranted))

		arAfter := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: arName, Namespace: testNamespace}, arAfter)).To(Succeed())
		Expect(arAfter.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
		Expect(arAfter.Spec.Decisions).To(HaveLen(1))
		Expect(arAfter.Spec.Decisions[0].Comment).To(Equal("granted"))

		cond := meta.FindStatusCondition(b2.GetOwner().GetConditions(), ConditionTypeApprovalGranted)
		testutil.ExpectConditionToBeTrue(NewGomegaWithT(GinkgoT()), cond, "Granted")
	})

	// -------------------------------------------------------------------
	// Case 5a: Rejected Approval — builder returns Denied, condition set.
	// -------------------------------------------------------------------
	It("returns Denied when the Approval is rejected", func() {
		ownerName := uniqueOwnerName("rejected")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/reject"}
		requester := &approvalv1.Requester{
			TeamName:  "TeamReject",
			TeamEmail: "reject@telekom.de",
			Reason:    "reject me",
		}
		Expect(requester.SetProperties(props)).To(Succeed())

		jc := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b := NewApprovalBuilder(jc, owner)
		b.WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)

		approvalName := b.GetApproval().Name

		By("Creating a rejected Approval")
		approval := CreateApproval(approvalName, &ctypes.ObjectRef{Name: "placeholder", Namespace: testNamespace})
		ProgressApproval(approval, approvalv1.ApprovalStateRejected)
		waitForCacheApproval(approvalName)

		res, err := b.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(ApprovalResultDenied))

		cond := meta.FindStatusCondition(b.GetOwner().GetConditions(), ConditionTypeApprovalGranted)
		testutil.ExpectConditionToBeFalse(NewGomegaWithT(GinkgoT()), cond, "Rejected")
	})

	// -------------------------------------------------------------------
	// Case 5b: Suspended Approval — builder returns Denied, condition set.
	// -------------------------------------------------------------------
	It("returns Denied when the Approval is suspended", func() {
		ownerName := uniqueOwnerName("suspended")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/suspend"}
		requester := &approvalv1.Requester{
			TeamName:  "TeamSuspend",
			TeamEmail: "suspend@telekom.de",
			Reason:    "suspend me",
		}
		Expect(requester.SetProperties(props)).To(Succeed())

		jc := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b := NewApprovalBuilder(jc, owner)
		b.WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)

		approvalName := b.GetApproval().Name

		By("Creating a suspended Approval")
		approval := CreateApproval(approvalName, &ctypes.ObjectRef{Name: "placeholder", Namespace: testNamespace})
		ProgressApproval(approval, approvalv1.ApprovalStateSuspended)
		waitForCacheApproval(approvalName)

		res, err := b.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(ApprovalResultDenied))

		cond := meta.FindStatusCondition(b.GetOwner().GetConditions(), ConditionTypeApprovalGranted)
		testutil.ExpectConditionToBeFalse(NewGomegaWithT(GinkgoT()), cond, "Suspended")
	})
})
