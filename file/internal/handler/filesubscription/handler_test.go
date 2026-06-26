// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filesubscription

import (
	"context"
	"errors"
	"fmt"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8smeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	testNamespace        = "test"
	testFileTypeName     = "test-filetype"
	testExposureName     = "test-exposure"
	testSubscriptionName = "test-subscription"
	testZoneName         = "test-zone"
)

// buildScheme builds a runtime.Scheme with all types used by the handler.
func buildScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = filev1.AddToScheme(s)
	_ = approvalv1.AddToScheme(s)
	_ = sftpv1.AddToScheme(s)
	return s
}

func newTestContext() (context.Context, *fake.MockJanitorClient) {
	mockClient := fake.NewMockJanitorClient(GinkgoT())
	ctx := cclient.WithClient(context.Background(), mockClient)
	return ctx, mockClient
}

func testFileType() *filev1.FileType {
	ref := types.ObjectRef{Name: testExposureName, Namespace: testNamespace}
	return &filev1.FileType{
		TypeMeta: metav1.TypeMeta{
			APIVersion: filev1.GroupVersion.String(),
			Kind:       "FileType",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testFileTypeName,
			Namespace: testNamespace,
		},
		Status: filev1.FileTypeStatus{
			FileExposureRef: &ref,
		},
	}
}

func testFileExposure() *filev1.FileExposure {
	return &filev1.FileExposure{
		TypeMeta: metav1.TypeMeta{
			APIVersion: filev1.GroupVersion.String(),
			Kind:       "FileExposure",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testExposureName,
			Namespace: testNamespace,
		},
		Spec: filev1.FileExposureSpec{
			FileType:   testFileTypeName,
			Zone:       &types.ObjectRef{Name: testZoneName, Namespace: testNamespace},
			Visibility: filev1.VisibilityEnterprise,
			Approval: filev1.Approval{
				Strategy: filev1.ApprovalStrategyAuto,
			},
		},
	}
}

func testSubscription() *filev1.FileSubscription {
	return &filev1.FileSubscription{
		TypeMeta: metav1.TypeMeta{
			APIVersion: filev1.GroupVersion.String(),
			Kind:       "FileSubscription",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       testSubscriptionName,
			Namespace:  testNamespace,
			Generation: 1,
			UID:        "sub-uid-1234",
		},
		Spec: filev1.FileSubscriptionSpec{
			FileType: testFileTypeName,
			Zone:     &types.ObjectRef{Name: testZoneName, Namespace: testNamespace},
		},
	}
}

