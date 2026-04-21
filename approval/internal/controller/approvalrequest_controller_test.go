// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
)

var _ = Describe("ApprovalRequest Controller", func() {

	ctx := context.Background()

	sourceResource := test.NewObject("my-test-resource", testNamespace)
	sourceResource.SetLabels(map[string]string{
		config.EnvironmentLabelKey: testEnvironment,
	})

	requester := approvalv1.Requester{
		TeamName:  "test--requester",
		TeamEmail: "test@requester.com",
		Properties: runtime.RawExtension{
			Raw: []byte(`{"scopes": ["test"]}`),
		},
		ApplicationRef: &ctypes.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{},
			ObjectRef: ctypes.ObjectRef{
				Name:      "requester-app-name",
				Namespace: "default",
			},
		},
	}

	decider := approvalv1.Decider{
		TeamName:  "test--decider",
		TeamEmail: "test@decider.com",
		ApplicationRef: &ctypes.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{},
			ObjectRef: ctypes.ObjectRef{
				Name:      "decider-app-name",
				Namespace: "default",
			},
		},
	}

	arTempl := approvalv1.NewApprovalRequest(sourceResource, sourceResource.Spec)
	arTempl.SetLabels(map[string]string{
		config.EnvironmentLabelKey: testEnvironment,
	})

	Context("When reconciling a resource", func() {

		BeforeEach(func() {
			By("Creating the test resource")
			Expect(k8sClient.Create(ctx, sourceResource)).To(Succeed())
		})

		It("should automatically accept auto-approved approval-requests", func() {

			By("defining the ApprovalRequest with auto strategy and granted state")
			ar := arTempl.DeepCopy()

			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:    *ctypes.TypedObjectRefFromObject(sourceResource, k8sClient.Scheme()),
				Requester: requester,
				Decider:   decider,
				Strategy:  approvalv1.ApprovalStrategyAuto,
				State:     approvalv1.ApprovalStateGranted,
				Action:    "subscribe",
				Decisions: []approvalv1.Decision{
					{Name: "System", Comment: approvalv1.AutoApprovedComment, ResultingState: approvalv1.ApprovalStateGranted},
				},
			}

			By("Creating the ApprovalRequest")
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			Eventually(func(g Gomega) {
				By("Checking the ApprovalRequest object")
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      ar.GetName(),
					Namespace: ar.GetNamespace(),
				}, ar)

				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(ar.Status.Approval.Name).To(Equal("testresource--my-test-resource"))
				g.Expect(ar.Spec.Requester.Properties.Raw).NotTo(BeNil())

				By("Checking the Approval object")
				a := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "testresource--my-test-resource",
					Namespace: ar.GetNamespace(),
				}, a)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				g.Expect(a.Spec.Requester.Properties.Raw).NotTo(BeNil())
				g.Expect(a.Spec.Requester.TeamName).To(Equal("test--requester"))
				g.Expect(a.Spec.Requester.TeamEmail).To(Equal("test@requester.com"))

				g.Expect(a.Spec.Decider.TeamName).To(Equal("test--decider"))
				g.Expect(a.Spec.Decider.TeamEmail).To(Equal("test@decider.com"))

				By("Checking the AUTO-approved decision was carried to the Approval")
				g.Expect(a.Spec.Decisions).To(HaveLen(1))
				g.Expect(a.Spec.Decisions[0].Name).To(Equal("System"))
				g.Expect(a.Spec.Decisions[0].Comment).To(Equal(approvalv1.AutoApprovedComment))
				g.Expect(a.Spec.Decisions[0].ResultingState).To(Equal(approvalv1.ApprovalStateGranted))

				processingCondition := meta.FindStatusCondition(a.Status.Conditions, condition.ConditionTypeProcessing)
				readyCondition := meta.FindStatusCondition(a.Status.Conditions, condition.ConditionTypeReady)

				g.Expect(processingCondition).ToNot(BeNil())
				g.Expect(processingCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(processingCondition.Message).To(Equal("Approval granted"))

				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(readyCondition.Message).To(Equal("Approval has been granted"))

				By("ApprovaRequest simple:granted")
				ar.Spec.Strategy = approvalv1.ApprovalStrategySimple

				err = k8sClient.Update(ctx, ar)
				g.Expect(err).NotTo(HaveOccurred())

				By("Checking notification was created for granted state")
				Expect(ar.Status.NotificationRefs).NotTo(BeNil())
				Expect(ar.Status.NotificationRefs).NotTo(BeEmpty())
				var notification = &notificationv1.Notification{}
				Expect(k8sClient.Get(ctx, ar.Status.NotificationRefs[0].K8s(), notification)).NotTo(HaveOccurred())
				Expect(notification.Spec.Purpose).To(ContainSubstring("approvalrequest--subscribe--created--decider"))

				g.Expect(a.ObjectMeta.OwnerReferences).To(HaveLen(1))
				g.Expect(a.ObjectMeta.OwnerReferences[0].APIVersion).To(Equal("testgroup.cp.ei.telekom.de/v1"))
				g.Expect(a.ObjectMeta.OwnerReferences[0].Kind).To(Equal("TestResource"))
				g.Expect(a.ObjectMeta.OwnerReferences[0].Name).To(Equal("my-test-resource"))

				By("ApprovaRequest simmple:rejected")
				ar.Spec.State = approvalv1.ApprovalStateRejected

				err = k8sClient.Update(ctx, ar)
				g.Expect(err).NotTo(HaveOccurred())

				processingCondition = meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeProcessing)
				readyCondition = meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeReady)

				g.Expect(processingCondition).ToNot(BeNil())
				g.Expect(processingCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(processingCondition.Message).To(Equal("Request rejected"))

				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Message).To(Equal("Request has been rejected"))

			}, timeout, interval).Should(Succeed())
		})

	})

	Context("Simple strategy", func() {

		It("should handle Pending -> Granted transition and carry decisions to Approval", func() {
			By("Creating a separate source resource for this test")
			simpleResource := test.NewObject("simple-granted-src", testNamespace)
			simpleResource.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, simpleResource)).To(Succeed())

			By("Creating a Simple ApprovalRequest in Pending state")
			ar := approvalv1.NewApprovalRequest(simpleResource, "simple-granted")
			ar.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:    *ctypes.TypedObjectRefFromObject(simpleResource, k8sClient.Scheme()),
				Requester: requester,
				Decider:   decider,
				Strategy:  approvalv1.ApprovalStrategySimple,
				State:     approvalv1.ApprovalStatePending,
				Action:    "subscribe",
			}

			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			By("Waiting for the Pending state to be reconciled")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      ar.GetName(),
					Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())

				readyCondition := meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Reason).To(Equal("Pending"))

				g.Expect(ar.Status.AvailableTransitions).NotTo(BeNil())
				g.Expect(ar.Status.AvailableTransitions.HasState(approvalv1.ApprovalStateGranted)).To(BeTrue())
				g.Expect(ar.Status.AvailableTransitions.HasState(approvalv1.ApprovalStateRejected)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			By("Transitioning to Granted with a decision")
			ar.Spec.State = approvalv1.ApprovalStateGranted
			ar.Spec.Decisions = []approvalv1.Decision{
				{Name: "Alice", Email: "alice@example.com", Comment: "Approved", ResultingState: approvalv1.ApprovalStateGranted},
			}
			Expect(k8sClient.Update(ctx, ar)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      ar.GetName(),
					Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())

				By("Checking the ApprovalRequest is now Granted")
				approvedCondition := meta.FindStatusCondition(ar.Status.Conditions, "Approved")
				g.Expect(approvedCondition).ToNot(BeNil())
				g.Expect(approvedCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(approvedCondition.Reason).To(Equal("Granted"))

				By("Checking the Approval object was created with the decision")
				approvalObj := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "testresource--simple-granted-src",
					Namespace: ar.GetNamespace(),
				}, approvalObj)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(approvalObj.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				g.Expect(approvalObj.Spec.Strategy).To(Equal(approvalv1.ApprovalStrategySimple))
				g.Expect(approvalObj.Spec.Decisions).To(HaveLen(1))
				g.Expect(approvalObj.Spec.Decisions[0].Name).To(Equal("Alice"))
				g.Expect(approvalObj.Spec.Decisions[0].Email).To(Equal("alice@example.com"))
			}, timeout, interval).Should(Succeed())
		})

		It("should handle Pending -> Rejected transition with a decision", func() {
			By("Creating a separate source resource for this test")
			simpleResource := test.NewObject("simple-rejected-src", testNamespace)
			simpleResource.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, simpleResource)).To(Succeed())

			By("Creating a Simple ApprovalRequest in Pending state")
			ar := approvalv1.NewApprovalRequest(simpleResource, "simple-rejected")
			ar.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:    *ctypes.TypedObjectRefFromObject(simpleResource, k8sClient.Scheme()),
				Requester: requester,
				Decider:   decider,
				Strategy:  approvalv1.ApprovalStrategySimple,
				State:     approvalv1.ApprovalStatePending,
				Action:    "subscribe",
			}

			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			By("Waiting for the Pending state to be reconciled")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      ar.GetName(),
					Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())

				readyCondition := meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Reason).To(Equal("Pending"))
			}, timeout, interval).Should(Succeed())

			By("Transitioning to Rejected with a decision")
			ar.Spec.State = approvalv1.ApprovalStateRejected
			ar.Spec.Decisions = []approvalv1.Decision{
				{Name: "Bob", Email: "bob@example.com", Comment: "Denied - insufficient justification", ResultingState: approvalv1.ApprovalStateRejected},
			}
			Expect(k8sClient.Update(ctx, ar)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      ar.GetName(),
					Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())

				By("Checking the ApprovalRequest is now Rejected")
				approvedCondition := meta.FindStatusCondition(ar.Status.Conditions, "Approved")
				g.Expect(approvedCondition).ToNot(BeNil())
				g.Expect(approvedCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(approvedCondition.Reason).To(Equal("Rejected"))

				readyCondition := meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Reason).To(Equal("Rejected"))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("FourEyes strategy", func() {

		It("should handle Pending -> Semigranted transition with correct conditions", func() {
			By("Creating a separate source resource for this test")
			fourEyesResource := test.NewObject("foureyes-semigranted-src", testNamespace)
			fourEyesResource.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, fourEyesResource)).To(Succeed())

			By("Creating a FourEyes ApprovalRequest in Semigranted state")
			ar := approvalv1.NewApprovalRequest(fourEyesResource, "foureyes-semigranted")
			ar.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:    *ctypes.TypedObjectRefFromObject(fourEyesResource, k8sClient.Scheme()),
				Requester: requester,
				Decider:   decider,
				Strategy:  approvalv1.ApprovalStrategyFourEyes,
				State:     approvalv1.ApprovalStateSemigranted,
				Action:    "subscribe",
				Decisions: []approvalv1.Decision{
					{Name: "Alice", Email: "alice@example.com", Comment: "First approval", ResultingState: approvalv1.ApprovalStateSemigranted},
				},
			}

			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      ar.GetName(),
					Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())

				By("Checking Semigranted conditions on ApprovalRequest")
				approvedCondition := meta.FindStatusCondition(ar.Status.Conditions, "Approved")
				g.Expect(approvedCondition).ToNot(BeNil())
				g.Expect(approvedCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(approvedCondition.Reason).To(Equal("Semigranted"))

				processingCondition := meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeProcessing)
				g.Expect(processingCondition).ToNot(BeNil())
				g.Expect(processingCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(processingCondition.Reason).To(Equal("Semigranted"))
				g.Expect(processingCondition.Message).To(Equal("Request partially approved, awaiting second approval"))

				readyCondition := meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Reason).To(Equal("Semigranted"))

				By("Verifying available transitions include Granted and Rejected from Semigranted")
				g.Expect(ar.Status.AvailableTransitions).NotTo(BeNil())
				g.Expect(ar.Status.AvailableTransitions.HasState(approvalv1.ApprovalStateGranted)).To(BeTrue())
				g.Expect(ar.Status.AvailableTransitions.HasState(approvalv1.ApprovalStateRejected)).To(BeTrue())

			}, timeout, interval).Should(Succeed())
		})

		It("should handle Semigranted -> Granted transition and create Approval", func() {
			By("Creating a separate source resource for this test")
			fourEyesResource := test.NewObject("foureyes-granted-src", testNamespace)
			fourEyesResource.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, fourEyesResource)).To(Succeed())

			By("Creating a FourEyes ApprovalRequest starting in Semigranted state")
			ar := approvalv1.NewApprovalRequest(fourEyesResource, "foureyes-granted")
			ar.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:    *ctypes.TypedObjectRefFromObject(fourEyesResource, k8sClient.Scheme()),
				Requester: requester,
				Decider:   decider,
				Strategy:  approvalv1.ApprovalStrategyFourEyes,
				State:     approvalv1.ApprovalStateSemigranted,
				Action:    "subscribe",
				Decisions: []approvalv1.Decision{
					{Name: "Alice", Email: "alice@example.com", Comment: "First approval", ResultingState: approvalv1.ApprovalStateSemigranted},
				},
			}

			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			By("Waiting for the Semigranted state to be reconciled")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      ar.GetName(),
					Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())

				approvedCondition := meta.FindStatusCondition(ar.Status.Conditions, "Approved")
				g.Expect(approvedCondition).ToNot(BeNil())
				g.Expect(approvedCondition.Reason).To(Equal("Semigranted"))
			}, timeout, interval).Should(Succeed())

			By("Transitioning to Granted with a second decision from a different person")
			ar.Spec.State = approvalv1.ApprovalStateGranted
			ar.Spec.Decisions = append(ar.Spec.Decisions, approvalv1.Decision{
				Name: "Bob", Email: "bob@example.com", Comment: "Second approval", ResultingState: approvalv1.ApprovalStateGranted,
			})
			Expect(k8sClient.Update(ctx, ar)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      ar.GetName(),
					Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())

				By("Checking the ApprovalRequest is now Granted")
				approvedCondition := meta.FindStatusCondition(ar.Status.Conditions, "Approved")
				g.Expect(approvedCondition).ToNot(BeNil())
				g.Expect(approvedCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(approvedCondition.Reason).To(Equal("Granted"))

				By("Checking the Approval object was created with Granted state")
				approvalObj := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "testresource--foureyes-granted-src",
					Namespace: ar.GetNamespace(),
				}, approvalObj)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(approvalObj.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				g.Expect(approvalObj.Spec.Strategy).To(Equal(approvalv1.ApprovalStrategyFourEyes))
				g.Expect(approvalObj.Spec.Decisions).To(HaveLen(2))
			}, timeout, interval).Should(Succeed())
		})

		It("should not notify requester on Semigranted state", func() {
			By("Creating a separate source resource for this test")
			fourEyesResource := test.NewObject("foureyes-notify-src", testNamespace)
			fourEyesResource.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, fourEyesResource)).To(Succeed())

			By("Creating a FourEyes ApprovalRequest in Semigranted state")
			ar := approvalv1.NewApprovalRequest(fourEyesResource, "foureyes-notify")
			ar.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:    *ctypes.TypedObjectRefFromObject(fourEyesResource, k8sClient.Scheme()),
				Requester: requester,
				Decider:   decider,
				Strategy:  approvalv1.ApprovalStrategyFourEyes,
				State:     approvalv1.ApprovalStateSemigranted,
				Action:    "subscribe",
				Decisions: []approvalv1.Decision{
					{Name: "Alice", Email: "alice@example.com", Comment: "First approval", ResultingState: approvalv1.ApprovalStateSemigranted},
				},
			}

			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      ar.GetName(),
					Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())

				By("Checking that only the decider notification was sent (not the requester)")
				// For Semigranted, shouldNotifyRequester returns false,
				// so only 1 notification (to the decider) should be created
				g.Expect(ar.Status.NotificationRefs).To(HaveLen(1))

				notif := &notificationv1.Notification{}
				err = k8sClient.Get(ctx, ar.Status.NotificationRefs[0].K8s(), notif)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(notif.Spec.Purpose).To(ContainSubstring("decider"))
			}, timeout, interval).Should(Succeed())
		})

		It("should handle Semigranted -> Rejected transition", func() {
			By("Creating a separate source resource for this test")
			fourEyesResource := test.NewObject("foureyes-rejected-src", testNamespace)
			fourEyesResource.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, fourEyesResource)).To(Succeed())

			By("Creating a FourEyes ApprovalRequest in Semigranted state")
			ar := approvalv1.NewApprovalRequest(fourEyesResource, "foureyes-rejected")
			ar.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:    *ctypes.TypedObjectRefFromObject(fourEyesResource, k8sClient.Scheme()),
				Requester: requester,
				Decider:   decider,
				Strategy:  approvalv1.ApprovalStrategyFourEyes,
				State:     approvalv1.ApprovalStateSemigranted,
				Action:    "subscribe",
				Decisions: []approvalv1.Decision{
					{Name: "Alice", Email: "alice@example.com", Comment: "First approval", ResultingState: approvalv1.ApprovalStateSemigranted},
				},
			}

			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			By("Waiting for Semigranted reconciliation")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      ar.GetName(),
					Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())

				approvedCondition := meta.FindStatusCondition(ar.Status.Conditions, "Approved")
				g.Expect(approvedCondition).ToNot(BeNil())
				g.Expect(approvedCondition.Reason).To(Equal("Semigranted"))
			}, timeout, interval).Should(Succeed())

			By("Transitioning to Rejected")
			ar.Spec.State = approvalv1.ApprovalStateRejected
			ar.Spec.Decisions = append(ar.Spec.Decisions, approvalv1.Decision{
				Name: "Bob", Email: "bob@example.com", Comment: "Denied", ResultingState: approvalv1.ApprovalStateRejected,
			})
			Expect(k8sClient.Update(ctx, ar)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      ar.GetName(),
					Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())

				approvedCondition := meta.FindStatusCondition(ar.Status.Conditions, "Approved")
				g.Expect(approvedCondition).ToNot(BeNil())
				g.Expect(approvedCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(approvedCondition.Reason).To(Equal("Rejected"))

				readyCondition := meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Reason).To(Equal("Rejected"))
			}, timeout, interval).Should(Succeed())
		})
	})
})
