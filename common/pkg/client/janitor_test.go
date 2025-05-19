// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("JanitorClient", func() {

	var (
		ctx   context.Context
		jc    *janitorClient
		templ = test.NewObject(name, namespace)
	)

	templ.SetLabels(map[string]string{
		config.EnvironmentLabelKey: environment,
	})

	BeforeEach(func() {
		jc = &janitorClient{
			ScopedClient: NewScopedClient(k8sClient, environment),
			state:        make(map[schema.GroupVersionKind]map[client.ObjectKey]bool),
		}
		ctx = contextutil.WithEnv(context.Background(), environment)
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, templ.DeepCopy())
		Expect(client.IgnoreNotFound(err)).To(Succeed())
	})

	Context("NewClient", func() {

		It("should return a new JanitorImpl", func() {
			t := NewJanitorClient(NewScopedClient(k8sClient, environment))
			Expect(t).To(BeAssignableToTypeOf(&janitorClient{}))
		})

	})

	Context("Cleanup", func() {

		It("should update the state", func() {

			obj := templ.DeepCopy()

			_, err := jc.CreateOrUpdate(ctx, obj, DoNothing())
			Expect(err).NotTo(HaveOccurred())
			Expect(jc.state).To(HaveKeyWithValue(schema.GroupVersionKind{
				Group:   "testgroup.cp.ei.telekom.de",
				Version: "v1",
				Kind:    "TestResource",
			}, map[client.ObjectKey]bool{
				{Namespace: "default", Name: "test"}: true,
			}))

		})

		It("should create a resource and add it to the desired state", func() {
			obj := templ.DeepCopy()

			jc.Reset()

			res, err := jc.CreateOrUpdate(ctx, obj, DoNothing())
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(controllerutil.OperationResultCreated))

			deleted, err := jc.Cleanup(ctx, test.NewObjectList(), []client.ListOption{client.InNamespace(namespace)})
			Expect(err).ToNot(HaveOccurred())
			Expect(deleted).To(Equal(0))
		})

		It("should delete objects that are not desired", func() {
			undesiredObj := templ.DeepCopy()
			undesiredObj.SetName("undesired")
			Expect(k8sClient.Create(ctx, undesiredObj)).To(Succeed())

			obj := templ.DeepCopy()

			jc.Reset()

			res, err := jc.CreateOrUpdate(ctx, obj, DoNothing())
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(controllerutil.OperationResultCreated))

			deleted, err := jc.Cleanup(ctx, test.NewObjectList(), []client.ListOption{client.InNamespace(namespace)})
			Expect(err).ToNot(HaveOccurred())
			Expect(deleted).To(Equal(1))

			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(undesiredObj), undesiredObj))).To(BeTrue())
		})
	})

	Context("Wrap", func() {

		It("should delete object that were not created in the Wrap function", func() {
			undesiredObj := templ.DeepCopy()
			undesiredObj.SetName("undesired")
			Expect(k8sClient.Create(ctx, undesiredObj)).To(Succeed())

			obj := templ.DeepCopy()

			objList := test.NewObjectList()

			deleted, err := jc.Wrap(ctx, objList, []client.ListOption{client.InNamespace(namespace)}, func(c ScopedClient) bool {

				res, err := c.CreateOrUpdate(ctx, obj, DoNothing())
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(controllerutil.OperationResultCreated))

				return true
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(deleted).To(Equal(1))

			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(undesiredObj), undesiredObj))).To(BeTrue())
			Expect(objList.Items).To(HaveLen(1))
		})

		It("should not delete object that were created in the Wrap function", func() {
			undesiredObj := templ.DeepCopy()
			undesiredObj.SetName("undesired")
			Expect(k8sClient.Create(ctx, undesiredObj)).To(Succeed())

			obj := templ.DeepCopy()

			objList := test.NewObjectList()

			deleted, err := jc.Wrap(ctx, objList, []client.ListOption{client.InNamespace(namespace)}, func(c ScopedClient) bool {
				res, err := c.CreateOrUpdate(ctx, obj, DoNothing())
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(controllerutil.OperationResultCreated))
				return false
			})

			Expect(err.Error()).To(Equal("aborted by user"))
			Expect(deleted).To(Equal(-1))
			Expect(objList.Items).To(BeEmpty())

			// Cleanup
			Expect(k8sClient.Delete(ctx, undesiredObj)).To(Succeed())
		})
	})

	Context("CleanupAll", func() {

		It("should delete all objects", func() {
			undesiredObj := templ.DeepCopy()
			undesiredObj.SetName("undesired")
			Expect(k8sClient.Create(ctx, undesiredObj)).To(Succeed())
			unDesiredObj2 := templ.DeepCopy()
			unDesiredObj2.SetName("undesired2")
			Expect(k8sClient.Create(ctx, unDesiredObj2)).To(Succeed())

			obj := templ.DeepCopy()
			res, err := jc.CreateOrUpdate(ctx, obj, DoNothing())
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(controllerutil.OperationResultCreated))

			deleted, err := jc.CleanupAll(ctx, []client.ListOption{client.InNamespace(namespace)})
			Expect(err).ToNot(HaveOccurred())
			Expect(deleted).To(Equal(2))

			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(undesiredObj), undesiredObj))).To(BeTrue())
		})
	})

	Context("AddKnownTypeToState", func() {

		It("should add a known type to the state", func() {
			jc.Reset()
			jc.AddKnownTypeToState(templ.DeepCopy())
			Expect(jc.state).To(HaveKey(schema.GroupVersionKind{
				Group:   "testgroup.cp.ei.telekom.de",
				Version: "v1",
				Kind:    "TestResource",
			}))
		})
	})
})
