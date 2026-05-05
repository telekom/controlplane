// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apichangelog_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"github.com/telekom/controlplane/rover/internal/handler/apichangelog"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createApiChangelogObject() *roverv1.ApiChangelog {
	return &roverv1.ApiChangelog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-apichangelog",
			Namespace: teamNamespace,
		},
		Spec: roverv1.ApiChangelogSpec{
			SpecificationRef: types.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ApiSpecification",
					APIVersion: "rover.cp.ei.telekom.de/v1",
				},
				ObjectRef: types.ObjectRef{
					Name:      "eni-my-api",
					Namespace: teamNamespace,
				},
			},
			Contents: "test--eni--team--my-api-v1",
			Hash:     "abc123hash",
		},
	}
}

var _ = Describe("ApiChangelog Handler", func() {

	var ctx context.Context
	var changelogHandler *apichangelog.ApiChangelogHandler

	BeforeEach(func() {
		ctx = context.Background()

		By("Setup ApiChangelog Handler")
		changelogHandler = &apichangelog.ApiChangelogHandler{}
	})

	Context("CreateOrUpdate", func() {
		It("should successfully set conditions", func() {
			By("Create ApiChangelog Object")
			changelogObj := createApiChangelogObject()

			By("Call CreateOrUpdate on ApiChangelog Handler")
			err := changelogHandler.CreateOrUpdate(ctx, changelogObj)
			Expect(err).ToNot(HaveOccurred())

			By("Verify Ready condition is set")
			readyCond := meta.FindStatusCondition(changelogObj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Ready"))

			By("Verify Processing condition is set to false")
			processingCond := meta.FindStatusCondition(changelogObj.Status.Conditions, condition.ConditionTypeProcessing)
			Expect(processingCond).NotTo(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should handle changelog with different specification ref", func() {
			changelogObj := createApiChangelogObject()
			changelogObj.Spec.SpecificationRef.Name = "eni-test-api"

			err := changelogHandler.CreateOrUpdate(ctx, changelogObj)
			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(changelogObj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Delete", func() {
		It("should be a no-op", func() {
			By("Create ApiChangelog Object")
			changelogObj := createApiChangelogObject()

			By("Call Delete on ApiChangelog Handler")
			err := changelogHandler.Delete(ctx, changelogObj)
			Expect(err).ToNot(HaveOccurred())

			By("Verify no conditions are set (it's a no-op)")
			// Delete doesn't set any conditions in the handler
			// File deletion is handled by REST API layer
		})
	})
})
