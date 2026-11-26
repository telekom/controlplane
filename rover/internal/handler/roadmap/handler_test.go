// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package roadmap_test

import (
	"context"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"github.com/telekom/controlplane/rover/internal/handler/roadmap"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func createRoadmapObject(kind string) *roverv1.Roadmap {
	return &roverv1.Roadmap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roadmap",
			Namespace: teamNamespace,
			UID:       "roadmap-uid",
		},
		Spec: roverv1.RoadmapSpec{
			SpecificationRef: types.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{
					Kind:       kind,
					APIVersion: "rover.cp.ei.telekom.de/v1",
				},
				ObjectRef: types.ObjectRef{
					Name:      "eni-my-api-v1",
					Namespace: teamNamespace,
				},
			},
			Contents: "test--eni--team--my-api-v1",
			Hash:     "abc123hash",
		},
	}
}

// newMockContext injects a mock client. If apiSpec is nil, Get returns NotFound.
// The mirror passed to CreateOrUpdate is written to captured.
// No call is expected on the client unless the test triggers it, so unexpected calls fail the test.
func newMockContext(apiSpec *roverv1.ApiSpecification, captured **apiapi.ApiRoadmap, anyChanged bool) context.Context {
	fakeClient := fakeclient.NewMockJanitorClient(GinkgoT())
	testScheme := runtime.NewScheme()
	_ = roverv1.AddToScheme(testScheme)
	_ = apiapi.AddToScheme(testScheme)

	fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
	fakeClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			if apiSpec == nil {
				return apierrors.NewNotFound(schema.GroupResource{Resource: "apispecifications"}, "eni-my-api-v1")
			}
			apiSpec.DeepCopyInto(obj.(*roverv1.ApiSpecification))
			return nil
		}).Maybe()
	fakeClient.EXPECT().CreateOrUpdate(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, obj client.Object, fn controllerutil.MutateFn) (controllerutil.OperationResult, error) {
			Expect(fn()).To(Succeed())
			*captured = obj.(*apiapi.ApiRoadmap)
			return controllerutil.OperationResultCreated, nil
		}).Maybe()
	fakeClient.EXPECT().AnyChanged().Return(anyChanged).Maybe()

	return cclient.WithClient(context.Background(), fakeClient)
}

var _ = Describe("Roadmap Handler", func() {
	var roadmapHandler *roadmap.RoadmapHandler
	var apiSpec *roverv1.ApiSpecification

	BeforeEach(func() {
		roadmapHandler = &roadmap.RoadmapHandler{}
		apiSpec = &roverv1.ApiSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "eni-my-api-v1", Namespace: teamNamespace},
			Spec:       roverv1.ApiSpecificationSpec{BasePath: "/eni/my-api/v1"},
		}
	})

	Context("CreateOrUpdate", func() {
		It("should mirror a roadmap for an ApiSpecification into an api ApiRoadmap", func() {
			var mirror *apiapi.ApiRoadmap
			ctx := newMockContext(apiSpec, &mirror, false)
			roadmapObj := createRoadmapObject("ApiSpecification")

			Expect(roadmapHandler.CreateOrUpdate(ctx, roadmapObj)).To(Succeed())

			Expect(mirror).NotTo(BeNil())
			Expect(mirror.Name).To(Equal(roadmapObj.Name))
			Expect(mirror.Spec.BasePath).To(Equal("/eni/my-api/v1"))
			Expect(mirror.Spec.Contents).To(Equal(roadmapObj.Spec.Contents))
			Expect(mirror.Spec.Hash).To(Equal(roadmapObj.Spec.Hash))
			Expect(mirror.Labels).To(HaveKey(apiapi.BasePathLabelKey))
			Expect(mirror.OwnerReferences).To(HaveLen(1))
			Expect(mirror.OwnerReferences[0].UID).To(Equal(roadmapObj.UID))
			Expect(roadmapObj.Status.ApiRoadmap.Name).To(Equal(mirror.Name))
			Expect(roadmapObj.Status.ApiRoadmap.Namespace).To(Equal(mirror.Namespace))

			readyCond := meta.FindStatusCondition(roadmapObj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should report not ready while the mirror is being changed", func() {
			var mirror *apiapi.ApiRoadmap
			ctx := newMockContext(apiSpec, &mirror, true)
			roadmapObj := createRoadmapObject("ApiSpecification")

			Expect(roadmapHandler.CreateOrUpdate(ctx, roadmapObj)).To(Succeed())

			readyCond := meta.FindStatusCondition(roadmapObj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should not mirror a roadmap for an EventSpecification", func() {
			// A bare context has no client: the handler must not touch one for this kind.
			roadmapObj := createRoadmapObject("EventSpecification")

			Expect(roadmapHandler.CreateOrUpdate(context.Background(), roadmapObj)).To(Succeed())

			readyCond := meta.FindStatusCondition(roadmapObj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should be blocked and not mirror if the ApiSpecification does not exist", func() {
			var mirror *apiapi.ApiRoadmap
			ctx := newMockContext(nil, &mirror, false)

			err := roadmapHandler.CreateOrUpdate(ctx, createRoadmapObject("ApiSpecification"))

			Expect(err).To(HaveOccurred())
			blocked, ok := err.(ctrlerrors.BlockedError)
			Expect(ok).To(BeTrue())
			Expect(blocked.IsBlocked()).To(BeTrue())
			Expect(mirror).To(BeNil())
		})
	})

	Context("Delete", func() {
		It("should be a no-op", func() {
			Expect(roadmapHandler.Delete(context.Background(), createRoadmapObject("ApiSpecification"))).To(Succeed())
		})
	})
})
