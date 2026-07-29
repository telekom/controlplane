// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apispecification_test

import (
	"context"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	handler "github.com/telekom/controlplane/rover/internal/handler/apispecification"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newApiSpec(category string) *roverv1.ApiSpecification {
	return &roverv1.ApiSpecification{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-spec",
			Namespace: "test-env--test-team",
			UID:       "test-uid-1234",
			Labels: map[string]string{
				"controlplane.2/environment": "test-env",
			},
		},
		Spec: roverv1.ApiSpecificationSpec{
			Specification: "file-id-123",
			Category:      category,
			BasePath:      "/eni/test/v1",
			Hash:          "hash1",
			Version:       "1.0.0",
		},
	}
}

func hasCondition(apiSpec *roverv1.ApiSpecification, condType string) bool {
	for _, c := range apiSpec.GetConditions() {
		if c.Type == condType {
			return true
		}
	}
	return false
}

func conditionMessage(apiSpec *roverv1.ApiSpecification, condType string) string {
	for _, c := range apiSpec.GetConditions() {
		if c.Type == condType {
			return c.Message
		}
	}
	return ""
}

// setupMockClient creates a mock JanitorClient injected into context.
// The mock expects CreateOrUpdate and returns success.
func setupMockClient(ctx context.Context) context.Context {
	fakeClient := fakeclient.NewMockJanitorClient(GinkgoT())
	testScheme := runtime.NewScheme()
	_ = roverv1.AddToScheme(testScheme)
	_ = apiapi.AddToScheme(testScheme)

	fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
	fakeClient.EXPECT().
		CreateOrUpdate(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ client.Object, fn controllerutil.MutateFn) (controllerutil.OperationResult, error) {
			_ = fn()
			return controllerutil.OperationResultCreated, nil
		}).Maybe()
	fakeClient.EXPECT().AnyChanged().Return(true).Maybe()

	return cclient.WithClient(ctx, fakeClient)
}

var _ = Describe("ApiSpecification Handler", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("CreateOrUpdate", func() {
		It("should create Api resource successfully", func() {
			mockCtx := setupMockClient(ctx)
			h := &handler.ApiSpecificationHandler{}
			apiSpec := newApiSpec("other")

			err := h.CreateOrUpdate(mockCtx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeTrue())
			Expect(conditionMessage(apiSpec, condition.ConditionTypeProcessing)).To(ContainSubstring("API updated"))
		})

		It("should create Api resource regardless of failing lint result", func() {
			mockCtx := setupMockClient(ctx)
			h := &handler.ApiSpecificationHandler{}
			apiSpec := newApiSpec("warn-cat")
			apiSpec.Spec.Lint = &roverv1.LintResult{Passed: false, Message: "found 2 warnings"}

			err := h.CreateOrUpdate(mockCtx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeTrue())
			Expect(conditionMessage(apiSpec, condition.ConditionTypeProcessing)).To(ContainSubstring("API updated"))
		})

		It("should create Api resource when linting passed", func() {
			mockCtx := setupMockClient(ctx)
			h := &handler.ApiSpecificationHandler{}
			apiSpec := newApiSpec("other")
			apiSpec.Spec.Lint = &roverv1.LintResult{Passed: true, Message: "no errors"}

			err := h.CreateOrUpdate(mockCtx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeTrue())
			Expect(conditionMessage(apiSpec, condition.ConditionTypeProcessing)).To(ContainSubstring("API updated"))
		})
	})

	Context("Delete", func() {
		It("should return nil", func() {
			h := &handler.ApiSpecificationHandler{}
			err := h.Delete(ctx, newApiSpec("other"))
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
