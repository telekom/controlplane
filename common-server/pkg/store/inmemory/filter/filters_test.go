// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filter_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/inmemory/filter"
)

func TestFilter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Filter Suite")
}

var _ = Describe("Filters", func() {

	Context("Building filter funcs", func() {

		It("should build the filter funcs from the store filters", func() {
			filters := []store.Filter{
				{
					Path:  "foo",
					Op:    store.OpEqual,
					Value: "bar",
				},
				{
					Path:  "baz",
					Op:    store.OpRegex,
					Value: `^q\w{2}$`,
				},
				{
					Path:  "qux",
					Op:    store.OpFullText,
					Value: "hell",
				},
				{
					Path:  "qux",
					Op:    store.OpNotEqual,
					Value: "foo",
				},
				{
					Path: "qux",
				},
			}

			filterFunc := filter.NewFilterFuncs(filters)

			Expect(filterFunc([]byte(`{"foo": "bar", "baz": "qux", "qux": "hello world"}`))).To(BeTrue())
		})
	})

	Context("JsonPathFilter", func() {
		It("should return true for NopFilter", func() {
			Expect(filter.NopFilter([]byte("data"))).To(BeTrue())
		})

		It("should return false if the path does not exist", func() {
			filterFunc := filter.JsonPathFilter("foo")
			Expect(filterFunc([]byte(`{"bar": "baz"}`))).To(BeFalse())
		})

		It("should return false if the path is an empty array", func() {
			filterFunc := filter.JsonPathFilter("foo")
			Expect(filterFunc([]byte(`{"foo": []}`))).To(BeFalse())
		})

		It("should return true if the path exists", func() {
			filterFunc := filter.JsonPathFilter("foo")
			Expect(filterFunc([]byte(`{"foo": "bar"}`))).To(BeTrue())
		})
	})

	Context("FullTextFilter", func() {

		It("should return true if the text is contained in the data", func() {
			filterFunc := filter.FullTextFilter("bar")
			Expect(filterFunc([]byte(`{"foo": "bar"}`))).To(BeTrue())
		})

		It("should return false if the text is not contained in the data", func() {
			filterFunc := filter.FullTextFilter("bar")
			Expect(filterFunc([]byte(`{"foo": "baz"}`))).To(BeFalse())
		})
	})

	Context("JsonPathFilterValue", func() {

		It("should return false if the path does not exist", func() {
			filterFunc := filter.JsonPathFilterValue("foo", nil)
			Expect(filterFunc([]byte(`{"bar": "baz"}`))).To(BeFalse())
		})

		It("should return false if the path is an empty array", func() {
			filterFunc := filter.JsonPathFilterValue("foo", nil)
			Expect(filterFunc([]byte(`{"foo": []}`))).To(BeFalse())
		})

		It("should return true if the path exists", func() {
			filterFunc := filter.JsonPathFilterValue("foo", nil)
			Expect(filterFunc([]byte(`{"foo": "bar"}`))).To(BeTrue())
		})

		It("should return true if the path exists and the value is equal", func() {
			filterFunc := filter.JsonPathFilterValue("foo", filter.NewSimple("bar"))
			Expect(filterFunc([]byte(`{"foo": "bar"}`))).To(BeTrue())
		})

		It("should return false if the path exists and the value is not equal", func() {
			filterFunc := filter.JsonPathFilterValue("foo", filter.NewSimple("bar"))
			Expect(filterFunc([]byte(`{"foo": "baz"}`))).To(BeFalse())
		})
	})

	Context("Or", func() {

		It("should return true if any of the filters return true", func() {
			filterFunc := filter.Or(
				filter.JsonPathFilter("foo"),
				filter.JsonPathFilter("bar"),
			)
			Expect(filterFunc([]byte(`{"foo": "baz"}`))).To(BeTrue())
		})

		It("should return false if all of the filters return false", func() {
			filterFunc := filter.Or(
				filter.JsonPathFilter("foo"),
				filter.JsonPathFilter("bar"),
			)
			Expect(filterFunc([]byte(`{"baz": "qux"}`))).To(BeFalse())
		})
	})

	Context("And", func() {

		It("should return true if all of the filters return true", func() {
			filterFunc := filter.And(
				filter.JsonPathFilter("foo"),
				filter.JsonPathFilter("bar"),
			)
			Expect(filterFunc([]byte(`{"foo": "baz", "bar": "qux"}`))).To(BeTrue())
		})

		It("should return false if any of the filters return false", func() {
			filterFunc := filter.And(
				filter.JsonPathFilter("foo"),
				filter.JsonPathFilter("bar"),
			)
			Expect(filterFunc([]byte(`{"foo": "baz"}`))).To(BeFalse())
		})
	})

	Context("Not", func() {

		It("should return true if the filter returns false", func() {
			filterFunc := filter.Not(filter.JsonPathFilter("foo"))
			Expect(filterFunc([]byte(`{"bar": "baz"}`))).To(BeTrue())
		})

		It("should return false if the filter returns true", func() {
			filterFunc := filter.Not(filter.JsonPathFilter("foo"))
			Expect(filterFunc([]byte(`{"foo": "bar"}`))).To(BeFalse())
		})
	})

})
