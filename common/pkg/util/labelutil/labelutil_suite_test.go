// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package labelutil

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLabelutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Labelutil Suite")
}

var _ = Describe("Labelutil", func() {

	Context("NormalizeValue", func() {

		It("should normalize value", func() {

			value := " foo/bar_baz/ "
			Expect(NormalizeValue(value)).To(Equal("foo-bar-baz"))

			value = "/foo/bar_baz\\"
			Expect(NormalizeValue(value)).To(Equal("foo-bar-baz"))
		})

		It("should normalize and shorten names", func() {
			// value string must be larger than the MaxNameLength after normalization
			value := `This is a very long value/with some_unwanted\\characters that needs to be normalized and shortened.
			But I have no ideas what else to add to make it even longer. 
			How long is it now? Let's see if this is enough. Still not enough, need to add a few more`

			Expect(len(value)).To(BeNumerically(">", MaxNameLength))
			shortenedValue := NormalizeNameValue(value)
			Expect(len(shortenedValue)).To(BeNumerically("<=", MaxNameLength))
			Expect(shortenedValue).To(Equal("this-is-a-very-l694579d4do-add-a-few-more"))
		})

		It("should not shorten names", func() {
			value := "This is a very long value/with some_unwanted\\characters that needs to be normalized and shortened"
			shortenedValue := NormalizeNameValue(value)
			Expect(shortenedValue).To(HaveLen(len(value)))
			Expect(shortenedValue).To(Equal("this-is-a-very-long-value-with-some-unwanted-characters-that-needs-to-be-normalized-and-shortened"))
		})

		It("should shorten labels", func() {
			value := "This is a very long value/with some_unwanted\\characters that needs to be normalized and shortened"
			shortenedValue := NormalizeLabelValue(value)
			Expect(len(shortenedValue)).To(BeNumerically("<=", MaxLabelLength))
			Expect(shortenedValue).To(Equal("this-is-a-very-l.679cb75c5.ed-and-shortened"))
		})

		It("should not shorten labels", func() {
			value := "short_value"
			shortenedValue := NormalizeLabelValue(value)
			Expect(shortenedValue).To(Equal("short-value"))
		})

	})
})
