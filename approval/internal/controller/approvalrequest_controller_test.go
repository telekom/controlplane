// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApprovalRequest Controller", func() {
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
				notification := &notificationv1.Notification{}
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

	Context("scoped approval handling", func() {
		It("creates a scoped Approval with ag-v1- prefix and correct key", func() {
			By("Creating a source resource for the scoped test")
			src := test.NewObject("scoped-basic-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			target := *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme())

			By("Computing the expected scoped Approval name")
			expectedApprovalName, err := approvalv1.ScopedApprovalName(target, approvalv1.ApprovalKeyProvider)
			Expect(err).NotTo(HaveOccurred())
			Expect(expectedApprovalName).To(HavePrefix("ag-v1-"))

			By("Creating a keyed auto-approved ApprovalRequest")
			ar := approvalv1.NewApprovalRequest(src, "scoped-basic")
			ar.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:      target,
				Requester:   requester,
				Decider:     decider,
				Strategy:    approvalv1.ApprovalStrategyAuto,
				State:       approvalv1.ApprovalStateGranted,
				Action:      "subscribe",
				ApprovalKey: approvalv1.ApprovalKeyProvider,
				Decisions: []approvalv1.Decision{
					{
						Name:           approvalv1.SystemDecisionName,
						Comment:        approvalv1.AutoApprovedComment,
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			}

			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name: ar.GetName(), Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ar.Status.Approval.Name).To(Equal(expectedApprovalName))

				a := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name: expectedApprovalName, Namespace: ar.GetNamespace(),
				}, a)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(a.Spec.ApprovalKey).To(Equal(approvalv1.ApprovalKeyProvider))
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				g.Expect(a.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(a.Spec.ApprovedRequest.Name).To(Equal(ar.GetName()))
				// The keyed builder requires exactly this controller owner on a persisted grant.
				g.Expect(metav1.IsControlledBy(a, src)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		DescribeTable("does not adopt or grant an existing scoped Approval without this target's controller owner",
			func(suffix string, ownerRefs []metav1.OwnerReference) {
				By("Creating a source resource")
				src := test.NewObject("scoped-"+suffix+"-src", testNamespace)
				src.SetLabels(map[string]string{
					config.EnvironmentLabelKey: testEnvironment,
				})
				Expect(k8sClient.Create(ctx, src)).To(Succeed())
				DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

				target := *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme())
				approvalName, err := approvalv1.ScopedApprovalName(target, approvalv1.ApprovalKeyProvider)
				Expect(err).NotTo(HaveOccurred())

				By("Pre-creating a Granted scoped Approval at the deterministic name")
				existing := &approvalv1.Approval{
					ObjectMeta: metav1.ObjectMeta{
						Name:            approvalName,
						Namespace:       testNamespace,
						OwnerReferences: ownerRefs,
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: approvalv1.ApprovalSpec{
						State:       approvalv1.ApprovalStateGranted,
						Target:      target,
						ApprovalKey: approvalv1.ApprovalKeyProvider,
						Strategy:    approvalv1.ApprovalStrategySimple,
						Action:      "subscribe",
						Requester:   requester,
						Decider:     decider,
					},
				}
				Expect(k8sClient.Create(ctx, existing)).To(Succeed())

				By("Waiting for the Approval controller to settle the pre-created Approval")
				var rv string
				Eventually(func(g Gomega) {
					a := &approvalv1.Approval{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), a)).To(Succeed())
					g.Expect(meta.IsStatusConditionTrue(a.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
					rv = a.ResourceVersion
				}, timeout, interval).Should(Succeed())

				By("Creating a granted keyed ApprovalRequest for the same gate")
				ar := approvalv1.NewApprovalRequest(src, "scoped-"+suffix)
				ar.SetLabels(map[string]string{
					config.EnvironmentLabelKey: testEnvironment,
				})
				ar.Spec = approvalv1.ApprovalRequestSpec{
					Target:      target,
					Requester:   requester,
					Decider:     decider,
					Strategy:    approvalv1.ApprovalStrategyAuto,
					State:       approvalv1.ApprovalStateGranted,
					Action:      "subscribe",
					ApprovalKey: approvalv1.ApprovalKeyProvider,
					Decisions: []approvalv1.Decision{
						{
							Name:           approvalv1.SystemDecisionName,
							Comment:        approvalv1.AutoApprovedComment,
							ResultingState: approvalv1.ApprovalStateGranted,
						},
					},
				}
				Expect(k8sClient.Create(ctx, ar)).To(Succeed())

				By("Waiting for the request to fail on the missing or foreign authority")
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar), ar)).To(Succeed())
					ready := meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeReady)
					g.Expect(ready).NotTo(BeNil())
					g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
					g.Expect(ready.Message).To(ContainSubstring("missing or foreign controller owner"))
				}, timeout, interval).Should(Succeed())

				By("Verifying the Approval is never adopted, bound, or rewritten across retries")
				Consistently(func(g Gomega) {
					a := &approvalv1.Approval{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), a)).To(Succeed())
					g.Expect(a.ResourceVersion).To(Equal(rv))
					g.Expect(a.OwnerReferences).To(Equal(ownerRefs))
					g.Expect(a.Spec.ApprovedRequest).To(BeNil())
				}, 2*time.Second, interval).Should(Succeed())
			},
			Entry("no ownerReferences", "orphan", nil),
			Entry("foreign controller", "foreign", []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "foreign-object",
				UID:        ktypes.UID("foreign-uid-000"),
				Controller: ptr.To(true),
			}}),
		)

		It("creates two independent Approvals for different keys on the SAME target", func() {
			By("Creating one source resource shared by both keyed ARs")
			src := test.NewObject("scoped-same-target-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			target := *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme())

			nameA, err := approvalv1.ScopedApprovalName(target, "provider")
			Expect(err).NotTo(HaveOccurred())
			nameB, err := approvalv1.ScopedApprovalName(target, "consumer")
			Expect(err).NotTo(HaveOccurred())
			Expect(nameA).NotTo(Equal(nameB))

			By("Creating the provider keyed ApprovalRequest")
			arA := approvalv1.NewApprovalRequest(src, "same-tgt-provider")
			arA.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			arA.Spec = approvalv1.ApprovalRequestSpec{
				Target:      target,
				Requester:   requester,
				Decider:     decider,
				Strategy:    approvalv1.ApprovalStrategyAuto,
				State:       approvalv1.ApprovalStateGranted,
				Action:      "subscribe",
				ApprovalKey: "provider",
				Decisions: []approvalv1.Decision{
					{
						Name:           approvalv1.SystemDecisionName,
						Comment:        approvalv1.AutoApprovedComment,
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			}
			Expect(k8sClient.Create(ctx, arA)).To(Succeed())

			By("Creating the consumer keyed ApprovalRequest on the SAME target")
			arB := approvalv1.NewApprovalRequest(src, "same-tgt-consumer")
			arB.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			arB.Spec = approvalv1.ApprovalRequestSpec{
				Target:      target,
				Requester:   requester,
				Decider:     decider,
				Strategy:    approvalv1.ApprovalStrategyAuto,
				State:       approvalv1.ApprovalStateGranted,
				Action:      "subscribe",
				ApprovalKey: "consumer",
				Decisions: []approvalv1.Decision{
					{
						Name:           approvalv1.SystemDecisionName,
						Comment:        approvalv1.AutoApprovedComment,
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			}
			Expect(k8sClient.Create(ctx, arB)).To(Succeed())

			Eventually(func(g Gomega) {
				By("Verifying both Approvals exist with correct keys")
				aA := &approvalv1.Approval{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: nameA, Namespace: testNamespace}, aA)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(aA.Spec.ApprovalKey).To(Equal("provider"))
				g.Expect(aA.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				g.Expect(aA.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(aA.Spec.ApprovedRequest.Name).To(Equal(arA.GetName()))

				aB := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{Name: nameB, Namespace: testNamespace}, aB)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(aB.Spec.ApprovalKey).To(Equal("consumer"))
				g.Expect(aB.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				g.Expect(aB.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(aB.Spec.ApprovedRequest.Name).To(Equal(arB.GetName()))

				By("Verifying each AR has notification refs (no ownership conflict)")
				err = k8sClient.Get(ctx, client.ObjectKey{Name: arA.GetName(), Namespace: testNamespace}, arA)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(arA.Status.NotificationRefs).NotTo(BeEmpty())

				err = k8sClient.Get(ctx, client.ObjectKey{Name: arB.GetName(), Namespace: testNamespace}, arB)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(arB.Status.NotificationRefs).NotTo(BeEmpty())

				By("Verifying notifications have different names")
				notifA := &notificationv1.Notification{}
				err = k8sClient.Get(ctx, arA.Status.NotificationRefs[0].K8s(), notifA)
				g.Expect(err).NotTo(HaveOccurred())

				notifB := &notificationv1.Notification{}
				err = k8sClient.Get(ctx, arB.Status.NotificationRefs[0].K8s(), notifB)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(notifA.Name).NotTo(Equal(notifB.Name))
			}, timeout, interval).Should(Succeed())
		})

		It("unscoped request still creates a legacy-named Approval", func() {
			By("Creating a source resource")
			src := test.NewObject("scoped-legacy-compat-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			ar := approvalv1.NewApprovalRequest(src, "scoped-legacy-compat")
			ar.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:    *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme()),
				Requester: requester,
				Decider:   decider,
				Strategy:  approvalv1.ApprovalStrategyAuto,
				State:     approvalv1.ApprovalStateGranted,
				Action:    "subscribe",
				Decisions: []approvalv1.Decision{
					{
						Name:           approvalv1.SystemDecisionName,
						Comment:        approvalv1.AutoApprovedComment,
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			}

			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			legacyName := "testresource--scoped-legacy-compat-src"

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name: ar.GetName(), Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ar.Status.Approval.Name).To(Equal(legacyName))

				a := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name: legacyName, Namespace: ar.GetNamespace(),
				}, a)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(a.Spec.ApprovalKey).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("does not overwrite a revoked scoped Approval with a new grant", func() {
			By("Creating a source resource")
			src := test.NewObject("scoped-revoke-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			target := *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme())
			approvalName, err := approvalv1.ScopedApprovalName(target, approvalv1.ApprovalKeyProvider)
			Expect(err).NotTo(HaveOccurred())

			By("Pre-creating a Rejected scoped Approval")
			revokedApproval := &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name:      approvalName,
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: approvalv1.ApprovalSpec{
					State:       approvalv1.ApprovalStateRejected,
					Target:      target,
					ApprovalKey: approvalv1.ApprovalKeyProvider,
					Strategy:    approvalv1.ApprovalStrategySimple,
					Action:      "subscribe",
					Requester:   requester,
					Decider:     decider,
					Decisions: []approvalv1.Decision{
						{Name: "Admin", Comment: "Access revoked", ResultingState: approvalv1.ApprovalStateRejected},
					},
				},
			}
			Expect(k8sClient.Create(ctx, revokedApproval)).To(Succeed())

			By("Creating a granted keyed ApprovalRequest that targets the same gate")
			ar := approvalv1.NewApprovalRequest(src, "scoped-revoke")
			ar.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:      target,
				Requester:   requester,
				Decider:     decider,
				Strategy:    approvalv1.ApprovalStrategyAuto,
				State:       approvalv1.ApprovalStateGranted,
				Action:      "subscribe",
				ApprovalKey: approvalv1.ApprovalKeyProvider,
				Decisions: []approvalv1.Decision{
					{
						Name:           approvalv1.SystemDecisionName,
						Comment:        approvalv1.AutoApprovedComment,
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			By("Waiting for the AR to be reconciled, then verifying the Approval stayed Rejected")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name: ar.GetName(), Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())

				a := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name: approvalName, Namespace: testNamespace,
				}, a)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateRejected))
				g.Expect(a.Spec.Decisions).To(HaveLen(1))
				g.Expect(a.Spec.Decisions[0].Comment).To(Equal("Access revoked"))
				g.Expect(a.OwnerReferences).To(BeEmpty())

				ready := meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(ready.Reason).To(Equal("Revoked"))
				g.Expect(ready.Message).To(ContainSubstring("approval stays revoked"))
			}, timeout, interval).Should(Succeed())
		})

		It("lifts a revoked scoped Approval when a person grants a newer request", func() {
			By("Creating a source resource")
			src := test.NewObject("scoped-lift-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			target := *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme())
			approvalName, err := approvalv1.ScopedApprovalName(target, approvalv1.ApprovalKeyProvider)
			Expect(err).NotTo(HaveOccurred())
			approvalKey := client.ObjectKey{Name: approvalName, Namespace: testNamespace}

			By("Granting the first request by a person")
			ar1 := newScopedRequest(src, "scoped-lift-a", &target, approvalv1.ApprovalKeyProvider, &requester, &decider,
				approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted,
				approvalv1.Decision{Name: "Alice", Email: "alice@decider.com", ResultingState: approvalv1.ApprovalStateGranted})
			Expect(k8sClient.Create(ctx, ar1)).To(Succeed())
			Eventually(func(g Gomega) {
				a := &approvalv1.Approval{}
				g.Expect(k8sClient.Get(ctx, approvalKey, a)).To(Succeed())
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				g.Expect(a.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar1.UID))
			}, timeout, interval).Should(Succeed())

			By("Revoking the Approval")
			revokeApproval(approvalKey)

			By("Keeping the revocation for the request it was granted for")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar1), ar1)).To(Succeed())
				ready := meta.FindStatusCondition(ar1.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.Reason).To(Equal("Revoked"))
			}, timeout, interval).Should(Succeed())

			By("Replacing the first request with a newer one, as an intent change does")
			deleteRequest(ar1)
			ar2 := newScopedRequest(src, "scoped-lift-b", &target, approvalv1.ApprovalKeyProvider, &requester, &decider,
				approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending)
			Expect(k8sClient.Create(ctx, ar2)).To(Succeed())

			By("Keeping the revocation while the newer request is only Pending")
			Consistently(func(g Gomega) {
				a := &approvalv1.Approval{}
				g.Expect(k8sClient.Get(ctx, approvalKey, a)).To(Succeed())
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateRejected))
			}, time.Second, interval).Should(Succeed())

			By("Granting the newer request by a person, after the revocation")
			grantedAt := metav1.Now()
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar2), ar2)).To(Succeed())
				ar2.Spec.State = approvalv1.ApprovalStateGranted
				ar2.AppendDecision(approvalv1.Decision{Name: "Bob", Email: "bob@decider.com", Timestamp: &grantedAt, ResultingState: approvalv1.ApprovalStateGranted})
				g.Expect(k8sClient.Update(ctx, ar2)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying the Approval is granted, bound to the newer request, and keeps the revocation decision")
			Eventually(func(g Gomega) {
				a := &approvalv1.Approval{}
				g.Expect(k8sClient.Get(ctx, approvalKey, a)).To(Succeed())
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				g.Expect(a.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar2.UID))
				g.Expect(a.Spec.Decisions).To(ContainElement(SatisfyAll(
					HaveField("Comment", "Access revoked"), HaveField("ResultingState", approvalv1.ApprovalStateRejected))))
				g.Expect(a.Spec.Decisions).To(ContainElement(SatisfyAll(
					HaveField("Name", "Bob"), HaveField("ResultingState", approvalv1.ApprovalStateGranted))))

				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar2), ar2)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(ar2.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("keeps a revoked scoped Approval when the newer request was granted before the revocation", func() {
			By("Creating a source resource")
			src := test.NewObject("scoped-stale-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			target := *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme())
			approvalName, err := approvalv1.ScopedApprovalName(target, approvalv1.ApprovalKeyProvider)
			Expect(err).NotTo(HaveOccurred())
			approvalKey := client.ObjectKey{Name: approvalName, Namespace: testNamespace}

			By("Granting the first request by a person")
			ar1 := newScopedRequest(src, "scoped-stale-a", &target, approvalv1.ApprovalKeyProvider, &requester, &decider,
				approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted,
				approvalv1.Decision{Name: "Alice", Email: "alice@decider.com", ResultingState: approvalv1.ApprovalStateGranted})
			Expect(k8sClient.Create(ctx, ar1)).To(Succeed())
			Eventually(func(g Gomega) {
				a := &approvalv1.Approval{}
				g.Expect(k8sClient.Get(ctx, approvalKey, a)).To(Succeed())
				g.Expect(a.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar1.UID))
			}, timeout, interval).Should(Succeed())

			By("Revoking the Approval and replacing the request with one granted an hour before the revocation")
			revokeApproval(approvalKey)
			deleteRequest(ar1)
			grantedBefore := metav1.NewTime(time.Now().Add(-time.Hour))
			ar2 := newScopedRequest(src, "scoped-stale-b", &target, approvalv1.ApprovalKeyProvider, &requester, &decider,
				approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted,
				approvalv1.Decision{Name: "Bob", Email: "bob@decider.com", Timestamp: &grantedBefore, ResultingState: approvalv1.ApprovalStateGranted})
			Expect(k8sClient.Create(ctx, ar2)).To(Succeed())

			By("Keeping the revocation and reporting it on the newer request")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar2), ar2)).To(Succeed())
				ready := meta.FindStatusCondition(ar2.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.Reason).To(Equal("Revoked"))
			}, timeout, interval).Should(Succeed())
			Consistently(func(g Gomega) {
				a := &approvalv1.Approval{}
				g.Expect(k8sClient.Get(ctx, approvalKey, a)).To(Succeed())
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateRejected))
				g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar1.UID))
			}, time.Second, interval).Should(Succeed())
		})

		It("does not recreate a deleted revoked scoped Approval from an automatic grant", func() {
			By("Creating a source resource")
			src := test.NewObject("scoped-deleted-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			target := *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme())
			approvalName, err := approvalv1.ScopedApprovalName(target, approvalv1.ApprovalKeyProvider)
			Expect(err).NotTo(HaveOccurred())
			approvalKey := client.ObjectKey{Name: approvalName, Namespace: testNamespace}

			By("Auto-granting a request, which creates the Approval the first time")
			ar := newScopedRequest(src, "scoped-deleted-a", &target, approvalv1.ApprovalKeyProvider, &requester, &decider,
				approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted,
				approvalv1.Decision{Name: approvalv1.SystemDecisionName, Comment: approvalv1.AutoApprovedComment, ResultingState: approvalv1.ApprovalStateGranted})
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())
			Eventually(func(g Gomega) {
				a := &approvalv1.Approval{}
				g.Expect(k8sClient.Get(ctx, approvalKey, a)).To(Succeed())
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar), ar)).To(Succeed())
				g.Expect(ar.Status.Approval.UID).To(Equal(a.UID))
			}, timeout, interval).Should(Succeed())

			By("Revoking the Approval, then deleting it")
			revokeApproval(approvalKey)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar), ar)).To(Succeed())
				ready := meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.Reason).To(Equal("Revoked"))
			}, timeout, interval).Should(Succeed())
			deleteApproval(approvalKey)

			By("Touching the request, so it is reconciled without the Approval")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar), ar)).To(Succeed())
				ar.Annotations = map[string]string{"test.cp.ei.telekom.de/touch": "1"}
				g.Expect(k8sClient.Update(ctx, ar)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying the Approval is not recreated")
			Consistently(func(g Gomega) {
				err := k8sClient.Get(ctx, approvalKey, &approvalv1.Approval{})
				g.Expect(err).To(HaveOccurred())
			}, 2*time.Second, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar), ar)).To(Succeed())
				ready := meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(ready.Message).To(ContainSubstring("deleted"))
			}, timeout, interval).Should(Succeed())
		})

		It("recreates a deleted scoped Approval for a new automatic request", func() {
			// Only the request the Approval was issued for does not recreate it; a
			// request for a new intent has never been bound and creates it again.
			// Deleting an Approval needs operator or admin rights.
			gate := newScopedGate("scoped-recreated-src")
			ar1 := newScopedRequest(gate.src, "scoped-recreated-a", &gate.target, gate.approvalKey, &requester, &decider,
				approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, autoGrant())
			Expect(k8sClient.Create(ctx, ar1)).To(Succeed())
			expectGrantedFor(gate.key, ar1)

			By("Revoking and deleting the Approval, then replacing the request")
			revokeApproval(gate.key)
			deleteApproval(gate.key)
			deleteRequest(ar1)
			ar2 := newScopedRequest(gate.src, "scoped-recreated-b", &gate.target, gate.approvalKey, &requester, &decider,
				approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, autoGrant())
			Expect(k8sClient.Create(ctx, ar2)).To(Succeed())

			By("Verifying the new request creates a granted Approval")
			expectGrantedFor(gate.key, ar2)
		})

		It("keeps an Approval that expired while suspended revoked once the expiry is reconciled", func() {
			gate := newScopedGate("scoped-expired-src")
			ar1 := newScopedRequest(gate.src, "scoped-expired-a", &gate.target, gate.approvalKey, &requester, &decider,
				approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, autoGrant())
			Expect(k8sClient.Create(ctx, ar1)).To(Succeed())
			expectGrantedFor(gate.key, ar1)

			By("Suspending the Approval, then expiring it as the expiration controller does")
			suspendedAt := metav1.NewTime(time.Now().Add(-time.Minute))
			setScopedApprovalState(gate.key, approvalv1.ApprovalStateSuspended,
				&approvalv1.Decision{Name: "Alice", Email: "alice@decider.com", Comment: "suspended", Timestamp: &suspendedAt, ResultingState: approvalv1.ApprovalStateSuspended})
			expectLastState(gate.key, approvalv1.ApprovalStateSuspended)
			expiredAt := metav1.NewTime(time.Now().Add(-30 * time.Second))
			setScopedApprovalState(gate.key, approvalv1.ApprovalStateExpired,
				&approvalv1.Decision{Name: approvalv1.SystemDecisionName, Comment: "expired", Timestamp: &expiredAt, ResultingState: approvalv1.ApprovalStateExpired})
			// The approval controller has reconciled the expiry: LastState no longer
			// shows the suspension.
			expectLastState(gate.key, approvalv1.ApprovalStateExpired)

			By("Replacing the request with a new automatic one")
			deleteRequest(ar1)
			ar2 := newScopedRequest(gate.src, "scoped-expired-b", &gate.target, gate.approvalKey, &requester, &decider,
				approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, autoGrant())
			Expect(k8sClient.Create(ctx, ar2)).To(Succeed())

			By("Keeping the expired Approval and reporting the revocation on the new request")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar2), ar2)).To(Succeed())
				ready := meta.FindStatusCondition(ar2.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.Reason).To(Equal("Revoked"))
			}, timeout, interval).Should(Succeed())
			Consistently(func(g Gomega) {
				a := &approvalv1.Approval{}
				g.Expect(k8sClient.Get(ctx, gate.key, a)).To(Succeed())
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateExpired))
				g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar1.UID))
			}, 2*time.Second, interval).Should(Succeed())
		})

		It("keeps an Approval that expired after a suspension without a decision revoked", func() {
			gate := newScopedGate("scoped-silent-expired-src")
			ar1 := newScopedRequest(gate.src, "scoped-silent-expired-a", &gate.target, gate.approvalKey, &requester, &decider,
				approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, autoGrant())
			Expect(k8sClient.Create(ctx, ar1)).To(Succeed())
			expectGrantedFor(gate.key, ar1)

			By("Suspending the Approval without a decision, then letting the expiration controller expire it")
			setScopedApprovalState(gate.key, approvalv1.ApprovalStateSuspended, nil)
			expectLastState(gate.key, approvalv1.ApprovalStateSuspended)
			Eventually(func(g Gomega) {
				expiration := &approvalv1.ApprovalExpiration{}
				g.Expect(k8sClient.Get(ctx, gate.key, expiration)).To(Succeed())
				expiration.Spec.Expiration = metav1.NewTime(time.Now().Add(-time.Minute))
				g.Expect(k8sClient.Update(ctx, expiration)).To(Succeed())
			}, timeout, interval).Should(Succeed())
			expectLastState(gate.key, approvalv1.ApprovalStateExpired)

			By("Replacing the request with a new automatic one")
			deleteRequest(ar1)
			ar2 := newScopedRequest(gate.src, "scoped-silent-expired-b", &gate.target, gate.approvalKey, &requester, &decider,
				approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, autoGrant())
			Expect(k8sClient.Create(ctx, ar2)).To(Succeed())

			By("Keeping the expired Approval and reporting the revocation on the new request")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar2), ar2)).To(Succeed())
				ready := meta.FindStatusCondition(ar2.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.Reason).To(Equal("Revoked"))
			}, timeout, interval).Should(Succeed())
			Consistently(func(g Gomega) {
				a := &approvalv1.Approval{}
				g.Expect(k8sClient.Get(ctx, gate.key, a)).To(Succeed())
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateExpired))
				g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar1.UID))
			}, 2*time.Second, interval).Should(Succeed())
		})

		It("keeps a revocation made without a decision when an older grant arrives", func() {
			gate := newScopedGate("scoped-decisionless-src")
			grantedAt := metav1.NewTime(time.Now().Add(-3 * time.Hour))
			ar1 := newScopedRequest(gate.src, "scoped-decisionless-a", &gate.target, gate.approvalKey, &requester, &decider,
				approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted,
				approvalv1.Decision{Name: "Alice", Email: "alice@decider.com", Timestamp: &grantedAt, ResultingState: approvalv1.ApprovalStateGranted})
			Expect(k8sClient.Create(ctx, ar1)).To(Succeed())
			expectGrantedFor(gate.key, ar1)

			By("Revoking two hours ago and lifting it with a person's grant ninety minutes ago")
			firstRevokedAt := metav1.NewTime(time.Now().Add(-2 * time.Hour))
			setScopedApprovalState(gate.key, approvalv1.ApprovalStateRejected,
				&approvalv1.Decision{Name: "Alice", Email: "alice@decider.com", Comment: "first revocation", Timestamp: &firstRevokedAt, ResultingState: approvalv1.ApprovalStateRejected})
			deleteRequest(ar1)
			liftedAt := metav1.NewTime(time.Now().Add(-90 * time.Minute))
			ar2 := newScopedRequest(gate.src, "scoped-decisionless-b", &gate.target, gate.approvalKey, &requester, &decider,
				approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted,
				approvalv1.Decision{Name: "Bob", Email: "bob@decider.com", Timestamp: &liftedAt, ResultingState: approvalv1.ApprovalStateGranted})
			Expect(k8sClient.Create(ctx, ar2)).To(Succeed())
			expectGrantedFor(gate.key, ar2)

			By("Suspending the Approval now without recording a decision")
			setScopedApprovalState(gate.key, approvalv1.ApprovalStateSuspended, nil)
			deleteRequest(ar2)

			By("Creating a request granted an hour ago, after the first revocation but before the suspension")
			staleAt := metav1.NewTime(time.Now().Add(-time.Hour))
			ar3 := newScopedRequest(gate.src, "scoped-decisionless-c", &gate.target, gate.approvalKey, &requester, &decider,
				approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted,
				approvalv1.Decision{Name: "Carol", Email: "carol@decider.com", Timestamp: &staleAt, ResultingState: approvalv1.ApprovalStateGranted})
			Expect(k8sClient.Create(ctx, ar3)).To(Succeed())

			By("Keeping the suspension")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar3), ar3)).To(Succeed())
				ready := meta.FindStatusCondition(ar3.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.Reason).To(Equal("Revoked"))
			}, timeout, interval).Should(Succeed())
			Consistently(func(g Gomega) {
				a := &approvalv1.Approval{}
				g.Expect(k8sClient.Get(ctx, gate.key, a)).To(Succeed())
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateSuspended))
				g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar2.UID))
			}, 2*time.Second, interval).Should(Succeed())
		})

		It("binds a re-granted scoped Approval to the current request without waiting for the requeue", func() {
			By("Creating a source resource")
			src := test.NewObject("scoped-regrant-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			target := *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme())
			approvalName, err := approvalv1.ScopedApprovalName(target, approvalv1.ApprovalKeyProvider)
			Expect(err).NotTo(HaveOccurred())
			approvalKey := client.ObjectKey{Name: approvalName, Namespace: testNamespace}
			autoDecision := approvalv1.Decision{
				Name:           approvalv1.SystemDecisionName,
				Comment:        approvalv1.AutoApprovedComment,
				ResultingState: approvalv1.ApprovalStateGranted,
			}

			By("Auto-granting the first request")
			ar1 := newScopedRequest(src, "scoped-regrant-a", &target, approvalv1.ApprovalKeyProvider, &requester, &decider,
				approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, autoDecision)
			Expect(k8sClient.Create(ctx, ar1)).To(Succeed())
			Eventually(func(g Gomega) {
				a := &approvalv1.Approval{}
				g.Expect(k8sClient.Get(ctx, approvalKey, a)).To(Succeed())
				g.Expect(a.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar1.UID))
			}, timeout, interval).Should(Succeed())

			By("Revoking the Approval and auto-granting a newer request")
			revokeApproval(approvalKey)
			deleteRequest(ar1)
			ar2 := newScopedRequest(src, "scoped-regrant-b", &target, approvalv1.ApprovalKeyProvider, &requester, &decider,
				approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, autoDecision)
			Expect(k8sClient.Create(ctx, ar2)).To(Succeed())

			By("Keeping the revocation and reporting it on the newer request")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar2), ar2)).To(Succeed())
				ready := meta.FindStatusCondition(ar2.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.Reason).To(Equal("Revoked"))

				a := &approvalv1.Approval{}
				g.Expect(k8sClient.Get(ctx, approvalKey, a)).To(Succeed())
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateRejected))
				g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar1.UID))
			}, timeout, interval).Should(Succeed())

			By("Granting the Approval again")
			Eventually(func(g Gomega) {
				a := &approvalv1.Approval{}
				g.Expect(k8sClient.Get(ctx, approvalKey, a)).To(Succeed())
				a.Spec.State = approvalv1.ApprovalStateGranted
				a.AppendDecision(approvalv1.Decision{Name: "Alice", Email: "alice@decider.com", ResultingState: approvalv1.ApprovalStateGranted})
				g.Expect(k8sClient.Update(ctx, a)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying the Approval is bound to the newer request within seconds")
			Eventually(func(g Gomega) {
				a := &approvalv1.Approval{}
				g.Expect(k8sClient.Get(ctx, approvalKey, a)).To(Succeed())
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				g.Expect(a.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar2.UID))

				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar2), ar2)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(ar2.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("scoped idempotency and liveness", func() {
		It("rebinds approval when a keyed AR is recreated with the same name but new UID", func() {
			By("Creating a source resource")
			src := test.NewObject("scoped-idem-uid-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			target := *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme())
			approvalName, err := approvalv1.ScopedApprovalName(target, approvalv1.ApprovalKeyProvider)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a keyed auto-approved ApprovalRequest")
			ar := approvalv1.NewApprovalRequest(src, "scoped-idem-uid")
			ar.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:      target,
				Requester:   requester,
				Decider:     decider,
				Strategy:    approvalv1.ApprovalStrategyAuto,
				State:       approvalv1.ApprovalStateGranted,
				Action:      "subscribe",
				ApprovalKey: approvalv1.ApprovalKeyProvider,
				Decisions: []approvalv1.Decision{
					{
						Name:           approvalv1.SystemDecisionName,
						Comment:        approvalv1.AutoApprovedComment,
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			}
			arName := ar.GetName()

			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			var firstUID ktypes.UID
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name: arName, Namespace: testNamespace,
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ar.Status.Approval.Name).To(Equal(approvalName))

				a := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name: approvalName, Namespace: testNamespace,
				}, a)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(a.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar.UID))
				firstUID = ar.UID
			}, timeout, interval).Should(Succeed())

			By("Deleting the original AR")
			Expect(k8sClient.Delete(ctx, ar)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name: arName, Namespace: testNamespace,
				}, &approvalv1.ApprovalRequest{})
				return err != nil
			}, timeout, interval).Should(BeTrue())

			By("Recreating an AR with the same name (same hash input) but new UID")
			ar2 := approvalv1.NewApprovalRequest(src, "scoped-idem-uid")
			ar2.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar2.Spec = approvalv1.ApprovalRequestSpec{
				Target:      target,
				Requester:   requester,
				Decider:     decider,
				Strategy:    approvalv1.ApprovalStrategyAuto,
				State:       approvalv1.ApprovalStateGranted,
				Action:      "subscribe",
				ApprovalKey: approvalv1.ApprovalKeyProvider,
				Decisions: []approvalv1.Decision{
					{
						Name:           approvalv1.SystemDecisionName,
						Comment:        approvalv1.AutoApprovedComment,
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			}
			Expect(k8sClient.Create(ctx, ar2)).To(Succeed())

			By("Verifying the Approval gets the new UID")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name: ar2.GetName(), Namespace: testNamespace,
				}, ar2)
				g.Expect(err).NotTo(HaveOccurred())

				a := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name: approvalName, Namespace: testNamespace,
				}, a)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(a.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(a.Spec.ApprovedRequest.UID).NotTo(Equal(firstUID))
				g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar2.UID))
			}, timeout, interval).Should(Succeed())
		})

		It("does not overwrite approval when source AR transitions away from granted", func() {
			By("Creating a source resource")
			src := test.NewObject("scoped-stale-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			target := *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme())
			approvalName, err := approvalv1.ScopedApprovalName(target, approvalv1.ApprovalKeyProvider)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a keyed Simple ApprovalRequest in Granted state")
			ar := approvalv1.NewApprovalRequest(src, "scoped-stale")
			ar.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:      target,
				Requester:   requester,
				Decider:     decider,
				Strategy:    approvalv1.ApprovalStrategySimple,
				State:       approvalv1.ApprovalStateGranted,
				Action:      "subscribe",
				ApprovalKey: approvalv1.ApprovalKeyProvider,
				Decisions: []approvalv1.Decision{
					{Name: "Alice", Email: "alice@example.com", Comment: "Approved", ResultingState: approvalv1.ApprovalStateGranted},
				},
			}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			By("Waiting for the Approval to be created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name: ar.GetName(), Namespace: testNamespace,
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ar.Status.Approval.Name).To(Equal(approvalName))

				a := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name: approvalName, Namespace: testNamespace,
				}, a)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				g.Expect(a.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(a.Spec.ApprovedRequest.Name).To(Equal(ar.GetName()))
			}, timeout, interval).Should(Succeed())

			By("Recording the Approval's resourceVersion before the state change")
			approvalBefore := &approvalv1.Approval{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name: approvalName, Namespace: testNamespace,
			}, approvalBefore)).To(Succeed())
			rvBefore := approvalBefore.ResourceVersion

			By("Transitioning the AR to Rejected")
			ar.Spec.State = approvalv1.ApprovalStateRejected
			ar.Spec.Decisions = append(ar.Spec.Decisions, approvalv1.Decision{
				Name: "Bob", Comment: "Revoked", ResultingState: approvalv1.ApprovalStateRejected,
			})
			Expect(k8sClient.Update(ctx, ar)).To(Succeed())

			By("Waiting for the AR to be reconciled with Rejected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name: ar.GetName(), Namespace: testNamespace,
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())

				approvedCondition := meta.FindStatusCondition(ar.Status.Conditions, "Approved")
				g.Expect(approvedCondition).NotTo(BeNil())
				g.Expect(approvedCondition.Reason).To(Equal("Rejected"))
			}, timeout, interval).Should(Succeed())

			By("Verifying the Approval was not overwritten — still has the original grant")
			approvalAfter := &approvalv1.Approval{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name: approvalName, Namespace: testNamespace,
			}, approvalAfter)).To(Succeed())
			Expect(approvalAfter.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
			Expect(approvalAfter.ResourceVersion).To(Equal(rvBefore))
		})

		// Regression: verifies that when a keyed AR is replaced (different intent
		// hash → different AR name), the Approval rebinds to the new AR. This
		// exercises the uncached-reader liveness check — a stale cache returning
		// a deleted AR1 would cause the mutate function to bind to a ghost request.
		It("rebinds approval to the second keyed AR when intent hash changes", func() {
			By("Creating a source resource")
			src := test.NewObject("scoped-rebind-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			target := *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme())
			approvalName, err := approvalv1.ScopedApprovalName(target, approvalv1.ApprovalKeyProvider)
			Expect(err).NotTo(HaveOccurred())

			By("Creating the first keyed auto-approved AR (intent A)")
			ar1 := approvalv1.NewApprovalRequest(src, "scoped-rebind-a")
			ar1.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar1.Spec = approvalv1.ApprovalRequestSpec{
				Target:      target,
				Requester:   requester,
				Decider:     decider,
				Strategy:    approvalv1.ApprovalStrategyAuto,
				State:       approvalv1.ApprovalStateGranted,
				Action:      "subscribe",
				ApprovalKey: approvalv1.ApprovalKeyProvider,
				Decisions: []approvalv1.Decision{
					{
						Name:           approvalv1.SystemDecisionName,
						Comment:        approvalv1.AutoApprovedComment,
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			}
			Expect(k8sClient.Create(ctx, ar1)).To(Succeed())

			var ar1UID ktypes.UID
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name: ar1.GetName(), Namespace: testNamespace,
				}, ar1)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ar1.Status.Approval.Name).To(Equal(approvalName))

				a := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name: approvalName, Namespace: testNamespace,
				}, a)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(a.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(a.Spec.ApprovedRequest.Name).To(Equal(ar1.GetName()))
				ar1UID = ar1.UID
			}, timeout, interval).Should(Succeed())

			By("Deleting AR1 to simulate intent change")
			Expect(k8sClient.Delete(ctx, ar1)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name: ar1.GetName(), Namespace: testNamespace,
				}, &approvalv1.ApprovalRequest{})
				return err != nil
			}, timeout, interval).Should(BeTrue())

			By("Creating a second keyed AR (intent B) for the same gate")
			ar2 := approvalv1.NewApprovalRequest(src, "scoped-rebind-b")
			ar2.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar2.Spec = approvalv1.ApprovalRequestSpec{
				Target:      target,
				Requester:   requester,
				Decider:     decider,
				Strategy:    approvalv1.ApprovalStrategyAuto,
				State:       approvalv1.ApprovalStateGranted,
				Action:      "subscribe",
				ApprovalKey: approvalv1.ApprovalKeyProvider,
				Decisions: []approvalv1.Decision{
					{
						Name:           approvalv1.SystemDecisionName,
						Comment:        approvalv1.AutoApprovedComment,
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			}
			Expect(k8sClient.Create(ctx, ar2)).To(Succeed())

			By("Verifying the Approval rebinds to AR2")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name: ar2.GetName(), Namespace: testNamespace,
				}, ar2)
				g.Expect(err).NotTo(HaveOccurred())

				a := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name: approvalName, Namespace: testNamespace,
				}, a)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(a.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(a.Spec.ApprovedRequest.Name).To(Equal(ar2.GetName()))
				g.Expect(a.Spec.ApprovedRequest.UID).NotTo(Equal(ar1UID))
				g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar2.UID))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("legacy naming", func() {
		It("produces the legacy Approval name and preserves the full target reference for auto-approved requests", func() {
			By("Creating a unique source resource for this characterization test")
			src := test.NewObject("legacy-naming-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			By("Building a complete auto-approved ApprovalRequest from the created source")
			ar := approvalv1.NewApprovalRequest(src, src.Spec)
			ar.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:    *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme()),
				Requester: requester,
				Decider:   decider,
				Strategy:  approvalv1.ApprovalStrategyAuto,
				State:     approvalv1.ApprovalStateGranted,
				Action:    "subscribe",
				Decisions: []approvalv1.Decision{
					{
						Name:           approvalv1.SystemDecisionName,
						Comment:        approvalv1.AutoApprovedComment,
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			}

			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			legacyApprovalName := "testresource--legacy-naming-src"

			Eventually(func(g Gomega) {
				By("Verifying the AR status references the legacy-named Approval")
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name: ar.GetName(), Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ar.Status.Approval.Name).To(Equal(legacyApprovalName))

				By("Verifying the Approval exists with the legacy name")
				a := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name: legacyApprovalName, Namespace: ar.GetNamespace(),
				}, a)
				g.Expect(err).NotTo(HaveOccurred())

				By("Verifying the full target reference on the Approval")
				g.Expect(a.Spec.Target.Kind).To(Equal("TestResource"))
				g.Expect(a.Spec.Target.APIVersion).To(Equal("testgroup.cp.ei.telekom.de/v1"))
				g.Expect(a.Spec.Target.Name).To(Equal("legacy-naming-src"))
				g.Expect(a.Spec.Target.Namespace).To(Equal(testNamespace))
				g.Expect(a.Spec.Target.UID).To(Equal(src.UID))

				By("Verifying the controller owner reference points to the source resource")
				g.Expect(a.ObjectMeta.OwnerReferences).To(HaveLen(1))
				g.Expect(a.ObjectMeta.OwnerReferences[0].APIVersion).To(Equal("testgroup.cp.ei.telekom.de/v1"))
				g.Expect(a.ObjectMeta.OwnerReferences[0].Kind).To(Equal("TestResource"))
				g.Expect(a.ObjectMeta.OwnerReferences[0].Name).To(Equal("legacy-naming-src"))
				g.Expect(a.ObjectMeta.OwnerReferences[0].UID).To(Equal(src.UID))

				By("Verifying auto-approved decision was carried to the Approval")
				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				g.Expect(a.Spec.Strategy).To(Equal(approvalv1.ApprovalStrategyAuto))
				g.Expect(a.Spec.Decisions).To(HaveLen(1))
				g.Expect(a.Spec.Decisions[0].Name).To(Equal(approvalv1.SystemDecisionName))
				g.Expect(a.Spec.Decisions[0].Comment).To(Equal(approvalv1.AutoApprovedComment))
				g.Expect(a.Spec.Decisions[0].ResultingState).To(Equal(approvalv1.ApprovalStateGranted))

				By("Verifying ApprovedRequest reference")
				g.Expect(a.Spec.ApprovedRequest).NotTo(BeNil())
				g.Expect(a.Spec.ApprovedRequest.Name).To(Equal(ar.GetName()))
				g.Expect(a.Spec.ApprovedRequest.Namespace).To(Equal(ar.GetNamespace()))
			}, timeout, interval).Should(Succeed())
		})

		It("produces legacy notification names for unscoped requests", func() {
			By("Creating a source resource")
			src := test.NewObject("legacy-notif-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			ar := approvalv1.NewApprovalRequest(src, "legacy-notif")
			ar.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			ar.Spec = approvalv1.ApprovalRequestSpec{
				Target:    *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme()),
				Requester: requester,
				Decider:   decider,
				Strategy:  approvalv1.ApprovalStrategyAuto,
				State:     approvalv1.ApprovalStateGranted,
				Action:    "subscribe",
				Decisions: []approvalv1.Decision{
					{
						Name:           approvalv1.SystemDecisionName,
						Comment:        approvalv1.AutoApprovedComment,
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			}

			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name: ar.GetName(), Namespace: ar.GetNamespace(),
				}, ar)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ar.Status.NotificationRefs).NotTo(BeEmpty())

				By("Verifying unscoped notification name does NOT have an-v1- prefix")
				notif := &notificationv1.Notification{}
				err = k8sClient.Get(ctx, ar.Status.NotificationRefs[0].K8s(), notif)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(notif.Name).NotTo(HavePrefix("an-v1-"))
				g.Expect(notif.Spec.Purpose).To(ContainSubstring("approvalrequest--subscribe--created--decider"))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Spectre configuration", func() {
		It("isolates notifications for the provider and consumer keys on the same target with same action", func() {
			By("Creating a single source resource")
			src := test.NewObject("spectre-cfg-src", testNamespace)
			src.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

			target := *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme())

			nameP, err := approvalv1.ScopedApprovalName(target, approvalv1.ApprovalKeyProvider)
			Expect(err).NotTo(HaveOccurred())
			nameC, err := approvalv1.ScopedApprovalName(target, approvalv1.ApprovalKeyConsumer)
			Expect(err).NotTo(HaveOccurred())
			Expect(nameP).NotTo(Equal(nameC))

			By("Creating the provider keyed ApprovalRequest")
			arP := approvalv1.NewApprovalRequest(src, "spectre-provider")
			arP.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			arP.Spec = approvalv1.ApprovalRequestSpec{
				Target:      target,
				Requester:   requester,
				Decider:     decider,
				Strategy:    approvalv1.ApprovalStrategyAuto,
				State:       approvalv1.ApprovalStateGranted,
				Action:      "subscribe",
				ApprovalKey: approvalv1.ApprovalKeyProvider,
				Decisions: []approvalv1.Decision{
					{
						Name:           approvalv1.SystemDecisionName,
						Comment:        approvalv1.AutoApprovedComment,
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			}
			Expect(k8sClient.Create(ctx, arP)).To(Succeed())

			By("Creating the consumer keyed ApprovalRequest on the SAME target with SAME action")
			arC := approvalv1.NewApprovalRequest(src, "spectre-consumer")
			arC.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})
			arC.Spec = approvalv1.ApprovalRequestSpec{
				Target:      target,
				Requester:   requester,
				Decider:     decider,
				Strategy:    approvalv1.ApprovalStrategyAuto,
				State:       approvalv1.ApprovalStateGranted,
				Action:      "subscribe",
				ApprovalKey: approvalv1.ApprovalKeyConsumer,
				Decisions: []approvalv1.Decision{
					{
						Name:           approvalv1.SystemDecisionName,
						Comment:        approvalv1.AutoApprovedComment,
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			}
			Expect(k8sClient.Create(ctx, arC)).To(Succeed())

			Eventually(func(g Gomega) {
				aP := &approvalv1.Approval{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: nameP, Namespace: testNamespace}, aP)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(aP.Spec.ApprovalKey).To(Equal(approvalv1.ApprovalKeyProvider))
				g.Expect(aP.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))

				aC := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{Name: nameC, Namespace: testNamespace}, aC)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(aC.Spec.ApprovalKey).To(Equal(approvalv1.ApprovalKeyConsumer))
				g.Expect(aC.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))

				By("Verifying both ARs have notification refs")
				err = k8sClient.Get(ctx, client.ObjectKey{Name: arP.GetName(), Namespace: testNamespace}, arP)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(arP.Status.NotificationRefs).NotTo(BeEmpty())

				err = k8sClient.Get(ctx, client.ObjectKey{Name: arC.GetName(), Namespace: testNamespace}, arC)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(arC.Status.NotificationRefs).NotTo(BeEmpty())

				By("Verifying notifications have an-v1- prefix and different names")
				notifP := &notificationv1.Notification{}
				err = k8sClient.Get(ctx, arP.Status.NotificationRefs[0].K8s(), notifP)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(notifP.Name).To(HavePrefix("an-v1-"))

				notifC := &notificationv1.Notification{}
				err = k8sClient.Get(ctx, arC.Status.NotificationRefs[0].K8s(), notifC)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(notifC.Name).To(HavePrefix("an-v1-"))

				g.Expect(notifP.Name).NotTo(Equal(notifC.Name))
			}, timeout, interval).Should(Succeed())
		})
	})
})

