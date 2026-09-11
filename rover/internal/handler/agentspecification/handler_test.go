// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agentspecification_test

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"github.com/telekom/controlplane/rover/internal/handler/agentspecification"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newAgentSpecification(basePath string) *roverv1.AgentSpecification {
	return &roverv1.AgentSpecification{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roverv1.MakeAgentSpecificationName(basePath),
			Namespace: "test-env--grp--team",
			UID:       "spec-uid-1",
		},
		Spec: roverv1.AgentSpecificationSpec{
			BasePath:      basePath,
			Version:       "1.0.0",
			Name:          "Test Agent",
			Description:   "A test agent card",
			Specification: "file-id-123",
			Category:      "other",
			Oauth2Scopes:  []string{"read", "write"},
		},
	}
}

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = roverv1.AddToScheme(s)
	_ = agenticv1.AddToScheme(s)
	return s
}

var _ = Describe("AgentSpecificationHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *agentspecification.AgentSpecificationHandler
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &agentspecification.AgentSpecificationHandler{}
		scheme = newScheme()
	})

	Describe("CreateOrUpdate", func() {
		It("should create an AgentCard with correct spec fields", func() {
			spec := newAgentSpecification("/agent/assistant/v1")

			fakeClient.EXPECT().Scheme().Return(scheme).Maybe()
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgentCard"), mock.Anything).
				Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
					// Execute the mutator to populate the AgentCard
					Expect(mutate()).To(Succeed())

					card := obj.(*agenticv1.AgentCard)
					Expect(card.Spec.BasePath).To(Equal("/agent/assistant/v1"))
					Expect(card.Spec.Version).To(Equal("1.0.0"))
					Expect(card.Spec.Name).To(Equal("Test Agent"))
					Expect(card.Spec.Description).To(Equal("A test agent card"))
					Expect(card.Spec.Specification).To(Equal("file-id-123"))
					Expect(card.Spec.Category).To(Equal("other"))
					Expect(card.Spec.Oauth2Scopes).To(Equal([]string{"read", "write"}))
					Expect(card.Labels).To(HaveKeyWithValue(
						agenticv1.AgentBasePathLabelKey,
						labelutil.NormalizeLabelValue("/agent/assistant/v1"),
					))
				}).
				Return(controllerutil.OperationResultCreated, nil)

			fakeClient.EXPECT().AnyChanged().Return(true)

			err := h.CreateOrUpdate(ctx, spec)

			Expect(err).NotTo(HaveOccurred())

			// Status should reference the created AgentCard
			Expect(spec.Status.AgentCard.Name).To(Equal(labelutil.NormalizeNameValue(
				roverv1.MakeAgentSpecificationName("/agent/assistant/v1"),
			)))
			Expect(spec.Status.AgentCard.Namespace).To(Equal("test-env--grp--team"))

			// Conditions should indicate processing
			readyCond := meta.FindStatusCondition(spec.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("Provisioning"))
		})

		It("should set Ready condition when AgentCard is unchanged", func() {
			spec := newAgentSpecification("/agent/assistant/v1")

			fakeClient.EXPECT().Scheme().Return(scheme).Maybe()
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgentCard"), mock.Anything).
				Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
					Expect(mutate()).To(Succeed())
				}).
				Return(controllerutil.OperationResultNone, nil)

			fakeClient.EXPECT().AnyChanged().Return(false)

			err := h.CreateOrUpdate(ctx, spec)

			Expect(err).NotTo(HaveOccurred())

			readyCond := meta.FindStatusCondition(spec.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Provisioned"))
		})

		It("should return error when CreateOrUpdate fails", func() {
			spec := newAgentSpecification("/agent/assistant/v1")

			fakeClient.EXPECT().Scheme().Return(scheme).Maybe()
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgentCard"), mock.Anything).
				Return(controllerutil.OperationResultNone, fmt.Errorf("connection refused"))

			err := h.CreateOrUpdate(ctx, spec)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create or update AgentCard"))
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})

		It("should set controller reference on the AgentCard", func() {
			spec := newAgentSpecification("/agent/assistant/v1")

			fakeClient.EXPECT().Scheme().Return(scheme).Maybe()
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgentCard"), mock.Anything).
				Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
					Expect(mutate()).To(Succeed())

					card := obj.(*agenticv1.AgentCard)
					// Verify owner reference was set
					ownerRefs := card.GetOwnerReferences()
					Expect(ownerRefs).To(HaveLen(1))
					Expect(ownerRefs[0].UID).To(Equal(spec.UID))
				}).
				Return(controllerutil.OperationResultCreated, nil)

			fakeClient.EXPECT().AnyChanged().Return(true)

			err := h.CreateOrUpdate(ctx, spec)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should derive AgentCard name from basePath", func() {
			spec := newAgentSpecification("/my/deep/agent/path/v2")

			fakeClient.EXPECT().Scheme().Return(scheme).Maybe()
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.AgentCard"), mock.Anything).
				Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
					Expect(mutate()).To(Succeed())
					Expect(obj.GetName()).To(Equal("my-deep-agent-path-v2"))
				}).
				Return(controllerutil.OperationResultCreated, nil)

			fakeClient.EXPECT().AnyChanged().Return(true)

			err := h.CreateOrUpdate(ctx, spec)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Delete", func() {
		It("should succeed without errors (cleanup via owner reference)", func() {
			spec := newAgentSpecification("/agent/assistant/v1")

			err := h.Delete(ctx, spec)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
