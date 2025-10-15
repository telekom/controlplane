// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cache_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/cache"
	"github.com/telekom/controlplane/secret-manager/test/mocks"
)

var _ = Describe("Cached Backend", func() {

	Context("Cached Backend Implementation", func() {

		var mockBackend *mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]
		var cachedBackend *cache.CachedBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]

		BeforeEach(func() {
			t := GinkgoT()
			mockBackend = &mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]{}
			mockBackend.Test(t)
			t.Cleanup(func() { mockBackend.AssertExpectations(t) })
			cachedBackend = cache.NewCachedBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]](mockBackend, 10*time.Second)
		})

		It("should create a new cached backend", func() {
			backend := cache.NewCachedBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]](mockBackend, 10*time.Second)
			Expect(backend).ToNot(BeNil())
		})

		It("should parse secret ID", func() {
			rawSecretId := "my-secret-id"
			mockBackend.EXPECT().ParseSecretId(rawSecretId).Return(&mocks.MockSecretId{}, nil).Once()

			secretId, err := cachedBackend.ParseSecretId(rawSecretId)
			Expect(err).NotTo(HaveOccurred())
			Expect(secretId).ToNot(BeNil())
		})

		It("should get the secret on cache miss", func() {
			ctx := context.Background()

			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id")

			secret := backend.NewDefaultSecret(secretId, "my-value")
			mockBackend.EXPECT().Get(ctx, secretId).Return(secret, nil).Once()

			secret, err := cachedBackend.Get(ctx, secretId)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).ToNot(BeNil())

			num, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "cache_access_total")
			Expect(err).NotTo(HaveOccurred())
			Expect(num).To(Equal(1))
		})

		It("should set the secret and update the cache", func() {
			ctx := context.Background()

			secretValue := backend.String("my-value")
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id")
			secretId.EXPECT().Copy().Return(secretId).Once()

			mockBackend.EXPECT().Set(ctx, secretId, secretValue).Return(backend.NewDefaultSecret(secretId, "my-value"), nil).Once()

			secret, err := cachedBackend.Set(ctx, secretId, secretValue)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).ToNot(BeNil())
			Expect(secret.Value()).To(Equal("my-value"))
			Expect(secret.Id()).To(Equal(secretId))
		})

		It("should get the secret from cache", func() {
			ctx := context.Background()

			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id")

			secret := backend.NewDefaultSecret(secretId, "my-value")
			cachedItem := cache.NewDefaultCacheItem(secretId, secret, 10)
			cachedBackend.Cache.Set(secretId.String(), cachedItem)

			secret, err := cachedBackend.Get(ctx, secretId)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).ToNot(BeNil())
			Expect(secret.Value()).To(Equal("my-value"))
			Expect(secret.Id()).To(Equal(secretId))

			num, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "cache_access_total")
			Expect(err).NotTo(HaveOccurred())
			Expect(num).To(Equal(3))
		})

		It("should return an error if the backend fails", func() {

			ctx := context.Background()

			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id")

			mockBackend.EXPECT().Get(ctx, secretId).Return(backend.DefaultSecret[*mocks.MockSecretId]{}, backend.ErrSecretNotFound(secretId)).Once()

			res, err := cachedBackend.Get(ctx, secretId)
			Expect(err).To(HaveOccurred())
			Expect(res.Value()).To(BeEmpty())
			Expect(res.Id()).To(BeNil())
			Expect(backend.IsNotFoundErr(err)).To(BeTrue())

			num, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "cache_access_total")
			Expect(err).NotTo(HaveOccurred())
			Expect(num).To(Equal(3))
		})

		It("should return the cached item when the value did not change", func() {
			ctx := context.Background()
			value := "my-value"

			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id")
			secretId.EXPECT().Copy().Return(secretId).Once()

			secretValue := backend.String(value)

			cachedBackend.Cache.Set(secretId.String(), cache.NewDefaultCacheItem(secretId, backend.NewDefaultSecret(secretId, value), 10))

			res, err := cachedBackend.Set(ctx, secretId, secretValue)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.Value()).To(Equal(value))
			Expect(res.Id()).To(Equal(secretId))
		})

		It("should return an error if the backend fails to set the secret", func() {
			ctx := context.Background()
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id")
			secretId.EXPECT().Copy().Return(secretId).Once()

			mockBackend.EXPECT().Set(ctx, secretId, backend.String("my-value")).Return(backend.DefaultSecret[*mocks.MockSecretId]{}, backend.ErrInvalidSecretId("invalid-id")).Once()

			res, err := cachedBackend.Set(ctx, secretId, backend.String("my-value"))
			Expect(err).To(HaveOccurred())
			Expect(res.Value()).To(BeEmpty())
			Expect(res.Id()).To(BeNil())
			Expect(err).To(MatchError(backend.ErrInvalidSecretId("invalid-id")))
		})

		It("should delete the secret from the cache and backend", func() {
			ctx := context.Background()
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id")

			mockBackend.EXPECT().Delete(ctx, secretId).Return(nil).Once()

			err := cachedBackend.Delete(ctx, secretId)
			Expect(err).NotTo(HaveOccurred())
			Expect(cachedBackend.Cache.Get(secretId.String())).To(BeNil())

		})
	})
})