// newScopedRequest returns a keyed ApprovalRequest for target in the given state.
func newScopedRequest(src *test.TestResource, hash string, target *ctypes.TypedObjectRef, key string,
	requester *approvalv1.Requester, decider *approvalv1.Decider,
	strategy approvalv1.ApprovalStrategy, state approvalv1.ApprovalState, decisions ...approvalv1.Decision,
) *approvalv1.ApprovalRequest {
	ar := approvalv1.NewApprovalRequest(src, hash)
	ar.SetLabels(map[string]string{
		config.EnvironmentLabelKey: testEnvironment,
	})
	ar.Spec = approvalv1.ApprovalRequestSpec{
		Target:      *target,
		Requester:   *requester,
		Decider:     *decider,
		Strategy:    strategy,
		State:       state,
		Action:      "subscribe",
		ApprovalKey: key,
		Decisions:   decisions,
	}
	return ar
}

// revokeApproval rejects the Approval at key, as its decider does. This envtest
// runs no webhook, so the decision carries the timestamp the mutating webhook
// would set; it is a minute in the past because timestamps have second
// precision, so a grant made now is strictly later.
func revokeApproval(key client.ObjectKey) {
	revokedAt := metav1.NewTime(time.Now().Add(-time.Minute))
	Eventually(func(g Gomega) {
		a := &approvalv1.Approval{}
		g.Expect(k8sClient.Get(ctx, key, a)).To(Succeed())
		a.Spec.State = approvalv1.ApprovalStateRejected
		a.AppendDecision(approvalv1.Decision{Name: "Alice", Email: "alice@decider.com", Comment: "Access revoked", Timestamp: &revokedAt, ResultingState: approvalv1.ApprovalStateRejected})
		g.Expect(k8sClient.Update(ctx, a)).To(Succeed())
	}, timeout, interval).Should(Succeed())
}

