// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cache_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	cache "github.com/telekom/controlplane/secret-manager/pkg/backend/cache"
	"github.com/telekom/controlplane/secret-manager/test/mocks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cached Backend cache", func() {
	Context("Cached Backend cache Implementation", func() {
		var mockBackend *mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]
		var cachedBackend *cache.CachedBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]

		BeforeEach(func() {
			t := GinkgoT()
			mockBackend = &mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]{}
			mockBackend.Test(t)
			t.Cleanup(func() { mockBackend.AssertExpectations(t) })
			cachedBackend = cache.NewCachedBackend(mockBackend, cache.WithTTL(10*time.Second))
		})

		It("should create a new cached backend", func() {
			b := cache.NewCachedBackend(mockBackend, cache.WithTTL(10*time.Second))
			Expect(b).ToNot(BeNil())
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
			secretId.EXPECT().CacheKey().Return("my-secret-id")
			secretId.EXPECT().String().Return("my-secret-id").Maybe()

			secret := backend.NewDefaultSecret(secretId, "my-value")
			mockBackend.On("Get", mock.Anything, secretId).Return(secret, nil).Once()

			result, err := cachedBackend.Get(ctx, secretId)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Value()).To(Equal("my-value"))
		})

		It("should return an error if the backend fails", func() {
			ctx := context.Background()

			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().CacheKey().Return("my-secret-id")
			secretId.EXPECT().String().Return("my-secret-id").Maybe()

			mockBackend.On("Get", mock.Anything, secretId).Return(backend.DefaultSecret[*mocks.MockSecretId]{}, backend.ErrSecretNotFound(secretId)).Once()

			res, err := cachedBackend.Get(ctx, secretId)
			Expect(err).To(HaveOccurred())
			Expect(res.Value()).To(BeEmpty())
			Expect(backend.IsNotFoundErr(err)).To(BeTrue())
		})

		It("should set the secret and update the cache", func() {
			ctx := context.Background()

			secretValue := backend.String("my-value")
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().CacheKey().Return("my-secret-id")
			secretId.EXPECT().String().Return("my-secret-id").Maybe()
			secretId.EXPECT().Copy().Return(secretId).Once()
			secretId.EXPECT().SubPath().Return("")

			mockBackend.EXPECT().Set(ctx, secretId, secretValue).Return(backend.NewDefaultSecret(secretId, "my-value"), nil).Once()

			secret, err := cachedBackend.Set(ctx, secretId, secretValue)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).ToNot(BeNil())
			Expect(secret.Value()).To(Equal("my-value"))
			Expect(secret.Id()).To(Equal(secretId))
		})

		It("should delete the secret from cache and backend", func() {
			ctx := context.Background()
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().CacheKey().Return("my-secret-id")
			secretId.EXPECT().String().Return("my-secret-id").Maybe()
			secretId.EXPECT().SubPath().Return("")

			mockBackend.EXPECT().Delete(ctx, secretId).Return(nil).Once()

			err := cachedBackend.Delete(ctx, secretId)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return a BackendError when setting an empty secret value", func() {
			ctx := context.Background()
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().CacheKey().Return("empty-value-id")
			secretId.EXPECT().String().Return("empty-value-id").Maybe()
			secretId.EXPECT().Copy().Return(secretId).Once()

			emptyValue := backend.String("")

			_, err := cachedBackend.Set(ctx, secretId, emptyValue)
			Expect(err).To(HaveOccurred())
			Expect(backend.IsBackendError(err)).To(BeTrue(), "error should be a BackendError")

			var backendErr *backend.BackendError
			Expect(errors.As(err, &backendErr)).To(BeTrue())
			Expect(backendErr.Code()).To(Equal(400))
		})

		It("should report cache size in bytes via CacheSizeBytes", func() {
			Expect(cachedBackend.CacheSizeBytes()).To(Equal(float64(0)), "empty cache should report 0 bytes")

			ctx := context.Background()
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().CacheKey().Return("size-test-id")
			secretId.EXPECT().String().Return("size-test-id").Maybe()
			secretId.EXPECT().Copy().Return(secretId).Once()
			secretId.EXPECT().SubPath().Return("")

			secretValue := backend.String("some-secret-value")
			mockBackend.EXPECT().Set(ctx, secretId, secretValue).
				Return(backend.NewDefaultSecret(secretId, "some-secret-value"), nil).Once()

			_, err := cachedBackend.Set(ctx, secretId, secretValue)
			Expect(err).NotTo(HaveOccurred())

			// Ristretto is eventually consistent; Wait ensures buffered sets are applied
			cachedBackend.Cache.Wait()

			Expect(cachedBackend.CacheSizeBytes()).To(BeNumerically(">", 0), "cache should report non-zero bytes after Set")
		})
	})

	Context("Singleflight deduplication", func() {
		It("should deduplicate concurrent Get requests for the same key", func() {
			ctx := context.Background()
			const numGoroutines = 10

			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().CacheKey().Return("my-secret-id")
			secretId.EXPECT().String().Return("my-secret-id").Maybe()

			mockBackend := &mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]{}
			mockBackend.Test(GinkgoT())

			cachedBackend := cache.NewCachedBackend(mockBackend, cache.WithTTL(10*time.Second))

			// Use a channel to block the backend call until all goroutines are waiting
			block := make(chan struct{})
			var callCount atomic.Int32

			mockBackend.On("Get", mock.Anything, mock.Anything).
				Return(func(_ context.Context, _ *mocks.MockSecretId) backend.DefaultSecret[*mocks.MockSecretId] {
					callCount.Add(1)
					<-block
					return backend.NewDefaultSecret(secretId, "my-value")
				}, func(_ context.Context, _ *mocks.MockSecretId) error {
					return nil
				})

			var wg sync.WaitGroup
			results := make([]backend.DefaultSecret[*mocks.MockSecretId], numGoroutines)
			errs := make([]error, numGoroutines)

			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(idx int) {
					defer GinkgoRecover()
					defer wg.Done()
					results[idx], errs[idx] = cachedBackend.Get(ctx, secretId)
				}(i)
			}

			// Give goroutines time to enter singleflight
			time.Sleep(50 * time.Millisecond)
			close(block) // Release the backend call
			wg.Wait()

			// The backend should have been called exactly once
			Expect(callCount.Load()).To(Equal(int32(1)),
				fmt.Sprintf("expected backend.Get to be called once, but was called %d times", callCount.Load()))

			for i := 0; i < numGoroutines; i++ {
				Expect(errs[i]).NotTo(HaveOccurred())
				Expect(results[i].Value()).To(Equal("my-value"))
			}
		})

		It("should share errors across deduplicated Get requests", func() {
			ctx := context.Background()
			const numGoroutines = 5

			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().CacheKey().Return("err-secret-id")
			secretId.EXPECT().String().Return("err-secret-id").Maybe()

			mockBackend := &mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]{}
			mockBackend.Test(GinkgoT())

			cachedBackend := cache.NewCachedBackend(mockBackend, cache.WithTTL(10*time.Second))

			block := make(chan struct{})
			var callCount atomic.Int32

			mockBackend.On("Get", mock.Anything, mock.Anything).
				Return(func(_ context.Context, _ *mocks.MockSecretId) backend.DefaultSecret[*mocks.MockSecretId] {
					callCount.Add(1)
					<-block
					return backend.DefaultSecret[*mocks.MockSecretId]{}
				}, func(_ context.Context, _ *mocks.MockSecretId) error {
					return backend.ErrSecretNotFound(secretId)
				})

			var wg sync.WaitGroup
			errs := make([]error, numGoroutines)

			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(idx int) {
					defer GinkgoRecover()
					defer wg.Done()
					_, errs[idx] = cachedBackend.Get(ctx, secretId)
				}(i)
			}

			time.Sleep(50 * time.Millisecond)
			close(block)
			wg.Wait()

			Expect(callCount.Load()).To(Equal(int32(1)))

			for i := 0; i < numGoroutines; i++ {
				Expect(errs[i]).To(HaveOccurred())
				Expect(backend.IsNotFoundErr(errs[i])).To(BeTrue())
			}
		})

		It("should allow new backend calls after Forget via Delete", func() {
			ctx := context.Background()

			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().CacheKey().Return("forget-secret-id")
			secretId.EXPECT().String().Return("forget-secret-id").Maybe()
			secretId.EXPECT().SubPath().Return("").Maybe()
			secretId.EXPECT().Copy().Return(secretId).Maybe()

			mockBackend := &mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]{}
			mockBackend.Test(GinkgoT())

			cachedBackend := cache.NewCachedBackend(mockBackend, cache.WithTTL(10*time.Second))

			// First Get populates via backend
			mockBackend.On("Get", mock.Anything, mock.Anything).
				Return(backend.NewDefaultSecret(secretId, "value-1"), nil).Once()

			result, err := cachedBackend.Get(ctx, secretId)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Value()).To(Equal("value-1"))

			// Delete invalidates singleflight + cache
			mockBackend.On("Delete", mock.Anything, mock.Anything).Return(nil).Once()
			err = cachedBackend.Delete(ctx, secretId)
			Expect(err).NotTo(HaveOccurred())

			// Next Get should hit the backend again (not serve stale data)
			mockBackend.On("Get", mock.Anything, mock.Anything).
				Return(backend.NewDefaultSecret(secretId, "value-2"), nil).Once()

			// Ristretto is eventually consistent; wait briefly for the Del to propagate
			time.Sleep(10 * time.Millisecond)

			result, err = cachedBackend.Get(ctx, secretId)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Value()).To(Equal("value-2"))
		})

		It("should not fail other callers when first caller's context is cancelled", func() {
			const numWaiters = 5

			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().CacheKey().Return("ctx-cancel-id")
			secretId.EXPECT().String().Return("ctx-cancel-id").Maybe()

			mockBackend := &mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]{}
			mockBackend.Test(GinkgoT())

			cachedBackend := cache.NewCachedBackend(mockBackend, cache.WithTTL(10*time.Second))

			// Block the backend call until we release it
			block := make(chan struct{})
			var callCount atomic.Int32

			mockBackend.On("Get", mock.Anything, mock.Anything).
				Return(func(_ context.Context, _ *mocks.MockSecretId) backend.DefaultSecret[*mocks.MockSecretId] {
					callCount.Add(1)
					<-block
					return backend.NewDefaultSecret(secretId, "the-value")
				}, func(_ context.Context, _ *mocks.MockSecretId) error {
					return nil
				})

			// First caller: use a cancellable context and cancel it
			cancelCtx, cancel := context.WithCancel(context.Background())

			var wg sync.WaitGroup
			results := make([]backend.DefaultSecret[*mocks.MockSecretId], numWaiters+1)
			errs := make([]error, numWaiters+1)

			// Launch the first caller (will be cancelled)
			wg.Add(1)
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				results[0], errs[0] = cachedBackend.Get(cancelCtx, secretId)
			}()

			// Launch additional waiters with non-cancellable contexts
			for i := 1; i <= numWaiters; i++ {
				wg.Add(1)
				go func(idx int) {
					defer GinkgoRecover()
					defer wg.Done()
					results[idx], errs[idx] = cachedBackend.Get(context.Background(), secretId)
				}(i)
			}

			// Give goroutines time to enter singleflight
			time.Sleep(50 * time.Millisecond)

			// Cancel the first caller's context before releasing the backend
			cancel()
			time.Sleep(10 * time.Millisecond)

			// Release the backend call
			close(block)
			wg.Wait()

			// The backend should have been called exactly once (singleflight dedup)
			Expect(callCount.Load()).To(Equal(int32(1)))

			// All callers (including the cancelled one) should succeed because
			// context.WithoutCancel is used inside singleflight
			for i := 0; i <= numWaiters; i++ {
				Expect(errs[i]).NotTo(HaveOccurred(),
					fmt.Sprintf("caller %d should not have failed", i))
				Expect(results[i].Value()).To(Equal("the-value"),
					fmt.Sprintf("caller %d should have received the value", i))
			}
		})
	})

	Context("Sub-secret cache invalidation", func() {
		It("should invalidate parent cache entry when setting a sub-secret", func() {
			ctx := context.Background()

			// Create the parent secret ID
			parentSecretId := mocks.NewMockSecretId(GinkgoT())
			parentSecretId.EXPECT().CacheKey().Return("env:team:app:externalSecrets")
			parentSecretId.EXPECT().String().Return("env:team:app:externalSecrets:").Maybe()

			// Create the sub-secret ID
			subSecretId := mocks.NewMockSecretId(GinkgoT())
			subSecretId.EXPECT().CacheKey().Return("env:team:app:externalSecrets/key1")
			subSecretId.EXPECT().String().Return("env:team:app:externalSecrets/key1:abc123").Maybe()
			subSecretId.EXPECT().Copy().Return(subSecretId).Once()
			subSecretId.EXPECT().SubPath().Return("key1")
			subSecretId.EXPECT().ParentId().Return(parentSecretId)

			mockBackend := &mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]{}
			mockBackend.Test(GinkgoT())
			GinkgoT().Cleanup(func() { mockBackend.AssertExpectations(GinkgoT()) })

			cachedBackend := cache.NewCachedBackend(mockBackend, cache.WithTTL(10*time.Second))

			// Pre-populate the parent's cache entry
			parentSecret := backend.NewDefaultSecret(parentSecretId, `{"key1":"old-value","key2":"value2"}`)
			cachedBackend.Cache.SetWithTTL("env:team:app:externalSecrets", parentSecret, 100, 10*time.Second)
			cachedBackend.Cache.Wait()

			// Verify parent is in cache
			_, found := cachedBackend.Cache.Get("env:team:app:externalSecrets")
			Expect(found).To(BeTrue(), "parent should be in cache before sub-secret Set")

			// Set the sub-secret value
			subSecretValue := backend.String("new-value")
			mockBackend.EXPECT().Set(ctx, subSecretId, subSecretValue).
				Return(backend.NewDefaultSecret(subSecretId, "new-value"), nil).Once()

			_, err := cachedBackend.Set(ctx, subSecretId, subSecretValue)
			Expect(err).NotTo(HaveOccurred())

			// Ristretto is eventually consistent
			cachedBackend.Cache.Wait()

			// Parent cache entry should have been invalidated
			_, found = cachedBackend.Cache.Get("env:team:app:externalSecrets")
			Expect(found).To(BeFalse(), "parent cache entry should be invalidated after sub-secret Set")
		})

		It("should invalidate parent cache entry when deleting a sub-secret", func() {
			ctx := context.Background()

			// Create the parent secret ID
			parentSecretId := mocks.NewMockSecretId(GinkgoT())
			parentSecretId.EXPECT().CacheKey().Return("env:team:app:externalSecrets")
			parentSecretId.EXPECT().String().Return("env:team:app:externalSecrets:").Maybe()

			// Create the sub-secret ID
			subSecretId := mocks.NewMockSecretId(GinkgoT())
			subSecretId.EXPECT().CacheKey().Return("env:team:app:externalSecrets/key1")
			subSecretId.EXPECT().String().Return("env:team:app:externalSecrets/key1:abc123").Maybe()
			subSecretId.EXPECT().SubPath().Return("key1")
			subSecretId.EXPECT().ParentId().Return(parentSecretId)

			mockBackend := &mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]{}
			mockBackend.Test(GinkgoT())
			GinkgoT().Cleanup(func() { mockBackend.AssertExpectations(GinkgoT()) })

			cachedBackend := cache.NewCachedBackend(mockBackend, cache.WithTTL(10*time.Second))

			// Pre-populate the parent's cache entry
			parentSecret := backend.NewDefaultSecret(parentSecretId, `{"key1":"value1","key2":"value2"}`)
			cachedBackend.Cache.SetWithTTL("env:team:app:externalSecrets", parentSecret, 100, 10*time.Second)
			cachedBackend.Cache.Wait()

			// Verify parent is in cache
			_, found := cachedBackend.Cache.Get("env:team:app:externalSecrets")
			Expect(found).To(BeTrue(), "parent should be in cache before sub-secret Delete")

			// Delete the sub-secret
			mockBackend.EXPECT().Delete(ctx, subSecretId).Return(nil).Once()

			err := cachedBackend.Delete(ctx, subSecretId)
			Expect(err).NotTo(HaveOccurred())

			// Ristretto is eventually consistent
			cachedBackend.Cache.Wait()

			// Parent cache entry should have been invalidated
			_, found = cachedBackend.Cache.Get("env:team:app:externalSecrets")
			Expect(found).To(BeFalse(), "parent cache entry should be invalidated after sub-secret Delete")
		})

		It("should use stable cache key regardless of checksum", func() {
			ctx := context.Background()

			// Two secret IDs with different checksums but same logical identity
			secretId1 := mocks.NewMockSecretId(GinkgoT())
			secretId1.EXPECT().CacheKey().Return("env:team:app:path")
			secretId1.EXPECT().String().Return("env:team:app:path:checksum1").Maybe()

			secretId2 := mocks.NewMockSecretId(GinkgoT())
			secretId2.EXPECT().CacheKey().Return("env:team:app:path")
			secretId2.EXPECT().String().Return("env:team:app:path:checksum2").Maybe()

			mockBackend := &mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]{}
			mockBackend.Test(GinkgoT())
			GinkgoT().Cleanup(func() { mockBackend.AssertExpectations(GinkgoT()) })

			cachedBackend := cache.NewCachedBackend(mockBackend, cache.WithTTL(10*time.Second))

			// First Get with secretId1 populates the cache
			mockBackend.On("Get", mock.Anything, secretId1).
				Return(backend.NewDefaultSecret(secretId1, "my-value"), nil).Once()

			result1, err := cachedBackend.Get(ctx, secretId1)
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.Value()).To(Equal("my-value"))

			// Ristretto is eventually consistent
			cachedBackend.Cache.Wait()

			// Second Get with secretId2 (different checksum, same CacheKey) should hit cache
			result2, err := cachedBackend.Get(ctx, secretId2)
			Expect(err).NotTo(HaveOccurred())
			Expect(result2.Value()).To(Equal("my-value"))
		})
	})
})
