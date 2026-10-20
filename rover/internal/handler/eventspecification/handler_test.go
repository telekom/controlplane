// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventspecification_test

import (
	"context"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	handler "github.com/telekom/controlplane/rover/internal/handler/eventspecification"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EventSpecification Handler", func() {
	It("should not be ready when the unchanged EventType is not ready", func() {
		testScheme := runtime.NewScheme()
		Expect(roverv1.AddToScheme(testScheme)).To(Succeed())
		Expect(eventv1.AddToScheme(testScheme)).To(Succeed())

		fakeClient := fakeclient.NewMockJanitorClient(GinkgoT())
		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(mock.Anything, mock.Anything, mock.Anything).
			Return(controllerutil.OperationResultNone, nil)
		fakeClient.EXPECT().AnyChanged().Return(false)
		fakeClient.EXPECT().AllReady().Return(false)
		ctx := cclient.WithClient(context.Background(), fakeClient)

		eventSpec := &roverv1.EventSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "test-spec", Namespace: "test-env--test-team"},
			Spec:       roverv1.EventSpecificationSpec{Type: "de.telekom.test.v1", Version: "1.0.0"},
		}

		Expect((&handler.EventSpecificationHandler{}).CreateOrUpdate(ctx, eventSpec)).To(Succeed())

		ready := meta.FindStatusCondition(eventSpec.GetConditions(), condition.ConditionTypeReady)
		Expect(ready).ToNot(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(condition.ReasonSubResourceNotReady))
	})
})