// deleteRequest deletes ar and waits until it is gone.
func deleteRequest(ar *approvalv1.ApprovalRequest) {
	Expect(k8sClient.Delete(ctx, ar)).To(Succeed())
	Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ar), &approvalv1.ApprovalRequest{})
		g.Expect(err).To(HaveOccurred())
	}, timeout, interval).Should(Succeed())
}

// scopedGate is a source resource and the scoped Approval of one of its gates.
type scopedGate struct {
	src         *test.TestResource
	target      ctypes.TypedObjectRef
	approvalKey string
	key         client.ObjectKey
}

// newScopedGate creates the source resource name and returns its provider gate.
func newScopedGate(name string) scopedGate {
	approvalKey := approvalv1.ApprovalKeyProvider
	src := test.NewObject(name, testNamespace)
	src.SetLabels(map[string]string{
		config.EnvironmentLabelKey: testEnvironment,
	})
	Expect(k8sClient.Create(ctx, src)).To(Succeed())
	DeferCleanup(func() { _ = k8sClient.Delete(ctx, src) })

	target := *ctypes.TypedObjectRefFromObject(src, k8sClient.Scheme())
	approvalName, err := approvalv1.ScopedApprovalName(target, approvalKey)
	Expect(err).NotTo(HaveOccurred())
	return scopedGate{src: src, target: target, approvalKey: approvalKey, key: client.ObjectKey{Name: approvalName, Namespace: testNamespace}}
}

