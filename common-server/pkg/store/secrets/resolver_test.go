// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package secrets_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/fake"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("Secrets Resolver", func() {

	var ctx context.Context
	var mockedSecretManager *fake.MockSecretManager
	var resolver secrets.Replacer

	BeforeEach(func() {
		ctx = context.Background()
		mockedSecretManager = fake.NewMockSecretManager(GinkgoT())
		resolver = &secrets.SecretManagerResolver{M: mockedSecretManager}
	})

	Context("Resolve from Bytes", func() {

		b := []byte(`{"root": "$<test:::mySecret:>", "sub": {"key": "$<test:::mySecret:>"}}`)

		It("should replace all secrets in a byte array", func() {
			mockedSecretManager.EXPECT().Get(ctx, "test:::mySecret:").Return("mySecretValue", nil).Times(2)
			result, err := resolver.ReplaceAll(ctx, b, []string{"root", "sub.key"})
			Expect(err).ToNot(HaveOccurred())
			b, ok := result.([]byte)
			Expect(ok).To(BeTrue())
			Expect(string(b)).To(Equal(`{"root": "mySecretValue", "sub": {"key": "mySecretValue"}}`))
		})

		It("should return an error if the secret is not found", func() {
			mockedSecretManager.EXPECT().Get(ctx, "test:::mySecret:").Return("", api.ErrNotFound).Times(1)
			result, err := resolver.ReplaceAll(ctx, b, []string{"root", "sub.key"})
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to get secret value"))
		})

		It("should also work with strings", func() {
			mockedSecretManager.EXPECT().Get(ctx, "test:::mySecret:").Return("mySecretValue", nil).Times(2)
			result, err := resolver.ReplaceAll(ctx, string(b), []string{"root", "sub.key"})
			Expect(err).ToNot(HaveOccurred())
			str, ok := result.(string)
			Expect(ok).To(BeTrue())
			Expect(str).To(Equal(`{"root": "mySecretValue", "sub": {"key": "mySecretValue"}}`))
		})

	})

	Context("Resolve from Map", func() {

		It("should replace all secrets in a map", func() {
			m := map[string]any{
				"root": "$<test:::mySecret:>",
				"sub":  map[string]any{"key": "$<test:::mySecret:>"},
			}

			mockedSecretManager.EXPECT().Get(ctx, "test:::mySecret:").Return("mySecretValue", nil).Times(2)
			result, err := resolver.ReplaceAll(ctx, m, []string{"root", "sub.key"})
			Expect(err).ToNot(HaveOccurred())
			resMap, ok := result.(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(resMap["root"]).To(Equal("mySecretValue"))
			Expect(resMap["sub"].(map[string]any)["key"]).To(Equal("mySecretValue"))
		})

		It("should return an error if the secret is not found", func() {
			m := map[string]any{
				"root": "$<test:::mySecret:>",
				"sub":  map[string]any{"key": "$<test:::mySecret:>"},
			}

			mockedSecretManager.EXPECT().Get(ctx, "test:::mySecret:").Return("", api.ErrNotFound).Times(1)
			result, err := resolver.ReplaceAll(ctx, m, []string{"root", "sub.key"})
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to get secret value"))
		})
	})

	Context("Resolve from Unstructured", func() {

		It("should replace all secrets in an unstructured object", func() {
			u := &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{
						"root": "$<test:::mySecret:>",
						"sub":  map[string]any{"key": "$<test:::mySecret:>"},
					},
				},
			}

			mockedSecretManager.EXPECT().Get(ctx, "test:::mySecret:").Return("mySecretValue", nil).Times(2)
			result, err := resolver.ReplaceAll(ctx, u, []string{"spec.root", "spec.sub.key"})
			Expect(err).ToNot(HaveOccurred())
			resUnstructured, ok := result.(*unstructured.Unstructured)
			Expect(ok).To(BeTrue())
			resMap := resUnstructured.UnstructuredContent()
			Expect(resMap["spec"].(map[string]any)["root"]).To(Equal("mySecretValue"))
			Expect(resMap["spec"].(map[string]any)["sub"].(map[string]any)["key"]).To(Equal("mySecretValue"))
		})

		It("should return an error if the secret is not found", func() {
			u := map[string]any{
				"root": "$<test:::mySecret:>",
				"sub":  map[string]any{"key": "$<test:::mySecret:>"},
			}

			mockedSecretManager.EXPECT().Get(ctx, "test:::mySecret:").Return("", api.ErrNotFound).Times(1)
			result, err := resolver.ReplaceAll(ctx, u, []string{"root", "sub.key"})
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to get secret value"))
		})
	})
})
