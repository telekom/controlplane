// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package informer_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/internal/informer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamic "k8s.io/client-go/dynamic/fake"
)

var _ = Describe("Informer", func() {

	var ctx context.Context
	var cancel context.CancelFunc
	gvr := schema.GroupVersionResource{
		Group:    "testgroup",
		Version:  "v1",
		Resource: "testobjects",
	}
	var eventHandler *mockEventHandler
	var mockClient *dynamic.FakeDynamicClient

	Context("Kubernetes Informer", Ordered, func() {

		BeforeEach(func() {
			ctx = logr.NewContext(context.Background(), GinkgoLogr)
			ctx, cancel = context.WithCancel(ctx)
			mockClient = dynamic.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
				gvr: "TestObjectList",
			})
			eventHandler = &mockEventHandler{}

		})

		It("should initialize a new Informer instance", func() {
			inf := informer.New(ctx, gvr, mockClient, eventHandler)
			Expect(inf).NotTo(BeNil())
		})

		It("should start the Informer", func() {
			inf := informer.New(ctx, gvr, mockClient, eventHandler)
			Expect(inf).NotTo(BeNil())
			err := inf.Start()
			Expect(err).ToNot(HaveOccurred())
			Eventually(func(g Gomega) {
				g.Expect(inf.Ready()).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("should stop the informer gracefully", func() {
			inf := informer.New(ctx, gvr, mockClient, eventHandler)
			Expect(inf).NotTo(BeNil())
			err := inf.Start()
			Expect(err).ToNot(HaveOccurred())
			cancel()

		})

		It("should handle events", func() {
			resourceClient := mockClient.Resource(gvr).Namespace("default")
			inf := informer.New(ctx, gvr, mockClient, eventHandler)
			Expect(inf).NotTo(BeNil())
			err := inf.Start()
			Expect(err).ToNot(HaveOccurred())
			obj := NewUnstructured("test")

			obj, err = resourceClient.Create(ctx, obj, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(eventHandler.Events()).To(HaveLen(1))
				g.Expect(eventHandler.Events()[0].Action).To(Equal("create"))
				g.Expect(eventHandler.Events()[0].Object).To(Equal(obj))
			}, timeout, interval).Should(Succeed())

			eventHandler.Reset()

			obj.SetLabels(map[string]string{"foo": "bar"})

			_, err = resourceClient.Update(ctx, obj, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(eventHandler.Events()).To(HaveLen(1))
				g.Expect(eventHandler.Events()[0].Action).To(Equal("update"))
			}, timeout, interval).Should(Succeed())

			eventHandler.Reset()

			err = resourceClient.Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(eventHandler.Events()).To(HaveLen(1))
				g.Expect(eventHandler.Events()[0].Action).To(Equal("delete"))
			}, timeout, interval).Should(Succeed())

		})

		It("should handle errors", func() {
			eventHandler.NextError = errors.New("FAKE_ERROR")
			inf := informer.New(ctx, gvr, mockClient, eventHandler)
			Expect(inf).NotTo(BeNil())
			err := inf.Start()
			Expect(err).ToNot(HaveOccurred())

			resourceClient := mockClient.Resource(gvr).Namespace("default")
			obj := NewUnstructured("test")
			_, err = resourceClient.Create(ctx, obj, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

		})
	})

})