// autoGrant is the decision an automatic grant records.
func autoGrant() approvalv1.Decision {
	return approvalv1.Decision{
		Name:           approvalv1.SystemDecisionName,
		Comment:        approvalv1.AutoApprovedComment,
		ResultingState: approvalv1.ApprovalStateGranted,
	}
}

// expectGrantedFor waits until the Approval at key is Granted and bound to ar.
func expectGrantedFor(key client.ObjectKey, ar *approvalv1.ApprovalRequest) {
	Eventually(func(g Gomega) {
		a := &approvalv1.Approval{}
		g.Expect(k8sClient.Get(ctx, key, a)).To(Succeed())
		g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
		g.Expect(a.Spec.ApprovedRequest).NotTo(BeNil())
		g.Expect(a.Spec.ApprovedRequest.UID).To(Equal(ar.UID))
	}, timeout, interval).Should(Succeed())
}

// expectLastState waits until the approval controller has reconciled the
// Approval at key in state.
func expectLastState(key client.ObjectKey, state approvalv1.ApprovalState) {
	Eventually(func(g Gomega) {
		a := &approvalv1.Approval{}
		g.Expect(k8sClient.Get(ctx, key, a)).To(Succeed())
		g.Expect(a.Status.LastState).To(Equal(state))
	}, timeout, interval).Should(Succeed())
}

// setScopedApprovalState sets the Approval at key to state, recording decision
// when it is not nil.
func setScopedApprovalState(key client.ObjectKey, state approvalv1.ApprovalState, decision *approvalv1.Decision) {
	Eventually(func(g Gomega) {
		a := &approvalv1.Approval{}
		g.Expect(k8sClient.Get(ctx, key, a)).To(Succeed())
		a.Spec.State = state
		if decision != nil {
			a.AppendDecision(*decision)
		}
		g.Expect(k8sClient.Update(ctx, a)).To(Succeed())
	}, timeout, interval).Should(Succeed())
}

// deleteApproval deletes the Approval at key and waits until it is gone; the
// approval controller's finalizer delays the removal.
func deleteApproval(key client.ObjectKey) {
	Expect(k8sClient.Delete(ctx, &approvalv1.Approval{
		ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
	})).To(Succeed())
	Eventually(func() bool {
		return apierrors.IsNotFound(k8sClient.Get(ctx, key, &approvalv1.Approval{}))
	}, timeout, interval).Should(BeTrue())
}
