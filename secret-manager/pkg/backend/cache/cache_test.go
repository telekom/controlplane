// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cache_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/cache"
	"github.com/telekom/controlplane/secret-manager/test/mocks"
)

var _ = Describe("Cache", func() {
	BeforeEach(func() {

	})

	Context("Cache Item", func() {
		It("should create a new cache item", func() {
			id := &mocks.MockSecretId{}
			item := backend.NewDefaultSecret[*mocks.MockSecretId](id, "my-value")
			cacheItem := cache.NewDefaultCacheItem(id, item, 10)
			Expect(cacheItem).ToNot(BeNil())
			Expect(cacheItem.Value()).To(Equal(item))
			Expect(cacheItem.Expired()).To(BeFalse())
		})
	})

	Context("Simple Cache Implementation", func() {

		var simpleCache cache.Cache[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]

		BeforeEach(func() {
			simpleCache = cache.NewSimpleCache[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]()
		})
		It("should set the value in the cache", func() {
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id").Times(2)
			item := backend.NewDefaultSecret[*mocks.MockSecretId](secretId, "my-value")

			simpleCache.Set(secretId.String(), cache.NewDefaultCacheItem(secretId, item, 10))

			cachedItem, ok := simpleCache.Get(secretId.String())
			Expect(ok).To(BeTrue())
			Expect(cachedItem).ToNot(BeNil())
			Expect(cachedItem.Value()).To(Equal(item))
			Expect(cachedItem.Id()).To(Equal(secretId))
		})

		It("should return false if the item is not found", func() {
			cachedItem, ok := simpleCache.Get("non-existing-id")
			Expect(ok).To(BeFalse())
			Expect(cachedItem).To(BeNil())
		})

		It("should delete the item from the cache", func() {
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id").Times(4)
			item := backend.NewDefaultSecret[*mocks.MockSecretId](secretId, "my-value")

			simpleCache.Set(secretId.String(), cache.NewDefaultCacheItem(secretId, item, 10))
			_, ok := simpleCache.Get(secretId.String())
			Expect(ok).To(BeTrue())

			simpleCache.Delete(secretId.String())
			_, ok = simpleCache.Get(secretId.String())
			Expect(ok).To(BeFalse())
		})

		It("should return false if the item is expired", func() {
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id").Times(2)
			item := backend.NewDefaultSecret[*mocks.MockSecretId](secretId, "my-value")

			simpleCache.Set(secretId.String(), cache.NewDefaultCacheItem(secretId, item, 0))

			time.Sleep(time.Second)

			cachedItem, ok := simpleCache.Get(secretId.String())
			Expect(ok).To(BeFalse())
			Expect(cachedItem).To(BeNil())
		})

	})
})
