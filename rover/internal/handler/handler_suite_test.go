// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	apihandler "github.com/telekom/controlplane/rover/internal/handler/rover/api"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	clientmock "github.com/telekom/controlplane/common/pkg/client/mocks"
	commoncondition "github.com/telekom/controlplane/common/pkg/condition"
)

var (
	testEnv = "testenv"
)

func TestHandleSubscription(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HandleSubscription Suite")
}

var _ = Describe("HandleSubscription", func() {
	var (
		mockCtrl   *gomock.Controller
		mockClient *clientmock.MockJanitorClient
		ctx        context.Context
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmock.NewMockJanitorClient(mockCtrl)
		ctx = context.TODO()
		ctx = contextutil.WithEnv(ctx, testEnv)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("succeeds when client.Create returns no error", func() {
		// values don't actually matter at the moment
		rover := &roverv1.Rover{}
		roverSub := &roverv1.ApiSubscription{}
		apiSub := &apiv1.ApiSubscription{}

		// Expect the client Create call to succeed
		mockClient.EXPECT().
			CreateOrUpdate(ctx, gomock.AssignableToTypeOf(apiSub), gomock.Any()).
			Return(controllerutil.OperationResultCreated, nil) // the created doesnt matter here

		err := apihandler.HandleSubscription(ctx, mockClient, rover, roverSub)
		Expect(err).NotTo(HaveOccurred())
	})

	It("blocked when client.Create returns NotFound error", func() {
		// values don't actually matter at the moment
		rover := &roverv1.Rover{}
		roverSub := &roverv1.ApiSubscription{
			BasePath: "/foo/bar",
		}
		apiSub := &apiv1.ApiSubscription{}

		// the desired error
		statusErr := apierrors.StatusError{}
		statusErr.ErrStatus.Reason = metav1.StatusReasonNotFound
		// Expect the client Create call to succeed
		mockClient.EXPECT().
			CreateOrUpdate(ctx, gomock.AssignableToTypeOf(apiSub), gomock.Any()).
			Return(controllerutil.OperationResultNone, &statusErr) // the created doesnt matter here

		err := apihandler.HandleSubscription(ctx, mockClient, rover, roverSub)
		AssertConditionBlockedPresence(rover.GetConditions(), "Blocked due to missing ApiExposure for subscription to basePath '/foo/bar'", true)
		Expect(err).To(HaveOccurred())
	})

	It("rejected when client.Create returns BadRequest error", func() {
		// values don't actually matter at the moment
		rover := &roverv1.Rover{}
		roverSub := &roverv1.ApiSubscription{
			BasePath: "/foo/bar",
		}
		apiSub := &apiv1.ApiSubscription{}

		// the desired error
		statusErr := apierrors.StatusError{}
		statusErr.ErrStatus.Reason = metav1.StatusReasonBadRequest
		// Expect the client Create call to succeed
		mockClient.EXPECT().
			CreateOrUpdate(ctx, gomock.AssignableToTypeOf(apiSub), gomock.Any()).
			Return(controllerutil.OperationResultNone, &statusErr) // the created doesnt matter here

		err := apihandler.HandleSubscription(ctx, mockClient, rover, roverSub)
		AssertConditionBlockedPresence(rover.GetConditions(), "", false)
		Expect(err).To(HaveOccurred())
	})

})

// AssertConditionBlockedPresence checks whether the Blocked condition is present or absent
func AssertConditionBlockedPresence(conditions []metav1.Condition, expectedMessage string, shouldExist bool) {
	found := false
	for _, cond := range conditions {
		if cond.Type == commoncondition.ProcessingCondition.Type && // Use your condition type
			cond.Status == metav1.ConditionFalse &&
			cond.Reason == "Blocked" &&
			cond.Message == expectedMessage {
			found = true
			break
		}
	}

	if shouldExist {
		Expect(found).To(BeTrue(), "Expected Blocked condition with message %q to be present", expectedMessage)
	} else {
		Expect(found).To(BeFalse(), "Expected Blocked condition with message %q to be absent", expectedMessage)
	}
}
