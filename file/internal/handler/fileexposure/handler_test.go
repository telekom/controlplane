// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fileexposure

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8smeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	testNamespace             = "test"
	testFileTypeName          = "test-filetype"
	testExposureName          = "test-exposure"
	testZoneServiceConfigName = "test-zone"
)

func newTestContext() (context.Context, *fake.MockJanitorClient) {
	mockClient := fake.NewMockJanitorClient(GinkgoT())
	ctx := cclient.WithClient(context.Background(), mockClient)
	return ctx, mockClient
}

func testFileType() *filev1.FileType {
	return &filev1.FileType{
		TypeMeta: metav1.TypeMeta{
			APIVersion: filev1.GroupVersion.String(),
			Kind:       "FileType",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testFileTypeName,
			Namespace: testNamespace,
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
			UID:       "uid-active",
		},
		Spec: filev1.FileExposureSpec{
			FileType: testFileTypeName,
			Zone:     &types.ObjectRef{Name: testZoneServiceConfigName, Namespace: testNamespace},
		},
	}
}

func testZoneServiceConfig() *filev1.ZoneServiceConfig {
	return &filev1.ZoneServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testZoneServiceConfigName,
			Namespace: testNamespace,
		},
	}
}

// mockListExposures sets up c.List to return the given FileExposures.
func mockListExposures(mockClient *fake.MockJanitorClient, exposures []filev1.FileExposure) {
	mockClient.EXPECT().
		List(mock.Anything, mock.AnythingOfType("*v1.FileExposureList"), mock.Anything, mock.Anything).
		Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
			*list.(*filev1.FileExposureList) = filev1.FileExposureList{Items: exposures}
		}).
		Return(nil).Once()
}

// mockGetFileType sets up c.Get to return the given FileType.
func mockGetFileType(mockClient *fake.MockJanitorClient, ft *filev1.FileType) {
	mockClient.EXPECT().
		Get(mock.Anything, k8stypes.NamespacedName{Name: testFileTypeName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileType")).
		Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
			*out.(*filev1.FileType) = *ft
		}).
		Return(nil).Once()
}

// mockListZoneServiceConfigs sets up c.List to return the given ZoneServiceConfigs.
func mockListZoneServiceConfigs(mockClient *fake.MockJanitorClient, configs []filev1.ZoneServiceConfig) {
	mockClient.EXPECT().
		List(mock.Anything, mock.AnythingOfType("*v1.ZoneServiceConfigList"), mock.Anything, mock.Anything).
		Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
			*list.(*filev1.ZoneServiceConfigList) = filev1.ZoneServiceConfigList{Items: configs}
		}).
		Return(nil).Once()
}

