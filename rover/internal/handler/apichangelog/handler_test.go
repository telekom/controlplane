// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apichangelog_test

import (
	"context"
	"errors"

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
	"github.com/telekom/controlplane/rover/internal/handler/apichangelog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func createApiChangelogObject() *roverv1.ApiChangelog {
	return &roverv1.ApiChangelog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-apichangelog",
			Namespace: teamNamespace,
			UID:       "changelog-uid",
		},
		Spec: roverv1.ApiChangelogSpec{
			SpecificationRef: types.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ApiSpecification",
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
func newMockContext(apiSpec *roverv1.ApiSpecification, captured **apiapi.ApiChangelog, anyChanged, allReady bool) context.Context {
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
			*captured = obj.(*apiapi.ApiChangelog)
			return controllerutil.OperationResultCreated, nil
		}).Maybe()
	fakeClient.EXPECT().AnyChanged().Return(anyChanged).Maybe()
	fakeClient.EXPECT().AllReady().Return(allReady).Maybe()

	return cclient.WithClient(context.Background(), fakeClient)
}

// newDeleteContext injects a mock client whose Delete records the object and returns err.
func newDeleteContext(err error, deleted **apiapi.ApiChangelog) context.Context {
	fakeClient := fakeclient.NewMockJanitorClient(GinkgoT())
	fakeClient.EXPECT().Delete(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, obj client.Object, _ ...client.DeleteOption) error {
			*deleted = obj.(*apiapi.ApiChangelog)
			return err
		})
	return cclient.WithClient(context.Background(), fakeClient)
}

var _ = Describe("ApiChangelog Handler", func() {
	var changelogHandler *apichangelog.ApiChangelogHandler
	var apiSpec *roverv1.ApiSpecification

	BeforeEach(func() {
		changelogHandler = &apichangelog.ApiChangelogHandler{}
		apiSpec = &roverv1.ApiSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "eni-my-api-v1", Namespace: teamNamespace},
			Spec:       roverv1.ApiSpecificationSpec{BasePath: "/eni/my-api/v1"},
		}
	})

	Context("CreateOrUpdate", func() {
		It("should mirror the changelog into an api ApiChangelog", func() {
			var mirror *apiapi.ApiChangelog
			ctx := newMockContext(apiSpec, &mirror, false, true)
			changelogObj := createApiChangelogObject()

			Expect(changelogHandler.CreateOrUpdate(ctx, changelogObj)).To(Succeed())

			Expect(mirror).NotTo(BeNil())
			Expect(mirror.Name).To(Equal(changelogObj.Name))
			Expect(mirror.Namespace).To(Equal(changelogObj.Namespace))
			Expect(mirror.Spec.BasePath).To(Equal("/eni/my-api/v1"))
			Expect(mirror.Spec.Contents).To(Equal(changelogObj.Spec.Contents))
			Expect(mirror.Spec.Hash).To(Equal(changelogObj.Spec.Hash))
			Expect(mirror.Labels).To(HaveKey(apiapi.BasePathLabelKey))
			Expect(mirror.OwnerReferences).To(HaveLen(1))
			Expect(mirror.OwnerReferences[0].UID).To(Equal(changelogObj.UID))
			Expect(changelogObj.Status.ApiChangelog.Name).To(Equal(mirror.Name))
			Expect(changelogObj.Status.ApiChangelog.Namespace).To(Equal(mirror.Namespace))

			readyCond := meta.FindStatusCondition(changelogObj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should report not ready while the mirror is being changed", func() {
			var mirror *apiapi.ApiChangelog
			ctx := newMockContext(apiSpec, &mirror, true, true)
			changelogObj := createApiChangelogObject()

			Expect(changelogHandler.CreateOrUpdate(ctx, changelogObj)).To(Succeed())

			readyCond := meta.FindStatusCondition(changelogObj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should report not ready when the mirror is unchanged but blocked", func() {
			var mirror *apiapi.ApiChangelog
			ctx := newMockContext(apiSpec, &mirror, false, false)
			changelogObj := createApiChangelogObject()

			Expect(changelogHandler.CreateOrUpdate(ctx, changelogObj)).To(Succeed())

			readyCond := meta.FindStatusCondition(changelogObj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should look up the ApiSpecification in its own namespace, ignoring the ref namespace", func() {
			var key client.ObjectKey
			fakeClient := fakeclient.NewMockJanitorClient(GinkgoT())
			fakeClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, k client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
					key = k
					return apierrors.NewNotFound(schema.GroupResource{Resource: "apispecifications"}, k.Name)
				})
			changelogObj := createApiChangelogObject()
			changelogObj.Spec.SpecificationRef.Namespace = "other-env--team"

			_ = changelogHandler.CreateOrUpdate(cclient.WithClient(context.Background(), fakeClient), changelogObj)

			Expect(key.Namespace).To(Equal(changelogObj.Namespace))
		})

		It("should be blocked and not mirror if the ApiSpecification does not exist", func() {
			var mirror *apiapi.ApiChangelog
			ctx := newMockContext(nil, &mirror, false, true)

			err := changelogHandler.CreateOrUpdate(ctx, createApiChangelogObject())

			Expect(err).To(HaveOccurred())
			blocked, ok := err.(ctrlerrors.BlockedError)
			Expect(ok).To(BeTrue())
			Expect(blocked.IsBlocked()).To(BeTrue())
			Expect(mirror).To(BeNil())
		})
	})

	Context("Delete", func() {
		It("should delete the mirror explicitly", func() {
			var deleted *apiapi.ApiChangelog
			ctx := newDeleteContext(nil, &deleted)
			changelogObj := createApiChangelogObject()

			Expect(changelogHandler.Delete(ctx, changelogObj)).To(Succeed())

			Expect(deleted).NotTo(BeNil())
			Expect(deleted.Name).To(Equal(changelogObj.Name))
			Expect(deleted.Namespace).To(Equal(changelogObj.Namespace))
		})

		It("should succeed if the mirror is already gone", func() {
			var deleted *apiapi.ApiChangelog
			ctx := newDeleteContext(apierrors.NewNotFound(schema.GroupResource{Resource: "apichangelogs"}, "x"), &deleted)

			Expect(changelogHandler.Delete(ctx, createApiChangelogObject())).To(Succeed())
		})

		It("should return other delete errors", func() {
			var deleted *apiapi.ApiChangelog
			ctx := newDeleteContext(errors.New("boom"), &deleted)

			Expect(changelogHandler.Delete(ctx, createApiChangelogObject())).To(HaveOccurred())
		})
	})
})
