// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("Client", func() {

	var (
		templ = test.NewObject(name, namespace)
	)

	Context("NewScopedClient", func() {
		It("should return a new ScopedClientImpl", func() {
			client := NewScopedClient(k8sClient, "test")
			Expect(client).To(BeAssignableToTypeOf(&scopedClientImpl{}))
		})

		It("should be able to cast to k8sClient", func() {
			scopedClient := NewScopedClient(k8sClient, "test")
			_, ok := scopedClient.(client.Client)
			Expect(ok).To(BeTrue())
		})
	})

	Context("ScopedClientImpl", func() {
		var (
			ctx          context.Context
			scopedClient *scopedClientImpl
		)

		BeforeEach(func() {
			scopedClient = &scopedClientImpl{
				Client:      k8sClient,
				environment: environment,
			}
			ctx = contextutil.WithEnv(context.Background(), environment)
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, &test.TestResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			})
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})

		Context("CreateOrUpdate", func() {
			It("should set the environment label", func() {
				obj := templ.DeepCopy()
				mutator := func() error {
					return nil
				}
				res, err := scopedClient.CreateOrUpdate(ctx, obj, mutator)
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(controllerutil.OperationResultCreated))
				Expect(obj.GetLabels()).To(HaveKeyWithValue(config.EnvironmentLabelKey, "test"))

			})

			It("should update the object", func() {
				scopedClient.Reset()

				obj := templ.DeepCopy()
				Expect(k8sClient.Create(ctx, obj)).To(Succeed())

				mutator := func() error {
					obj.SetLabels(map[string]string{
						config.BuildLabelKey("test"): "test",
					})
					return nil
				}

				res, err := scopedClient.CreateOrUpdate(ctx, obj, mutator)
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(controllerutil.OperationResultUpdated))
				Expect(obj.GetLabels()).To(HaveKeyWithValue(config.BuildLabelKey("test"), "test"))
				Expect(scopedClient.AnyChanged()).To(BeTrue())

			})

			It("should return an error if CreateOrUpdate fails", func() {

				obj := templ.DeepCopy()

				mutator := func() error {
					return errors.New("force error")
				}

				_, err := scopedClient.CreateOrUpdate(ctx, obj, mutator)
				Expect(err.Error()).To(Equal("failed to create or update object test: force error"))

			})
		})

		Context("Get", func() {

			AfterEach(func() {
				Expect(k8sClient.Delete(ctx, templ.DeepCopy())).To(Succeed())
			})

			It("should return an error if Get fails", func() {
				obj := templ.DeepCopy()

				Expect(k8sClient.Create(ctx, obj)).To(Succeed())

				err := scopedClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, obj)
				Expect(err.Error()).To(Equal("failed to get object: object does not have labels"))
			})

			It("should return an error if the object does not belong to the environment", func() {
				obj := templ.DeepCopy()
				obj.GetLabels()[config.EnvironmentLabelKey] = "other"

				Expect(k8sClient.Create(ctx, obj)).To(Succeed())

				err := scopedClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, obj)
				Expect(err.Error()).To(Equal("failed to get object: object does not belong to the environment"))
			})

			It("should return the object", func() {

				obj := templ.DeepCopy()
				obj.GetLabels()[config.EnvironmentLabelKey] = environment

				Expect(k8sClient.Create(ctx, obj)).To(Succeed())

				err := scopedClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj).ToNot(BeNil())

			})
		})

		Context("Delete", func() {

			AfterEach(func() {
				err := k8sClient.Delete(ctx, templ.DeepCopy())
				Expect(client.IgnoreNotFound(err)).To(Succeed())
			})

			It("should return an error if the object does not belong to the environment", func() {
				obj := templ.DeepCopy()
				obj.GetLabels()[config.EnvironmentLabelKey] = "other"

				Expect(k8sClient.Create(ctx, obj)).To(Succeed())

				err := scopedClient.Delete(ctx, obj)
				Expect(err.Error()).To(Equal("failed to delete object: failed to get object: object does not belong to the environment"))
			})

			It("should delete the object", func() {
				obj := templ.DeepCopy()
				obj.GetLabels()[config.EnvironmentLabelKey] = environment

				Expect(k8sClient.Create(ctx, obj)).To(Succeed())

				err := scopedClient.Delete(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
			})

		})

		Context("List", func() {

			AfterEach(func() {
				Expect(k8sClient.DeleteAllOf(ctx, &test.TestResource{}, client.InNamespace(namespace))).To(Succeed())
			})

			It("should only return objects that belong to the environment", func() {

				obj := templ.DeepCopy()
				obj.GetLabels()[config.EnvironmentLabelKey] = environment

				obj2 := templ.DeepCopy()
				obj2.SetName("test-2")
				obj2.GetLabels()[config.EnvironmentLabelKey] = "other"

				Expect(k8sClient.Create(ctx, obj)).To(Succeed())
				Expect(k8sClient.Create(ctx, obj2)).To(Succeed())

				list := &test.TestResourceList{}
				err := scopedClient.List(ctx, list)
				Expect(err).NotTo(HaveOccurred())
				Expect(list.Items).To(HaveLen(1))
				Expect(list.Items[0].Name).To(Equal("test"))

			})
		})
	})

	Context("CleanupState", func() {
		var (
			ctx          context.Context
			scopedClient *scopedClientImpl
		)

		BeforeEach(func() {
			scopedClient = &scopedClientImpl{
				Client:      k8sClient,
				environment: environment,
			}
			ctx = contextutil.WithEnv(context.Background(), environment)

			for i := 0; i < 10; i++ {
				copy := templ.DeepCopy()
				copy.Name = fmt.Sprintf("%s-%d", name, i)
				copy.GetLabels()[config.EnvironmentLabelKey] = environment
				Expect(k8sClient.Create(ctx, copy)).To(Succeed())
			}
		})

		AfterEach(func() {
			Expect(k8sClient.DeleteAllOf(ctx, &test.TestResource{}, client.InNamespace(namespace))).To(Succeed())
		})

		It("should delete all objects that are not in the desired state", func() {

			listOpts := []client.ListOption{}

			list := &test.TestResourceList{}

			desiredState := map[client.ObjectKey]bool{
				{Namespace: namespace, Name: "test-1"}: true,
				{Namespace: namespace, Name: "test-6"}: true,
			}

			deleted, err := cleanupState(ctx, scopedClient, listOpts, list, desiredState)
			Expect(err).NotTo(HaveOccurred())
			Expect(deleted).To(Equal(8))

			Expect(k8sClient.List(ctx, list)).To(Succeed())
			Expect(list.Items).To(HaveLen(2))
		})
	})

	Context("StateInfo", func() {

		var (
			ctx          context.Context
			scopedClient ScopedClient
			obj          *test.TestResource
		)

		AfterEach(func() {
			Expect(k8sClient.DeleteAllOf(ctx, &test.TestResource{}, client.InNamespace(namespace))).To(Succeed())
		})

		BeforeEach(func() {
			ctx = context.Background()
			scopedClient = NewScopedClient(k8sClient, environment)
			obj = templ.DeepCopy()
			obj.SetLabels(map[string]string{
				config.EnvironmentLabelKey: environment,
			})
		})

		It("should return ready when all resources are ready", func() {
			_, err := scopedClient.CreateOrUpdate(ctx, obj, DoNothing())
			Expect(err).ToNot(HaveOccurred())

			scopedClient.Reset()

			obj.SetCondition(condition.NewReadyCondition("Test", "test"))
			Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed())
			_, err = scopedClient.CreateOrUpdate(ctx, obj, DoNothing())
			Expect(err).ToNot(HaveOccurred())

			// Expect all resources to be ready
			Expect(scopedClient.AllReady()).To(BeTrue())

			notReadyObj := templ.DeepCopy()
			_, err = scopedClient.CreateOrUpdate(ctx, notReadyObj, DoNothing())
			Expect(err).ToNot(HaveOccurred())
			notReadyObj.SetCondition(condition.NewNotReadyCondition("Test", "test"))
			Expect(k8sClient.Status().Update(ctx, notReadyObj)).To(Succeed())
			_, err = scopedClient.CreateOrUpdate(ctx, obj, DoNothing())
			Expect(err).ToNot(HaveOccurred())

			// Expect one resource to be not ready
			Expect(scopedClient.AllReady()).To(BeFalse())
		})

		It("should return failed when atleast one resource is failed", func() {
			_, err := scopedClient.CreateOrUpdate(ctx, obj, DoNothing())
			Expect(err).ToNot(HaveOccurred())

			obj.SetCondition(condition.NewNotReadyCondition("ReadyTest", "test"))
			Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed())

			scopedClient.Reset()
			_, err = scopedClient.CreateOrUpdate(ctx, obj, DoNothing())
			Expect(err).ToNot(HaveOccurred())

			Expect(scopedClient.AllReady()).To(BeFalse())
		})

		It("should return changed when atleast one resource is changed", func() {
			scopedClient.Reset()
			_, err := scopedClient.CreateOrUpdate(ctx, obj, DoNothing())
			Expect(err).ToNot(HaveOccurred())

			Expect(scopedClient.AnyChanged()).To(BeTrue())
			// defaults as no conditions set
			Expect(scopedClient.AllReady()).To(BeTrue())
		})
	})
})
