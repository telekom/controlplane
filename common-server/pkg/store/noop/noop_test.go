// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package noop_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/noop"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("NoopStore", func() {
	var (
		s   store.ObjectStore[*unstructured.Unstructured]
		ctx context.Context
		gvr schema.GroupVersionResource
		gvk schema.GroupVersionKind
	)

	BeforeEach(func() {
		ctx = context.Background()
		gvr = schema.GroupVersionResource{
			Group:    "test.io",
			Version:  "v1",
			Resource: "things",
		}
		gvk = schema.GroupVersionKind{
			Group:   "test.io",
			Version: "v1",
			Kind:    "Thing",
		}
		s = noop.NewStore[*unstructured.Unstructured](gvr, gvk)
	})

	Context("Info", func() {
		It("should return the configured GVR and GVK", func() {
			gotGVR, gotGVK := s.Info()
			Expect(gotGVR).To(Equal(gvr))
			Expect(gotGVK).To(Equal(gvk))
		})
	})

	Context("Ready", func() {
		It("should always return true", func() {
			Expect(s.Ready()).To(BeTrue())
		})
	})

	Context("Get", func() {
		It("should return a NotFound error", func() {
			_, err := s.Get(ctx, "default", "my-thing")
			Expect(err).To(HaveOccurred())
			Expect(problems.IsNotFound(err)).To(BeTrue())
		})
	})

	Context("List", func() {
		It("should return an empty list", func() {
			resp, err := s.List(ctx, store.NewListOpts())
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.Items).To(BeEmpty())
		})
	})

	Context("Delete", func() {
		It("should return a NotFound error", func() {
			err := s.Delete(ctx, "default", "my-thing")
			Expect(err).To(HaveOccurred())
			Expect(problems.IsNotFound(err)).To(BeTrue())
		})
	})

	Context("CreateOrReplace", func() {
		It("should return a BadRequest error indicating the feature is disabled", func() {
			obj := &unstructured.Unstructured{}
			err := s.CreateOrReplace(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("disabled"))
		})
	})

	Context("Patch", func() {
		It("should return a BadRequest error indicating the feature is disabled", func() {
			_, err := s.Patch(ctx, "default", "my-thing", store.Patch{
				Path:  "/spec/foo",
				Op:    store.OpReplace,
				Value: "bar",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("disabled"))
		})
	})
})
