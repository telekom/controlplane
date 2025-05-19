package patch_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/inmemory/patch"
)

func TestPatch(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Patch Suite")
}

var _ = Describe("Patches", func() {

	Context("Building patch funcs", func() {

		It("should build the patch funcs from the store patches", func() {
			patches := []store.Patch{
				{
					Path:  "foo",
					Op:    store.OpAdd,
					Value: "baz",
				},
				{
					Path:  "baz",
					Op:    store.OpReplace,
					Value: "qux",
				},
				{
					Path: "quux",
					Op:   store.OpRemove,
				},
			}

			patchFunc := patch.NewPatchFuncs(patches)

			data, err := patchFunc([]byte(`{"foo": ["bar"], "baz": "old", "quux": "value"}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(MatchJSON(`{"foo": ["bar", "baz"], "baz": "qux", "quux": null}`))
		})

		It("should return an error if the patch operation is not supported", func() {

			patchFunc := patch.NewPatchFuncs([]store.Patch{
				{
					Op: store.OpMove,
				},
			})

			_, err := patchFunc([]byte(`{"foo": "bar"}`))
			Expect(err).To(HaveOccurred())
		})
	})

	Context("NopPatch", func() {

		It("should return the data unchanged", func() {
			patchFunc := patch.NopPatch

			data := []byte(`{"foo": "bar"}`)
			result, err := patchFunc(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(data))
		})
	})

	Context("AddPatch", func() {

		It("should add a value to an array", func() {
			patchFunc := patch.AddPatch("foo", "baz")

			data, err := patchFunc([]byte(`{"foo": ["bar"]}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(MatchJSON(`{"foo": ["bar", "baz"]}`))
		})

		It("should create an array if the path does not exist", func() {
			patchFunc := patch.AddPatch("foo", "baz")

			data, err := patchFunc([]byte(`{}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(MatchJSON(`{"foo": ["baz"]}`))
		})

		It("should return an error if the path exists and is not an array", func() {
			patchFunc := patch.AddPatch("foo", "baz")

			_, err := patchFunc([]byte(`{"foo": "bar"}`))
			Expect(err).To(HaveOccurred())
		})
	})

	Context("RemovePatch", func() {

		It("should remove a value", func() {
			patchFunc := patch.RemovePatch("foo")

			data, err := patchFunc([]byte(`{"foo": "bar"}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(MatchJSON(`{"foo": null}`))
		})

		It("should remove an array", func() {
			patchFunc := patch.RemovePatch("foo")

			data, err := patchFunc([]byte(`{"foo": ["bar"]}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(MatchJSON(`{"foo": null}`))
		})
	})

	Context("ReplacePatch", func() {

		It("should replace a value", func() {
			patchFunc := patch.ReplacePatch("foo", "baz")

			data, err := patchFunc([]byte(`{"foo": "bar"}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(MatchJSON(`{"foo": "baz"}`))
		})

		It("should replace numeric values", func() {
			patchFunc := patch.ReplacePatch("foo", 42)

			data, err := patchFunc([]byte(`{"foo": 23}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(MatchJSON(`{"foo": 42}`))
		})

		It("should replace an array", func() {
			patchFunc := patch.ReplacePatch("foo", []string{"baz"})

			data, err := patchFunc([]byte(`{"foo": ["bar"]}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(MatchJSON(`{"foo": ["baz"]}`))
		})

		It("should replace an array with a map", func() {
			patchFunc := patch.ReplacePatch("foo", map[string]interface{}{"baz": "qux"})

			data, err := patchFunc([]byte(`{"foo": ["bar"]}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(MatchJSON(`{"foo": {"baz": "qux"}}`))
		})

		It("should create an array if the path does not exist", func() {
			patchFunc := patch.ReplacePatch("foo", "baz")

			data, err := patchFunc([]byte(`{}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(MatchJSON(`{"foo": "baz"}`))
		})

		It("should return an error if the expected type does not match", func() {
			patchFunc := patch.ReplacePatch("foo", "baz")

			_, err := patchFunc([]byte(`{"foo": ["bar"]}`))
			Expect(err).To(HaveOccurred())
		})
	})

})
