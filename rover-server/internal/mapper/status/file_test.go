// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stretchr/testify/mock"
	storeLib "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	roverStore "github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	fileTestNamespace = "test-env--test-team"
	fileTestOwnerUID  = "owner-uid"
)

// newRoverWithFileRefs builds a Rover whose status references one FileExposure
// and one FileSubscription, which is what makes GetAllRoverProblems query the
// corresponding stores.
func newRoverWithFileRefs() *roverv1.Rover {
	ref := types.ObjectRef{Name: "demo-spec--test-rover", Namespace: fileTestNamespace}
	rover := &roverv1.Rover{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rover",
			Namespace: fileTestNamespace,
			UID:       fileTestOwnerUID,
		},
	}
	rover.Status.FileExposures = []types.ObjectRef{ref}
	rover.Status.FileSubscriptions = []types.ObjectRef{ref}
	return rover
}

func newNotReadyFileExposure(reason, message string) *filev1.FileExposure {
	obj := &filev1.FileExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "demo-spec--test-rover",
			Namespace:  fileTestNamespace,
			Generation: 1,
		},
	}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   filev1.GroupVersion.Group,
		Version: filev1.GroupVersion.Version,
		Kind:    "FileExposure",
	})
	obj.SetCondition(condition.NewNotReadyCondition(reason, message))
	obj.SetCondition(condition.NewProcessingCondition(reason, message))
	return obj
}

func newNotReadyFileSubscription(reason, message string) *filev1.FileSubscription {
	obj := &filev1.FileSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "demo-spec--test-rover",
			Namespace:  fileTestNamespace,
			Generation: 1,
		},
	}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   filev1.GroupVersion.Group,
		Version: filev1.GroupVersion.Version,
		Kind:    "FileSubscription",
	})
	obj.SetCondition(condition.NewNotReadyCondition(reason, message))
	obj.SetCondition(condition.NewProcessingCondition(reason, message))
	return obj
}

// newFileStores returns Stores wired with only the file sub-resource stores, so
// that GetAllRoverProblems exercises exactly the file checkers.
func newFileStores(exposures []*filev1.FileExposure, subscriptions []*filev1.FileSubscription) *roverStore.Stores {
	exposureMock := mocks.NewMockObjectStore[*filev1.FileExposure](GinkgoT())
	exposureMock.EXPECT().List(mock.Anything, mock.Anything).Return(
		&storeLib.ListResponse[*filev1.FileExposure]{Items: exposures}, nil).Maybe()

	subscriptionMock := mocks.NewMockObjectStore[*filev1.FileSubscription](GinkgoT())
	subscriptionMock.EXPECT().List(mock.Anything, mock.Anything).Return(
		&storeLib.ListResponse[*filev1.FileSubscription]{Items: subscriptions}, nil).Maybe()

	return &roverStore.Stores{
		FileExposureStore:     exposureMock,
		FileSubscriptionStore: subscriptionMock,
	}
}

var _ = Describe("File sub-resource problems", func() {

	Context("when a Rover references file sub-resources that are not ready", func() {
		It("should report a problem for the FileExposure and the FileSubscription", func() {
			exposure := newNotReadyFileExposure("ChildResourcesNotReady", "One or more child resources are not yet ready")
			subscription := newNotReadyFileSubscription("Blocked", "SFTP Tardis API returned 400")
			fileStores := newFileStores(
				[]*filev1.FileExposure{exposure},
				[]*filev1.FileSubscription{subscription},
			)

			result, err := GetAllRoverProblems(ctx, newRoverWithFileRefs(), fileStores)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Problems).To(HaveLen(2))

			kinds := []string{}
			messages := []string{}
			for _, p := range result.Problems {
				kinds = append(kinds, p.Resource.Kind)
				messages = append(messages, p.Message)
			}
			Expect(kinds).To(ConsistOf("FileExposure", "FileSubscription"))
			Expect(messages).To(ContainElement("SFTP Tardis API returned 400"))
		})
	})

	Context("when a Rover references file sub-resources that are ready", func() {
		It("should report no problems", func() {
			exposure := &filev1.FileExposure{
				ObjectMeta: metav1.ObjectMeta{Name: "demo-spec--test-rover", Namespace: fileTestNamespace, Generation: 1},
			}
			exposure.SetCondition(condition.NewReadyCondition("Ready", "Provisioned"))
			exposure.SetCondition(condition.NewDoneProcessingCondition("Provisioned"))

			subscription := &filev1.FileSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "demo-spec--test-rover", Namespace: fileTestNamespace, Generation: 1},
			}
			subscription.SetCondition(condition.NewReadyCondition("Ready", "Provisioned"))
			subscription.SetCondition(condition.NewDoneProcessingCondition("Provisioned"))

			fileStores := newFileStores(
				[]*filev1.FileExposure{exposure},
				[]*filev1.FileSubscription{subscription},
			)

			result, err := GetAllRoverProblems(ctx, newRoverWithFileRefs(), fileStores)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Problems).To(BeEmpty())
		})
	})

	Context("when a Rover references no file sub-resources", func() {
		It("should not query the file stores", func() {
			fileStores := newFileStores(nil, nil)

			result, err := GetAllRoverProblems(ctx, &roverv1.Rover{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rover",
					Namespace: fileTestNamespace,
					UID:       fileTestOwnerUID,
				},
			}, fileStores)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Problems).To(BeEmpty())
		})
	})
})