var _ = Describe("FileExposureHandler", func() {
	var handler *FileExposureHandler

	BeforeEach(func() {
		handler = &FileExposureHandler{}
	})

	Describe("CreateOrUpdate", func() {
		It("blocks when another FileExposure is already active", func() {
			exposure := testFileExposure()
			exposure.UID = "uid-mine"
			ctx, mockClient := newTestContext()

			// active exposure belongs to a different UID
			otherExposure := testFileExposure()
			otherExposure.Name = "other-exposure"
			otherExposure.UID = "uid-other"
			mockListExposures(mockClient, []filev1.FileExposure{*otherExposure})

			err := handler.CreateOrUpdate(ctx, exposure)

			Expect(err).NotTo(HaveOccurred())
			Expect(exposure.Status.FileTypeRef).NotTo(BeNil())
			Expect(k8smeta.IsStatusConditionFalse(exposure.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			ready := k8smeta.FindStatusCondition(exposure.Status.Conditions, condition.ConditionTypeReady)
			Expect(ready.Reason).To(Equal("FileExposureAlreadyExists"))
		})

		It("blocks when FileType is not found", func() {
			exposure := testFileExposure()
			ctx, mockClient := newTestContext()

			mockListExposures(mockClient, []filev1.FileExposure{*exposure})
			mockClient.EXPECT().
				Get(mock.Anything, k8stypes.NamespacedName{Name: testFileTypeName, Namespace: testNamespace}, mock.AnythingOfType("*v1.FileType")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: filev1.GroupVersion.Group, Resource: "filetypes"}, testFileTypeName)).
				Once()

			err := handler.CreateOrUpdate(ctx, exposure)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("FileType"))
		})

		It("returns error when exposure list fails", func() {
			exposure := testFileExposure()
			ctx, mockClient := newTestContext()

			mockClient.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.FileExposureList"), mock.Anything, mock.Anything).
				Return(fmt.Errorf("storage error")).Once()

			err := handler.CreateOrUpdate(ctx, exposure)

			Expect(err).To(MatchError(ContainSubstring("storage error")))
		})

		It("returns error when ZoneServiceConfig is not found", func() {
			exposure := testFileExposure()
			ctx, mockClient := newTestContext()

			mockListExposures(mockClient, []filev1.FileExposure{*exposure})
			mockGetFileType(mockClient, testFileType())
			// empty ZoneServiceConfig list → "expected exactly one" error
			mockListZoneServiceConfigs(mockClient, nil)

			err := handler.CreateOrUpdate(ctx, exposure)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ZoneServiceConfig"))
		})

		It("creates SFTP Instance and provider User while children not ready", func() {
			exposure := testFileExposure()
			ctx, mockClient := newTestContext()

			mockListExposures(mockClient, []filev1.FileExposure{*exposure})
			mockGetFileType(mockClient, testFileType())
			mockListZoneServiceConfigs(mockClient, []filev1.ZoneServiceConfig{*testZoneServiceConfig()})
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Instance"), mock.Anything).
				Return(controllerutil.OperationResultCreated, nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.User"), mock.Anything).
				Return(controllerutil.OperationResultCreated, nil).Once()
			mockClient.EXPECT().AllReady().Return(false).Once()

			err := handler.CreateOrUpdate(ctx, exposure)

			Expect(err).NotTo(HaveOccurred())
			Expect(exposure.Status.FileTypeRef).NotTo(BeNil())
			Expect(k8smeta.IsStatusConditionFalse(exposure.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		})

		It("sets Ready condition when all child resources are ready", func() {
			exposure := testFileExposure()
			ctx, mockClient := newTestContext()

			mockListExposures(mockClient, []filev1.FileExposure{*exposure})
			mockGetFileType(mockClient, testFileType())
			mockListZoneServiceConfigs(mockClient, []filev1.ZoneServiceConfig{*testZoneServiceConfig()})
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Instance"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.User"), mock.Anything).
				Return(controllerutil.OperationResultNone, nil).Once()
			mockClient.EXPECT().AllReady().Return(true).Once()

			err := handler.CreateOrUpdate(ctx, exposure)

			Expect(err).NotTo(HaveOccurred())
			Expect(k8smeta.IsStatusConditionTrue(exposure.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		})

		It("returns error when Instance creation fails", func() {
			exposure := testFileExposure()
			ctx, mockClient := newTestContext()

			mockListExposures(mockClient, []filev1.FileExposure{*exposure})
			mockGetFileType(mockClient, testFileType())
			mockListZoneServiceConfigs(mockClient, []filev1.ZoneServiceConfig{*testZoneServiceConfig()})
			mockClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Instance"), mock.Anything).
				Return(controllerutil.OperationResultNone, fmt.Errorf("instance creation failed")).Once()

			err := handler.CreateOrUpdate(ctx, exposure)

			Expect(err).To(MatchError(ContainSubstring("instance creation failed")))
		})
	})

	Describe("Delete", func() {
		It("skips deletion when no active FileExposure exists", func() {
			exposure := testFileExposure()
			ctx, mockClient := newTestContext()

			mockListExposures(mockClient, nil)

			err := handler.Delete(ctx, exposure)

			Expect(err).NotTo(HaveOccurred())
		})

		It("skips deletion when the active FileExposure has a different UID", func() {
			exposure := testFileExposure()
			exposure.UID = "uid-mine"
			ctx, mockClient := newTestContext()

			otherExposure := testFileExposure()
			otherExposure.UID = "uid-other"
			mockListExposures(mockClient, []filev1.FileExposure{*otherExposure})

			err := handler.Delete(ctx, exposure)

			Expect(err).NotTo(HaveOccurred())
		})

		It("deletes provider User and Instance when this exposure is active", func() {
			exposure := testFileExposure()
			ctx, mockClient := newTestContext()

			mockListExposures(mockClient, []filev1.FileExposure{*exposure})
			mockClient.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.User")).
				Return(nil).Once()
			mockClient.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.Instance")).
				Return(nil).Once()

			err := handler.Delete(ctx, exposure)

			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error when exposure list fails during delete", func() {
			exposure := testFileExposure()
			ctx, mockClient := newTestContext()

			mockClient.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.FileExposureList"), mock.Anything, mock.Anything).
				Return(fmt.Errorf("list error on delete")).Once()

			err := handler.Delete(ctx, exposure)

			Expect(err).To(MatchError(ContainSubstring("list error on delete")))
		})

		It("returns error when provider User deletion fails", func() {
			exposure := testFileExposure()
			ctx, mockClient := newTestContext()

			mockListExposures(mockClient, []filev1.FileExposure{*exposure})
			mockClient.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.User")).
				Return(fmt.Errorf("user delete failed")).Once()

			err := handler.Delete(ctx, exposure)

			Expect(err).To(MatchError(ContainSubstring("user delete failed")))
		})

		It("tolerates NotFound when deleting provider User", func() {
			exposure := testFileExposure()
			ctx, mockClient := newTestContext()

			mockListExposures(mockClient, []filev1.FileExposure{*exposure})
			mockClient.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.User")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: sftpv1.GroupVersion.Group, Resource: "users"}, "any")).
				Once()
			mockClient.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.Instance")).
				Return(nil).Once()

			err := handler.Delete(ctx, exposure)

			Expect(err).NotTo(HaveOccurred())
		})
	})
})
