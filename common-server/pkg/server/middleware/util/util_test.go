package util_test

import (
	"testing"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUtil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Middleware Util Suite")
}

var _ = Describe("Middleware Util", func() {

	Context("GetValue", func() {
		It("should get a string value", func() {
			val, ok := util.GetValue[string](map[string]any{"key": "value"}, "key")
			Expect(ok).To(BeTrue())
			Expect(val).To(Equal("value"))
		})

		It("should get a int value", func() {
			val, ok := util.GetValue[int](map[string]any{"key": 1}, "key")
			Expect(ok).To(BeTrue())
			Expect(val).To(Equal(1))
		})

		It("should get a map value", func() {
			val, ok := util.GetValue[map[string]any](map[string]any{"key": map[string]any{"key": "value"}}, "key")
			Expect(ok).To(BeTrue())
			Expect(val).To(Equal(map[string]any{"key": "value"}))
		})

		It("should return false if key does not exist", func() {
			_, ok := util.GetValue[string](map[string]any{"key": "value"}, "key2")
			Expect(ok).To(BeFalse())
		})

		It("should return false when the value is nil", func() {
			_, ok := util.GetValue[string](map[string]any{"key": nil}, "key")
			Expect(ok).To(BeFalse())
		})
	})
})
