// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	"errors"

	"github.com/stretchr/testify/mock"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
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
})
