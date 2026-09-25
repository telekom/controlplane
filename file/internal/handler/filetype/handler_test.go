// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filetype

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"
	k8smeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"

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
			Name:       testFileTypeName,
			Namespace:  testNamespace,
			Generation: 1,
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
			FileType: testFileTypeName,
			Zone:     &types.ObjectRef{Name: testZoneServiceConfigName, Namespace: testNamespace},
		},
	}
}

func testZone() *adminv1.Zone {
	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testZoneServiceConfigName,
			Namespace: testNamespace,
		},
		Status: adminv1.ZoneStatus{
			Namespace: testNamespace,
		},
	}
}

// mockListExposures sets up c.List to return the given exposures.
func mockListExposures(mockClient *fake.MockJanitorClient, exposures []filev1.FileExposure) {
	mockClient.EXPECT().
		List(mock.Anything, mock.AnythingOfType("*v1.FileExposureList"), mock.Anything, mock.Anything).
		Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
			*list.(*filev1.FileExposureList) = filev1.FileExposureList{Items: exposures}
		}).
		Return(nil).Once()
}

func mockGetZone(mockClient *fake.MockJanitorClient) {
	zone := testZone()
	mockClient.EXPECT().
		Get(mock.Anything, k8stypes.NamespacedName{Name: zone.Name, Namespace: zone.Namespace}, mock.AnythingOfType("*v1.Zone")).
		Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
			*obj.(*adminv1.Zone) = *zone
		}).
		Return(nil).Once()
}

var _ = Describe("FileTypeHandler", func() {
	var handler *FileTypeHandler

	BeforeEach(func() {
		handler = &FileTypeHandler{}
	})

	Describe("CreateOrUpdate", func() {
		It("blocks when no active FileExposure exists", func() {
			fileType := testFileType()
			ctx, mockClient := newTestContext()

			mockListExposures(mockClient, nil)

			err := handler.CreateOrUpdate(ctx, fileType)

			Expect(err).NotTo(HaveOccurred())
			Expect(fileType.Status.FileExposureRef).To(BeNil())
			Expect(k8smeta.IsStatusConditionFalse(fileType.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			ready := k8smeta.FindStatusCondition(fileType.Status.Conditions, condition.ConditionTypeReady)
			Expect(ready.Reason).To(Equal(condition.ReasonPreconditionNotMet))
		})

		It("returns error when exposure list fails", func() {
			fileType := testFileType()
			ctx, mockClient := newTestContext()

			mockClient.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.FileExposureList"), mock.Anything, mock.Anything).
				Return(fmt.Errorf("storage unavailable")).Once()

			err := handler.CreateOrUpdate(ctx, fileType)

			Expect(err).To(MatchError(ContainSubstring("storage unavailable")))
		})

		It("returns error when ZoneServiceConfig list fails", func() {
			fileType := testFileType()
			exposure := testFileExposure()
			ctx, mockClient := newTestContext()

			mockListExposures(mockClient, []filev1.FileExposure{*exposure})
			mockGetZone(mockClient)
			mockClient.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.ZoneServiceConfigList"), mock.Anything, mock.Anything).
				Return(fmt.Errorf("zone config unavailable")).Once()

			err := handler.CreateOrUpdate(ctx, fileType)

			Expect(err).To(MatchError(ContainSubstring("zone config unavailable")))
		})
	})
})