var _ = Describe("FileSubscriptionHandler", func() {
	var handler *FileSubscriptionHandler

	BeforeEach(func() {
		handler = &FileSubscriptionHandler{}
	})

	Describe("CreateOrUpdate", func() {
		It("blocks when the FileType is not found", func() {
			sub := testSubscription()
			ctx, mockClient := newTestContext()

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testFileTypeName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileType")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: filev1.GroupVersion.Group, Resource: "filetypes"}, testFileTypeName)).
				Once()

			err := handler.CreateOrUpdate(ctx, sub)

			var blocked ctrlerrors.BlockedError
			Expect(errors.As(err, &blocked)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("FileType"))
		})

		It("blocks when FileType has no active FileExposure", func() {
			sub := testSubscription()
			ctx, mockClient := newTestContext()

			ftNoExposure := testFileType()
			ftNoExposure.Status.FileExposureRef = nil
			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testFileTypeName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileType")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileType) = *ftNoExposure
				}).
				Return(nil).Once()

			err := handler.CreateOrUpdate(ctx, sub)

			Expect(err).NotTo(HaveOccurred())
			Expect(k8smeta.IsStatusConditionFalse(sub.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			ready := k8smeta.FindStatusCondition(sub.Status.Conditions, condition.ConditionTypeReady)
			Expect(ready.Reason).To(Equal("FileExposureNotFound"))
		})

		It("blocks when the active FileExposure is not found", func() {
			sub := testSubscription()
			ctx, mockClient := newTestContext()

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testFileTypeName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileType")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileType) = *testFileType()
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testExposureName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileExposure")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: filev1.GroupVersion.Group, Resource: "fileexposures"}, testExposureName)).
				Once()

			err := handler.CreateOrUpdate(ctx, sub)

			Expect(err).NotTo(HaveOccurred())
			Expect(k8smeta.IsStatusConditionFalse(sub.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		})

		It("blocks when visibility constraints prevent subscription", func() {
			sub := testSubscription()
			sub.Spec.Zone = &types.ObjectRef{Name: "zone-b", Namespace: testNamespace}
			ctx, mockClient := newTestContext()

			exposureZoneA := testFileExposure()
			exposureZoneA.Spec.Visibility = filev1.VisibilityZone
			exposureZoneA.Spec.Zone = &types.ObjectRef{Name: "zone-a", Namespace: testNamespace}

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testFileTypeName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileType")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileType) = *testFileType()
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testExposureName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileExposure")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileExposure) = *exposureZoneA
				}).
				Return(nil).Once()

			err := handler.CreateOrUpdate(ctx, sub)

			var blocked ctrlerrors.BlockedError
			Expect(errors.As(err, &blocked)).To(BeTrue())
			Expect(k8smeta.IsStatusConditionFalse(sub.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		})

		It("waits for approval when approval is pending (Approval not yet created)", func() {
			sub := testSubscription()
			ctx, mockClient := newTestContext()
			testScheme := buildScheme()

			exposure := testFileExposure()

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testFileTypeName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileType")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileType) = *testFileType()
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testExposureName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileExposure")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileExposure) = *exposure
				}).
				Return(nil).Once()
			// Scheme is needed by the approval builder's setWithHash()
			mockClient.EXPECT().Scheme().Return(testScheme).Maybe()
			// Approval builder creates the ApprovalRequest
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
				Return(controllerutil.OperationResultCreated, nil).Once()
			// Approval builder cleans up old requests
			mockClient.EXPECT().
				Cleanup(mock.Anything, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
				Return(0, nil).Once()
			// Approval does not exist yet → Pending
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Approval")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: approvalv1.GroupVersion.Group, Resource: "approvals"}, "any")).
				Once()
			// On Pending: deleteSubscriberUser is called → Delete on User (tolerates NotFound)
			mockClient.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.User")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: sftpv1.GroupVersion.Group, Resource: "users"}, "any")).
				Once()

			err := handler.CreateOrUpdate(ctx, sub)

			Expect(err).NotTo(HaveOccurred())
			Expect(k8smeta.IsStatusConditionFalse(sub.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			ready := k8smeta.FindStatusCondition(sub.Status.Conditions, condition.ConditionTypeReady)
			Expect(ready.Reason).To(Equal("ApprovalPending"))
		})

		It("provisions subscriber User when approval is granted", func() {
			sub := testSubscription()
			ctx, mockClient := newTestContext()
			testScheme := buildScheme()

			exposure := testFileExposure()
			approval := &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "filesubscription--" + testSubscriptionName,
					Namespace: testNamespace,
				},
				Spec: approvalv1.ApprovalSpec{
					State: approvalv1.ApprovalStateGranted,
				},
			}

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testFileTypeName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileType")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileType) = *testFileType()
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testExposureName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileExposure")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileExposure) = *exposure
				}).
				Return(nil).Once()
			mockClient.EXPECT().Scheme().Return(testScheme).Maybe()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().
				Cleanup(mock.Anything, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
				Return(0, nil).Once()
			// Approval exists and is granted
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Approval")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*approvalv1.Approval) = *approval
				}).
				Return(nil).Once()
			// syncSubscriberUser → SyncSFTPUser → CreateOrUpdate for sftpv1.User
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.User"), mock.Anything).
				Return(controllerutil.OperationResultCreated, nil).Once()
			mockClient.EXPECT().AllReady().Return(true).Once()

			err := handler.CreateOrUpdate(ctx, sub)

			Expect(err).NotTo(HaveOccurred())
			Expect(k8smeta.IsStatusConditionTrue(sub.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		})

		It("returns error when active FileExposure Get fails with unexpected error", func() {
			sub := testSubscription()
			ctx, mockClient := newTestContext()

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testFileTypeName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileType")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileType) = *testFileType()
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testExposureName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileExposure")).
				Return(fmt.Errorf("api server error")).Once()

			err := handler.CreateOrUpdate(ctx, sub)

			Expect(err).To(MatchError(ContainSubstring("api server error")))
		})

		It("sets NotReady condition and deletes subscriber User when approval is denied", func() {
			sub := testSubscription()
			ctx, mockClient := newTestContext()
			testScheme := buildScheme()

			exposure := testFileExposure()
			deniedApproval := &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{Name: "any", Namespace: testNamespace},
				Spec:       approvalv1.ApprovalSpec{State: approvalv1.ApprovalStateRejected},
			}

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testFileTypeName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileType")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileType) = *testFileType()
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testExposureName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileExposure")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileExposure) = *exposure
				}).
				Return(nil).Once()
			mockClient.EXPECT().Scheme().Return(testScheme).Maybe()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().
				Cleanup(mock.Anything, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
				Return(0, nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Approval")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*approvalv1.Approval) = *deniedApproval
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.User")).
				Return(nil).Once()

			err := handler.CreateOrUpdate(ctx, sub)

			Expect(err).NotTo(HaveOccurred())
			ready := k8smeta.FindStatusCondition(sub.Status.Conditions, condition.ConditionTypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Reason).To(Equal("ApprovalDenied"))
		})

		It("returns error when subscriber User cleanup fails after approval denial", func() {
			sub := testSubscription()
			ctx, mockClient := newTestContext()
			testScheme := buildScheme()

			exposure := testFileExposure()
			deniedApproval := &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{Name: "any", Namespace: testNamespace},
				Spec:       approvalv1.ApprovalSpec{State: approvalv1.ApprovalStateRejected},
			}

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testFileTypeName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileType")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileType) = *testFileType()
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testExposureName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileExposure")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileExposure) = *exposure
				}).
				Return(nil).Once()
			mockClient.EXPECT().Scheme().Return(testScheme).Maybe()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().
				Cleanup(mock.Anything, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
				Return(0, nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Approval")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*approvalv1.Approval) = *deniedApproval
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.User")).
				Return(fmt.Errorf("delete failed")).Once()

			err := handler.CreateOrUpdate(ctx, sub)

			Expect(err).To(MatchError(ContainSubstring("delete failed")))
		})

		It("sets Processing condition when child resources are not yet ready after sync", func() {
			sub := testSubscription()
			ctx, mockClient := newTestContext()
			testScheme := buildScheme()

			exposure := testFileExposure()
			grantedApproval := &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{Name: "any", Namespace: testNamespace},
				Spec:       approvalv1.ApprovalSpec{State: approvalv1.ApprovalStateGranted},
			}

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testFileTypeName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileType")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileType) = *testFileType()
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testExposureName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileExposure")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileExposure) = *exposure
				}).
				Return(nil).Once()
			mockClient.EXPECT().Scheme().Return(testScheme).Maybe()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().
				Cleanup(mock.Anything, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
				Return(0, nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Approval")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*approvalv1.Approval) = *grantedApproval
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.User"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().AllReady().Return(false).Once()

			err := handler.CreateOrUpdate(ctx, sub)

			Expect(err).NotTo(HaveOccurred())
			Expect(k8smeta.IsStatusConditionTrue(sub.Status.Conditions, condition.ConditionTypeProcessing)).To(BeTrue())
		})

		It("returns error when syncSubscriberUser fails", func() {
			sub := testSubscription()
			ctx, mockClient := newTestContext()
			testScheme := buildScheme()

			exposure := testFileExposure()
			grantedApproval := &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{Name: "any", Namespace: testNamespace},
				Spec:       approvalv1.ApprovalSpec{State: approvalv1.ApprovalStateGranted},
			}

			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testFileTypeName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileType")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileType) = *testFileType()
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testExposureName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileExposure")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*filev1.FileExposure) = *exposure
				}).
				Return(nil).Once()
			mockClient.EXPECT().Scheme().Return(testScheme).Maybe()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().
				Cleanup(mock.Anything, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
				Return(0, nil).Once()
			mockClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1.Approval")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*approvalv1.Approval) = *grantedApproval
				}).
				Return(nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.User"), mock.Anything).
				Return(controllerutil.OperationResultNone, fmt.Errorf("sync failed")).Once()

			err := handler.CreateOrUpdate(ctx, sub)

			Expect(err).To(MatchError(ContainSubstring("sync failed")))
		})
	})

	Describe("Delete", func() {
		It("deletes subscriber SFTP User", func() {
			sub := testSubscription()
			ctx, mockClient := newTestContext()

			mockClient.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.User")).
				Return(nil).Once()

			err := handler.Delete(ctx, sub)

			Expect(err).NotTo(HaveOccurred())
		})

		It("tolerates NotFound when deleting subscriber SFTP User", func() {
			sub := testSubscription()
			ctx, mockClient := newTestContext()

			mockClient.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.User")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: sftpv1.GroupVersion.Group, Resource: "users"}, "any")).
				Once()

			err := handler.Delete(ctx, sub)

			Expect(err).NotTo(HaveOccurred())
		})

		It("returns wrapped error when SFTP User deletion fails", func() {
			sub := testSubscription()
			ctx, mockClient := newTestContext()

			mockClient.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.User")).
				Return(fmt.Errorf("delete error")).Once()

			err := handler.Delete(ctx, sub)

			Expect(err).To(MatchError(ContainSubstring("delete error")))
		})
	})
})

var _ = Describe("filesubscription helpers", func() {
	Describe("subscriptionZoneName", func() {
		It("returns empty string when Zone is nil", func() {
			sub := &filev1.FileSubscription{}
			Expect(subscriptionZoneName(sub)).To(Equal(""))
		})

		It("returns the zone name when Zone is set", func() {
			sub := testSubscription()
			Expect(subscriptionZoneName(sub)).To(Equal(testZoneName))
		})
	})

	Describe("teamNameFromNamespace", func() {
		It("returns the full namespace when no '--' separator is present", func() {
			Expect(teamNameFromNamespace("myteam")).To(Equal("myteam"))
		})

		It("strips the environment prefix when '--' is present", func() {
			Expect(teamNameFromNamespace("env--group--team")).To(Equal("group--team"))
		})
	})
})
