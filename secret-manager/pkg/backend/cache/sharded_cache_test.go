// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cache_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/cache"
	"github.com/telekom/controlplane/secret-manager/test/mocks"
)

var _ = Describe("Sharded Cache", func() {

	Context("Sharded Cache Implementation", func() {
		It("should create a new sharded cache", func() {
			shardedCache := cache.NewShardedCache[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]](10)
			Expect(shardedCache).ToNot(BeNil())
		})

		It("should panic if the shard count is 0", func() {
			Expect(func() {
				cache.NewShardedCache[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]](0)
			}).To(Panic())
		})

		It("should set and get values from the sharded cache", func() {
			shardedCache := cache.NewShardedCache[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]](10)

			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id").Times(2)
			item := backend.NewDefaultSecret[*mocks.MockSecretId](secretId, "my-value")

			shardedCache.Set(secretId.String(), cache.NewDefaultCacheItem(secretId, item, 10))

			cachedItem, ok := shardedCache.Get(secretId.String())
			Expect(ok).To(BeTrue())
			Expect(cachedItem).ToNot(BeNil())
			Expect(cachedItem.Value()).To(Equal(item))
			Expect(cachedItem.Id()).To(Equal(secretId))
		})

		It("should return false if the item is not found in the sharded cache", func() {
			shardedCache := cache.NewShardedCache[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]](10)

			cachedItem, ok := shardedCache.Get("non-existing-id")
			Expect(ok).To(BeFalse())
			Expect(cachedItem).To(BeNil())
		})

		It("should delete the item from the sharded cache", func() {
			shardedCache := cache.NewShardedCache[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]](10)

			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id").Times(4)
			item := backend.NewDefaultSecret[*mocks.MockSecretId](secretId, "my-value")

			shardedCache.Set(secretId.String(), cache.NewDefaultCacheItem(secretId, item, 10))

			cachedItem, ok := shardedCache.Get(secretId.String())
			Expect(ok).To(BeTrue())
			Expect(cachedItem).ToNot(BeNil())

			shardedCache.Delete(secretId.String())

			cachedItem, ok = shardedCache.Get(secretId.String())
			Expect(ok).To(BeFalse())
			Expect(cachedItem).To(BeNil())
		})
	})
})
