package filter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/store/inmemory/filter"
)

var _ = Describe("Matchers", func() {

	Context("Simple", func() {

		It("should return true if the value is equal", func() {
			simple := filter.NewSimple("foo")
			Expect(simple.Equal("foo")).To(BeTrue())
		})

		It("should return false if the value is not equal", func() {
			simple := filter.NewSimple("foo")
			Expect(simple.Equal("bar")).To(BeFalse())
		})

		It("should return false if the value is nil", func() {
			simple := filter.NewSimple("foo")
			Expect(simple.Equal(nil)).To(BeFalse())
		})

		It("should support equality for integers", func() {
			simple := filter.NewSimple("42")
			Expect(simple.Equal(42)).To(BeTrue())
		})

		It("should support equality for floats", func() {
			simple := filter.NewSimple("42.42")
			Expect(simple.Equal(42.42)).To(BeTrue())
		})

		It("should support equality for booleans", func() {
			simple := filter.NewSimple("true")
			Expect(simple.Equal(true)).To(BeTrue())
		})

		It("should support equality for slices", func() {
			simple := filter.NewSimple(`["foo"]`)
			Expect(simple.Equal([]string{"foo"})).To(BeTrue())
		})

		It("should support equality for maps", func() {
			simple := filter.NewSimple(`{"foo":"bar"}`)
			Expect(simple.Equal(map[string]string{"foo": "bar"})).To(BeTrue())
		})

	})

	Context("NotEq", func() {

		It("should return true if the value is not equal", func() {
			simple := filter.NewSimple("foo")
			not := filter.NotEq(simple)
			Expect(not.Equal("bar")).To(BeTrue())
		})

		It("should return false if the value is equal", func() {
			simple := filter.NewSimple("foo")
			not := filter.NotEq(simple)
			Expect(not.Equal("foo")).To(BeFalse())
		})

	})

	Context("Regex", func() {

		It("should return true if the value matches the regex", func() {
			regex := filter.NewRegex(`^foo$`)
			Expect(regex.Equal("foo")).To(BeTrue())
		})

		It("should return false if the value does not match the regex", func() {
			regex := filter.NewRegex(`^foo$`)
			Expect(regex.Equal("bar")).To(BeFalse())
		})

		It("should return false if the value is nil", func() {
			regex := filter.NewRegex(`^foo$`)
			Expect(regex.Equal(nil)).To(BeFalse())
		})

		It("should support regex for integers", func() {
			regex := filter.NewRegex(`^\d+$`)
			Expect(regex.Equal(42)).To(BeTrue())
		})

	})
})
