// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	"errors"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiExposureMustNotAlreadyExist", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		candidate  *apiapi.ApiExposure
	)

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), testEnv)
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		candidate = &apiapi.ApiExposure{
			Spec: apiapi.ApiExposureSpec{ApiBasePath: "/eni/test/v1"},
		}
	})

	It("wraps and propagates the error when listing exposures fails", func() {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApiExposureList"), mock.Anything, mock.Anything).
			Return(errors.New("boom")).Once()

		err := ApiExposureMustNotAlreadyExist(ctx, candidate)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to find active ApiExposure"))
		Expect(err.Error()).To(ContainSubstring("boom"))
		Expect(candidate.Status.Active).To(BeFalse())
	})

	It("marks the candidate active when no other active exposure exists", func() {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApiExposureList"), mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*apiapi.ApiExposureList) = apiapi.ApiExposureList{}
			}).
			Return(nil).Once()

		err := ApiExposureMustNotAlreadyExist(ctx, candidate)
		Expect(err).NotTo(HaveOccurred())
		Expect(candidate.Status.Active).To(BeTrue())
	})

	It("explains automatic takeover in Ready and Processing when another exposure is active", func() {
		existing := apiapi.ApiExposure{
			ObjectMeta: metav1.ObjectMeta{Name: "existing", Namespace: "prod--eni--other-team"},
			Spec:       apiapi.ApiExposureSpec{ApiBasePath: "/eni/test/v1"},
			Status:     apiapi.ApiExposureStatus{Active: true},
		}
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApiExposureList"), mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*apiapi.ApiExposureList) = apiapi.ApiExposureList{Items: []apiapi.ApiExposure{existing}}
			}).
			Return(nil).Once()

		err := ApiExposureMustNotAlreadyExist(ctx, candidate)
		Expect(err).NotTo(HaveOccurred())
		Expect(candidate.Status.Active).To(BeFalse())

		readyCond := meta.FindStatusCondition(candidate.GetConditions(), condition.ConditionTypeReady)
		processingCond := meta.FindStatusCondition(candidate.GetConditions(), condition.ConditionTypeProcessing)
		Expect(readyCond).NotTo(BeNil())
		Expect(processingCond).NotTo(BeNil())
		Expect(readyCond.Message).To(Equal(
			`Another ApiExposure with base path "/eni/test/v1" already exists and prevents this instance from becoming active. It is owned by team "eni--other-team". ` +
				condition.MessageWaitingForTakeover))
		Expect(processingCond.Message).To(Equal(readyCond.Message))
	})
})

var _ = DescribeTable("setAlreadyExposedConditions",
	func(existingNamespace string, labels map[string]string, candidateNamespace, expectedOwner string) {
		existing := &apiapi.ApiExposure{
			ObjectMeta: metav1.ObjectMeta{Name: "existing", Namespace: existingNamespace, Labels: labels},
			Spec:       apiapi.ApiExposureSpec{ApiBasePath: "/eni/test/v1"},
		}
		candidate := &apiapi.ApiExposure{
			ObjectMeta: metav1.ObjectMeta{Name: "candidate", Namespace: candidateNamespace},
			Spec:       apiapi.ApiExposureSpec{ApiBasePath: "/eni/test/v1"},
		}
		setAlreadyExposedConditions(existing, candidate)
		ready := meta.FindStatusCondition(candidate.GetConditions(), condition.ConditionTypeReady)
		processing := meta.FindStatusCondition(candidate.GetConditions(), condition.ConditionTypeProcessing)
		Expect(ready).NotTo(BeNil())
		Expect(processing).NotTo(BeNil())
		Expect(ready.Message).To(Equal(
			`Another ApiExposure with base path "/eni/test/v1" already exists and prevents this instance from becoming active. It is owned by ` + expectedOwner + ". " +
				condition.MessageWaitingForTakeover))
		Expect(processing.Message).To(Equal(ready.Message))
	},
	Entry("same team with application label", "prod--eni--team-a",
		map[string]string{config.BuildLabelKey("application"): "app-1"}, "prod--eni--team-a",
		`team "eni--team-a", application "app-1"`),
	Entry("cross team with application label uses existing owner", "prod--eni--team-a",
		map[string]string{config.BuildLabelKey("application"): "app-1"}, "prod--eni--team-b",
		`team "eni--team-a", application "app-1"`),
	Entry("missing application label omits application", "prod--eni--team-a", nil, "prod--eni--team-b",
		`team "eni--team-a"`),
)
