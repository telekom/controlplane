// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Store Suite")
}

var _ = Describe("Store", func() {

	Context("Enforce Prefix", func() {

		It("should overwrite the selected prefix if it does not match the prefix", func() {

			listOpts := store.NewListOpts()
			listOpts.Prefix = "my-cursor-foo"
			store.EnforcePrefix("my-prefix-foo", &listOpts)

			Expect(listOpts.Prefix).To(Equal("my-prefix-foo"))
		})

		It("should not overwrite the selected prefix if it matches the prefix", func() {

			listOpts := store.NewListOpts()
			listOpts.Prefix = "my-prefix-foo-bar"
			store.EnforcePrefix("my-prefix-foo", &listOpts)

			Expect(listOpts.Prefix).To(Equal("my-prefix-foo-bar"))
		})

		It("should set the prefix if none is set", func() {
			listOpts := store.NewListOpts()
			store.EnforcePrefix("my-prefix-foo", &listOpts)

			Expect(listOpts.Prefix).To(Equal("my-prefix-foo"))
		})

		It("should do nothing if no prefix is provided", func() {
			listOpts := store.NewListOpts()
			store.EnforcePrefix("", &listOpts)

			Expect(listOpts.Prefix).To(Equal(""))

			store.EnforcePrefix(nil, &listOpts)
			Expect(listOpts.Prefix).To(Equal(""))
		})
	})

	Context("Parse Limit", func() {

		It("should return the default limit if the limit is not set", func() {
			Expect(store.ParseLimit("")).To(Equal(store.DefaultPageSize))
		})

		It("should return the default limit if the limit is invalid", func() {
			By("Checking for negative values")
			Expect(store.ParseLimit("-1")).To(Equal(store.DefaultPageSize))
		})

		It("should return the limit if it is valid", func() {
			Expect(store.ParseLimit("10")).To(Equal(10))
		})
	})

	Context("Equal GVK", func() {

		It("should return true if the GVKs are equal", func() {
			a := schema.GroupVersionKind{
				Group:   "foo",
				Version: "v1",
				Kind:    "bar",
			}
			b := schema.GroupVersionKind{
				Group:   "foo",
				Version: "v1",
				Kind:    "bar",
			}
			Expect(store.EqualGVK(a, b)).To(Succeed())
		})

		It("should return false if the GVKs are not equal", func() {
			a := schema.GroupVersionKind{
				Group:   "foo",
				Version: "v1",
				Kind:    "bar",
			}
			b := schema.GroupVersionKind{
				Group:   "foo",
				Version: "v2",
				Kind:    "bar",
			}
			By("Checking for different versions")
			p := store.EqualGVK(a, b)
			Expect(p).To(HaveOccurred())
			Expect(problems.IsValidationError(p)).To(BeTrue())

			By("Checking for different groups")
			b.Version = "v1"
			b.Group = "bar"
			p = store.EqualGVK(a, b)
			Expect(p).To(HaveOccurred())
			Expect(problems.IsValidationError(p)).To(BeTrue())

			By("Checking for different kinds")
			b.Group = "foo"
			b.Kind = "baz"
			p = store.EqualGVK(a, b)
			Expect(p).To(HaveOccurred())
			Expect(problems.IsValidationError(p)).To(BeTrue())
		})
	})

	Context("UrlEncoded", func() {

		It("should encode the ListOptions", func() {
			opts := store.NewListOpts()
			Expect(opts.UrlEncoded()).To(Equal("prefix=&cursor=&limit=100"))
		})

		It("should encode empty ListOptions", func() {
			opts := store.ListOpts{}
			Expect(opts.UrlEncoded()).To(Equal("prefix=&cursor=&limit=0"))
		})

		It("should encode ListOptions with filters", func() {
			opts := store.NewListOpts()
			opts.Filters = []store.Filter{
				{
					Path:  "foo",
					Op:    store.OpEqual,
					Value: "bar",
				},
				{
					Path:  "baz",
					Op:    store.OpNotEqual,
					Value: "qux",
				},
			}

			Expect(opts.UrlEncoded()).To(Equal("prefix=&cursor=&limit=100&filter=foo==bar&filter=baz!=qux"))
		})

		It("should encode ListOptions with sorters", func() {
			opts := store.NewListOpts()

			opts.Sorters = []store.Sorter{
				{
					Path:  "foo",
					Order: store.SortOrderAsc,
				},
				{
					Path:  "bar",
					Order: store.SortOrderDesc,
				},
			}

			Expect(opts.UrlEncoded()).To(Equal("prefix=&cursor=&limit=100&sort=foo:asc&sort=bar:desc"))
		})
	})

	Context("ParseSorter", func() {

		It("should return a string representation of the sorter", func() {
			sorter := store.Sorter{
				Path:  "foo",
				Order: store.SortOrderAsc,
			}

			Expect(sorter.String()).To(Equal("foo:asc"))
		})

		It("should return an error if the sorter is invalid", func() {
			_, err := store.ParseSorter("foo")
			Expect(err).To(HaveOccurred())
		})

		It("should return an error if the order is invalid", func() {
			_, err := store.ParseSorter("foo:invalid")
			Expect(err).To(HaveOccurred())
		})

		It("should return a valid sorter (asc)", func() {
			sorter, err := store.ParseSorter("foo:asc")
			Expect(err).NotTo(HaveOccurred())
			Expect(sorter.Path).To(Equal("foo"))
			Expect(sorter.Order).To(Equal(store.SortOrderAsc))
		})

		It("should return a valid sorter (desc)", func() {
			sorter, err := store.ParseSorter("foo:desc")
			Expect(err).NotTo(HaveOccurred())
			Expect(sorter.Path).To(Equal("foo"))
			Expect(sorter.Order).To(Equal(store.SortOrderDesc))
		})

	})

	Context("ParseFilter", func() {

		It("should return a string representation of the filter", func() {
			filter := store.Filter{
				Path:  "foo",
				Op:    store.OpRegex,
				Value: "bar",
			}

			Expect(filter.String()).To(Equal("foo=~bar"))
		})

		It("should return an error if the filter is invalid", func() {
			_, err := store.ParseFilter("foo")
			Expect(err).To(HaveOccurred())
		})

		It("should return an error if the operator is invalid", func() {
			Expect(store.FilterOp("#@").IsValid()).To(BeFalse())
		})

		It("should return a valid filter (regex)", func() {
			filter, err := store.ParseFilter("foo=~bar")
			Expect(err).NotTo(HaveOccurred())
			Expect(filter.Path).To(Equal("foo"))
			Expect(filter.Op).To(Equal(store.OpRegex))
			Expect(filter.Value).To(Equal("bar"))
		})

		It("should return a valid filter (equal)", func() {
			filter, err := store.ParseFilter("foo==bar")
			Expect(err).NotTo(HaveOccurred())
			Expect(filter.Path).To(Equal("foo"))
			Expect(filter.Op).To(Equal(store.OpEqual))
			Expect(filter.Value).To(Equal("bar"))
		})

		It("should return a valid filter (not equal)", func() {
			filter, err := store.ParseFilter("foo!=bar")
			Expect(err).NotTo(HaveOccurred())
			Expect(filter.Path).To(Equal("foo"))
			Expect(filter.Op).To(Equal(store.OpNotEqual))
			Expect(filter.Value).To(Equal("bar"))
		})

		It("should return a valid filter (full text)", func() {
			filter, err := store.ParseFilter("foo~~bar")
			Expect(err).NotTo(HaveOccurred())
			Expect(filter.Path).To(Equal("foo"))
			Expect(filter.Op).To(Equal(store.OpFullText))
			Expect(filter.Value).To(Equal("bar"))
		})

	})

	Context("Patch", func() {
		It("should return an error if the operator is invalid", func() {
			Expect(store.PatchOp("#@").IsValid()).To(BeFalse())
		})

		It("should return no error if the operator is valid", func() {
			Expect(store.PatchOp("replace").IsValid()).To(BeTrue())
			Expect(store.PatchOp("replace").String()).To(Equal("replace"))
		})
	})
})
