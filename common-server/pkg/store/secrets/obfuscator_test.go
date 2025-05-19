package secrets_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("Secrets Obfuscator", func() {

	var ctx context.Context
	var obfuscator secrets.Replacer

	BeforeEach(func() {
		ctx = context.Background()
		obfuscator = secrets.NewObfuscator()
	})

	Context("Obfuscate from Bytes", func() {

		b := []byte(`{"root": "mySecretValue", "sub": {"key": "mySecretValue"}}`)

		It("should obfuscate all secrets in a byte array", func() {
			result, err := obfuscator.ReplaceAll(ctx, b, []string{"root", "sub.key"})
			Expect(err).ToNot(HaveOccurred())
			b, ok := result.([]byte)
			Expect(ok).To(BeTrue())
			Expect(string(b)).To(Equal(`{"root": "**********", "sub": {"key": "**********"}}`))
		})

		It("should also work with strings", func() {
			result, err := obfuscator.ReplaceAll(ctx, string(b), []string{"root", "sub.key"})
			Expect(err).ToNot(HaveOccurred())
			str, ok := result.(string)
			Expect(ok).To(BeTrue())
			Expect(str).To(Equal(`{"root": "**********", "sub": {"key": "**********"}}`))
		})
	})

	Context("Obfuscate from Map", func() {

		It("should obfuscate all secrets in a map", func() {
			m := map[string]any{
				"root": "mySecretValue",
				"sub":  map[string]any{"key": "mySecretValue"},
			}

			result, err := obfuscator.ReplaceAll(ctx, m, []string{"root", "sub.key"})
			Expect(err).ToNot(HaveOccurred())
			resMap, ok := result.(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(resMap["root"]).To(Equal("**********"))
			Expect(resMap["sub"].(map[string]any)["key"]).To(Equal("**********"))
		})
	})

	Context("Obfuscate from Unstructured", func() {

		It("should obfuscate all secrets in an unstructured object", func() {
			u := &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{
						"root": "mySecretValue",
						"sub":  map[string]any{"key": "mySecretValue"},
					},
				},
			}

			result, err := obfuscator.ReplaceAll(ctx, u, []string{"spec.root", "spec.sub.key"})
			Expect(err).ToNot(HaveOccurred())
			resUnstructured, ok := result.(*unstructured.Unstructured)
			Expect(ok).To(BeTrue())
			resMap := resUnstructured.UnstructuredContent()
			Expect(resMap["spec"].(map[string]any)["root"]).To(Equal("**********"))
			Expect(resMap["spec"].(map[string]any)["sub"].(map[string]any)["key"]).To(Equal("**********"))
		})
	})
})
