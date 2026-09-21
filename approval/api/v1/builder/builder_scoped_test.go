// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"fmt"

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

// createScopedApproval creates an Approval with the given scoped fields and
// progresses it to the target state. The Approval is created fresh (not via
// the shared CreateApproval helper) to avoid resourceVersion conflicts.
func createScopedApproval(
	name, key string,
	target ctypes.TypedObjectRef,
	state approvalv1.ApprovalState,
	approvedRequest *ctypes.ObjectRef,
) *approvalv1.Approval {
	appr := &approvalv1.Approval{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: approvalv1.ApprovalSpec{
			Strategy:        approvalv1.ApprovalStrategySimple,
			State:           state,
			ApprovalKey:     key,
			Target:          target,
			ApprovedRequest: approvedRequest,
		},
	}
	ExpectWithOffset(1, k8sClient.Create(ctx, appr)).To(Succeed())

	appr.Status.LastState = state
	ExpectWithOffset(1, k8sClient.Status().Update(ctx, appr)).To(Succeed())

	return appr
}

var _ = Describe("Scoped approval builder", func() {

	var specIdx int

	uniqueOwnerName := func(base string) string {
		specIdx++
		return fmt.Sprintf("%s-scoped-%d", base, specIdx)
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
		specIdx = 0
	})

	AfterEach(func() {
		_ = k8sClient.DeleteAllOf(ctx, &approvalv1.ApprovalRequest{}, client.InNamespace(testNamespace))
		_ = k8sClient.DeleteAllOf(ctx, &approvalv1.Approval{}, client.InNamespace(testNamespace))
		_ = k8sClient.DeleteAllOf(ctx, &test.TestResource{}, client.InNamespace(testNamespace))
	})

	// -------------------------------------------------------------------
	// Provider and consumer requests coexist on the same owner;
	// same-intent reapply retains each UID and decisions.
	// -------------------------------------------------------------------
	It("provider and consumer requests coexist; reapply retains UIDs and decisions", func() {
		ownerName := uniqueOwnerName("coexist")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/shared"}
		requester := &approvalv1.Requester{
			TeamName:  "TeamCoexist",
			TeamEmail: "coexist@telekom.de",
			Reason:    "coexist test",
		}
		Expect(requester.SetProperties(props)).To(Succeed())

		// Build provider gate
		jc1 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b1 := NewApprovalBuilder(jc1, owner)
		b1.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		res1, err := b1.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res1).To(Equal(ApprovalResultPending))

		providerARName := b1.GetApprovalRequest().Name
		providerARUID := b1.GetApprovalRequest().UID
		providerApprovalName := b1.GetApproval().Name

		waitForCacheAR(providerARName)

		// Build consumer gate
		jc2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b2 := NewApprovalBuilder(jc2, owner)
		b2.WithApprovalKey("consumer").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		res2, err := b2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res2).To(Equal(ApprovalResultPending))

		consumerARName := b2.GetApprovalRequest().Name
		consumerARUID := b2.GetApprovalRequest().UID
		consumerApprovalName := b2.GetApproval().Name

		Expect(providerARName).NotTo(Equal(consumerARName), "different keys produce different AR names")
		Expect(providerApprovalName).NotTo(Equal(consumerApprovalName), "different keys produce different Approval names")

		By("Verifying both ARs exist")
		arP := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: providerARName, Namespace: testNamespace}, arP)).To(Succeed())
		Expect(arP.Spec.ApprovalKey).To(Equal("provider"))

		arC := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: consumerARName, Namespace: testNamespace}, arC)).To(Succeed())
		Expect(arC.Spec.ApprovalKey).To(Equal("consumer"))

		By("Simulating a decider granting the provider AR")
		arP.Spec.State = approvalv1.ApprovalStateGranted
		arP.Spec.Decisions = []approvalv1.Decision{
			{Name: "Boss", Email: "boss@telekom.de", Comment: "ok", ResultingState: approvalv1.ApprovalStateGranted},
		}
		Expect(k8sClient.Update(ctx, arP)).To(Succeed())

		By("Reapplying both builders — UIDs and decisions should be preserved")
		jc3 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b3 := NewApprovalBuilder(jc3, owner)
		b3.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err = b3.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(b3.GetApprovalRequest().UID).To(Equal(providerARUID), "provider AR UID unchanged")

		jc4 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b4 := NewApprovalBuilder(jc4, owner)
		b4.WithApprovalKey("consumer").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err = b4.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(b4.GetApprovalRequest().UID).To(Equal(consumerARUID), "consumer AR UID unchanged")

		// Provider decisions preserved
		arPAfter := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: providerARName, Namespace: testNamespace}, arPAfter)).To(Succeed())
		Expect(arPAfter.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
		Expect(arPAfter.Spec.Decisions).To(HaveLen(1))
	})

	// -------------------------------------------------------------------
	// Changing one gate's intent removes only that gate's stale request.
	// -------------------------------------------------------------------
	It("changing one gate's intent removes only that gate's stale request", func() {
		ownerName := uniqueOwnerName("onegate")
		owner := createOwner(ownerName)

		propsA := map[string]any{"path": "/a"}
		propsB := map[string]any{"path": "/b"}
		requester := &approvalv1.Requester{TeamName: "TeamGate", TeamEmail: "gate@telekom.de", Reason: "gate"}
		Expect(requester.SetProperties(propsA)).To(Succeed())

		// Provider with intent A
		jc1 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b1 := NewApprovalBuilder(jc1, owner)
		b1.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := b1.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		providerAR1 := b1.GetApprovalRequest().Name

		// Consumer with intent A
		requesterC := &approvalv1.Requester{TeamName: "TeamGate", TeamEmail: "gate@telekom.de", Reason: "consumer"}
		Expect(requesterC.SetProperties(propsA)).To(Succeed())
		jc2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b2 := NewApprovalBuilder(jc2, owner)
		b2.WithApprovalKey("consumer").WithHashValue(requesterC.Properties).WithRequester(requesterC).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err = b2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		consumerARName := b2.GetApprovalRequest().Name

		waitForCacheAR(providerAR1)
		waitForCacheAR(consumerARName)

		By("Changing provider intent to B")
		requester2 := &approvalv1.Requester{TeamName: "TeamGate", TeamEmail: "gate@telekom.de", Reason: "changed"}
		Expect(requester2.SetProperties(propsB)).To(Succeed())
		jc3 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b3 := NewApprovalBuilder(jc3, owner)
		b3.WithApprovalKey("provider").WithHashValue(requester2.Properties).WithRequester(requester2).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err = b3.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		providerAR2 := b3.GetApprovalRequest().Name
		Expect(providerAR2).NotTo(Equal(providerAR1))

		By("Verifying provider's old AR is gone")
		Eventually(func(g Gomega) {
			ar := &approvalv1.ApprovalRequest{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: providerAR1, Namespace: testNamespace}, ar)
			g.Expect(err).To(HaveOccurred())
		}, timeout, interval).Should(Succeed())

		By("Verifying consumer's AR is untouched")
		arC := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: consumerARName, Namespace: testNamespace}, arC)).To(Succeed())
	})

	// -------------------------------------------------------------------
	// Provider, consumer and unscoped requests coexist on the same owner
	// in every order; unscoped stale cleanup is isolated too.
	// -------------------------------------------------------------------
	It("provider, consumer, and unscoped ARs coexist; cleanup is partition-isolated", func() {
		ownerName := uniqueOwnerName("triple")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/triple"}
		requester := &approvalv1.Requester{TeamName: "TeamTriple", TeamEmail: "triple@telekom.de", Reason: "triple"}
		Expect(requester.SetProperties(props)).To(Succeed())

		// Unscoped
		jcU := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bU := NewApprovalBuilder(jcU, owner)
		bU.WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := bU.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		unscopedARName := bU.GetApprovalRequest().Name

		// Provider
		jcP := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bP := NewApprovalBuilder(jcP, owner)
		bP.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err = bP.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		providerARName := bP.GetApprovalRequest().Name

		// Consumer
		jcC := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bC := NewApprovalBuilder(jcC, owner)
		bC.WithApprovalKey("consumer").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err = bC.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		consumerARName := bC.GetApprovalRequest().Name

		waitForCacheAR(unscopedARName)
		waitForCacheAR(providerARName)
		waitForCacheAR(consumerARName)

		By("Changing unscoped intent — only unscoped stale AR deleted")
		props2 := map[string]any{"path": "/changed"}
		requester2 := &approvalv1.Requester{TeamName: "TeamTriple", TeamEmail: "triple@telekom.de", Reason: "changed"}
		Expect(requester2.SetProperties(props2)).To(Succeed())

		jcU2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		bU2 := NewApprovalBuilder(jcU2, owner)
		bU2.WithHashValue(requester2.Properties).WithRequester(requester2).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err = bU2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		unscopedAR2Name := bU2.GetApprovalRequest().Name
		Expect(unscopedAR2Name).NotTo(Equal(unscopedARName))

		Eventually(func(g Gomega) {
			ar := &approvalv1.ApprovalRequest{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: unscopedARName, Namespace: testNamespace}, ar)
			g.Expect(err).To(HaveOccurred())
		}, timeout, interval).Should(Succeed())

		// Provider and consumer still exist
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: providerARName, Namespace: testNamespace}, &approvalv1.ApprovalRequest{})).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: consumerARName, Namespace: testNamespace}, &approvalv1.ApprovalRequest{})).To(Succeed())
	})

	// -------------------------------------------------------------------
	// Scoped AR has spec.approvalKey and the mirror label set correctly.
	// -------------------------------------------------------------------
	It("sets spec.approvalKey and mirror label on the persisted ApprovalRequest", func() {
		ownerName := uniqueOwnerName("labels")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/label-check"}
		requester := &approvalv1.Requester{TeamName: "TeamLabel", TeamEmail: "label@telekom.de", Reason: "label check"}
		Expect(requester.SetProperties(props)).To(Succeed())

		jc := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b := NewApprovalBuilder(jc, owner)
		b.WithApprovalKey("consumer").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := b.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		ar := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: b.GetApprovalRequest().Name, Namespace: testNamespace}, ar)).To(Succeed())
		Expect(ar.Spec.ApprovalKey).To(Equal("consumer"))
		Expect(ar.Labels[approvalv1.ApprovalKeyLabelKey]).To(Equal("consumer"))
	})

	// -------------------------------------------------------------------
	// Scoped Approval name is durable — does not change when intent hash changes.
	// -------------------------------------------------------------------
	It("scoped Approval name does not change when intent hash changes", func() {
		ownerName := uniqueOwnerName("durable")
		owner := createOwner(ownerName)

		propsA := map[string]any{"path": "/a"}
		propsB := map[string]any{"path": "/b"}
		requester := &approvalv1.Requester{TeamName: "TeamDurable", TeamEmail: "dur@telekom.de", Reason: "durable"}
		Expect(requester.SetProperties(propsA)).To(Succeed())

		jc1 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b1 := NewApprovalBuilder(jc1, owner)
		b1.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := b1.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		approvalName1 := b1.GetApproval().Name
		arName1 := b1.GetApprovalRequest().Name

		requester2 := &approvalv1.Requester{TeamName: "TeamDurable", TeamEmail: "dur@telekom.de", Reason: "changed"}
		Expect(requester2.SetProperties(propsB)).To(Succeed())
		jc2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b2 := NewApprovalBuilder(jc2, owner)
		b2.WithApprovalKey("provider").WithHashValue(requester2.Properties).WithRequester(requester2).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err = b2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(b2.GetApproval().Name).To(Equal(approvalName1), "Approval name is durable across intent changes")
		Expect(b2.GetApprovalRequest().Name).NotTo(Equal(arName1), "AR name changes with intent")
	})

	// -------------------------------------------------------------------
	// No-UID owner, invalid key, omitted hash fail before writes.
	// -------------------------------------------------------------------
	It("rejects keyed builder with no-UID owner", func() {
		owner := test.NewObject("no-uid", testNamespace)
		// Don't create via API — no UID assigned
		owner.SetLabels(map[string]string{config.EnvironmentLabelKey: testEnvironment})

		jc := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b := NewApprovalBuilder(jc, owner)
		b.WithApprovalKey("provider").WithHashValue("whatever").WithRequester(&approvalv1.Requester{TeamName: "T", TeamEmail: "t@t.de", Reason: "r"})
		_, err := b.Build(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("UID"))
	})

	It("rejects keyed builder with invalid key format", func() {
		ownerName := uniqueOwnerName("badkey")
		owner := createOwner(ownerName)

		jc := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b := NewApprovalBuilder(jc, owner)
		b.WithApprovalKey("INVALID_KEY!!").WithHashValue("whatever").WithRequester(&approvalv1.Requester{TeamName: "T", TeamEmail: "t@t.de", Reason: "r"})
		_, err := b.Build(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("DNS label"))
	})

	It("rejects keyed builder with omitted hash", func() {
		ownerName := uniqueOwnerName("nohash")
		owner := createOwner(ownerName)

		jc := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b := NewApprovalBuilder(jc, owner)
		b.WithApprovalKey("provider").WithRequester(&approvalv1.Requester{TeamName: "T", TeamEmail: "t@t.de", Reason: "r"})
		_, err := b.Build(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("WithHashValue is required"))
	})

	// -------------------------------------------------------------------
	// Scoped grant validation: missing ApprovedRequest, wrong UID, stale
	// grant all return non-Granted results.
	// -------------------------------------------------------------------
	It("returns Pending when scoped Approval has no ApprovedRequest", func() {
		ownerName := uniqueOwnerName("nobinding")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/nobind"}
		requester := &approvalv1.Requester{TeamName: "TeamBind", TeamEmail: "bind@telekom.de", Reason: "bind"}
		Expect(requester.SetProperties(props)).To(Succeed())

		jc := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b := NewApprovalBuilder(jc, owner)
		b.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := b.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		arName := b.GetApprovalRequest().Name
		approvalName := b.GetApproval().Name

		By("Creating a granted Approval WITHOUT ApprovedRequest binding")
		approval := createScopedApproval(approvalName, "provider", b.GetApprovalRequest().Spec.Target, approvalv1.ApprovalStateGranted, nil)
		waitForCacheApproval(approvalName)
		waitForCacheAR(arName)
		_ = approval

		By("Running builder again — should be Pending due to missing binding")
		jc2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b2 := NewApprovalBuilder(jc2, owner)
		b2.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		res, err := b2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(ApprovalResultPending))
	})

	It("returns Pending when scoped Approval has wrong UID in ApprovedRequest", func() {
		ownerName := uniqueOwnerName("wronguid")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/wronguid"}
		requester := &approvalv1.Requester{TeamName: "TeamWUID", TeamEmail: "wuid@telekom.de", Reason: "wuid"}
		Expect(requester.SetProperties(props)).To(Succeed())

		jc := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b := NewApprovalBuilder(jc, owner)
		b.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := b.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		arName := b.GetApprovalRequest().Name
		approvalName := b.GetApproval().Name

		By("Creating a granted Approval with stale UID in ApprovedRequest")
		staleRef := &ctypes.ObjectRef{
			Name:      arName,
			Namespace: testNamespace,
			UID:       "stale-uid-00000000-0000-0000-0000-000000000000",
		}
		_ = createScopedApproval(approvalName, "provider", b.GetApprovalRequest().Spec.Target, approvalv1.ApprovalStateGranted, staleRef)
		waitForCacheApproval(approvalName)
		waitForCacheAR(arName)

		jc2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b2 := NewApprovalBuilder(jc2, owner)
		b2.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		res, err := b2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(ApprovalResultPending))
	})

	// -------------------------------------------------------------------
	// Scoped Granted result with correct binding returns Granted.
	// -------------------------------------------------------------------
	It("returns Granted when scoped Approval has correct binding", func() {
		ownerName := uniqueOwnerName("bound")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/bound"}
		requester := &approvalv1.Requester{TeamName: "TeamBound", TeamEmail: "bound@telekom.de", Reason: "bound"}
		Expect(requester.SetProperties(props)).To(Succeed())

		jc := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b := NewApprovalBuilder(jc, owner)
		b.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := b.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		arName := b.GetApprovalRequest().Name
		arUID := b.GetApprovalRequest().UID
		approvalName := b.GetApproval().Name

		By("Creating a correctly-bound granted Approval")
		correctRef := &ctypes.ObjectRef{
			Name:      arName,
			Namespace: testNamespace,
			UID:       arUID,
		}
		_ = createScopedApproval(approvalName, "provider", b.GetApprovalRequest().Spec.Target, approvalv1.ApprovalStateGranted, correctRef)
		waitForCacheApproval(approvalName)

		jc2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b2 := NewApprovalBuilder(jc2, owner)
		b2.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		res, err := b2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(ApprovalResultGranted))

		cond := meta.FindStatusCondition(b2.GetOwner().GetConditions(), ConditionTypeApprovalGranted)
		testutil.ExpectConditionToBeTrue(NewGomegaWithT(GinkgoT()), cond, "Granted")
	})

	// -------------------------------------------------------------------
	// Scoped Denied (Rejected Approval) still works.
	// -------------------------------------------------------------------
	It("returns Denied when scoped Approval is rejected", func() {
		ownerName := uniqueOwnerName("denied")
		owner := createOwner(ownerName)

		props := map[string]any{"path": "/deny"}
		requester := &approvalv1.Requester{TeamName: "TeamDeny", TeamEmail: "deny@telekom.de", Reason: "deny"}
		Expect(requester.SetProperties(props)).To(Succeed())

		jc := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b := NewApprovalBuilder(jc, owner)
		b.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := b.Build(ctx)
		Expect(err).NotTo(HaveOccurred())

		approvalName := b.GetApproval().Name

		By("Creating a rejected Approval with correct identity")
		_ = createScopedApproval(approvalName, "provider", b.GetApprovalRequest().Spec.Target, approvalv1.ApprovalStateRejected, nil)
		waitForCacheApproval(approvalName)

		jc2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b2 := NewApprovalBuilder(jc2, owner)
		b2.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		res, err := b2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(ApprovalResultDenied))
	})

	// -------------------------------------------------------------------
	// A different request hash does not clear a revoked durable Approval.
	// -------------------------------------------------------------------
	It("denied Approval survives hash change on the request", func() {
		ownerName := uniqueOwnerName("revoked")
		owner := createOwner(ownerName)

		propsA := map[string]any{"path": "/a"}
		requester := &approvalv1.Requester{TeamName: "TeamRevoke", TeamEmail: "revoke@telekom.de", Reason: "revoke"}
		Expect(requester.SetProperties(propsA)).To(Succeed())

		jc := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b := NewApprovalBuilder(jc, owner)
		b.WithApprovalKey("provider").WithHashValue(requester.Properties).WithRequester(requester).WithStrategy(approvalv1.ApprovalStrategySimple)
		_, err := b.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		approvalName := b.GetApproval().Name

		_ = createScopedApproval(approvalName, "provider", b.GetApprovalRequest().Spec.Target, approvalv1.ApprovalStateRejected, nil)
		waitForCacheApproval(approvalName)

		By("Changing intent to B — Approval should still be Denied")
		propsB := map[string]any{"path": "/b"}
		requester2 := &approvalv1.Requester{TeamName: "TeamRevoke", TeamEmail: "revoke@telekom.de", Reason: "revoke changed"}
		Expect(requester2.SetProperties(propsB)).To(Succeed())

		jc2 := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
		b2 := NewApprovalBuilder(jc2, owner)
		b2.WithApprovalKey("provider").WithHashValue(requester2.Properties).WithRequester(requester2).WithStrategy(approvalv1.ApprovalStrategySimple)
		res, err := b2.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(ApprovalResultDenied), "denied approval survives hash change")
	})
})
