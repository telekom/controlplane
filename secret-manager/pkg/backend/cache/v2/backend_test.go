// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v2_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	v2 "github.com/telekom/controlplane/secret-manager/pkg/backend/cache/v2"
	"github.com/telekom/controlplane/secret-manager/test/mocks"
)

var _ = Describe("Cached Backend V2", func() {

	Context("Cached Backend V2 Implementation", func() {

		var mockBackend *mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]
		var cachedBackend *v2.CachedBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]

		BeforeEach(func() {
			t := GinkgoT()
			mockBackend = &mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]{}
			mockBackend.Test(t)
			t.Cleanup(func() { mockBackend.AssertExpectations(t) })
			cachedBackend = v2.NewCachedBackend(mockBackend, 10*time.Second)
		})

		It("should create a new cached backend", func() {
			b := v2.NewCachedBackend(mockBackend, 10*time.Second)
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
			secretId.EXPECT().String().Return("my-secret-id")

			secret := backend.NewDefaultSecret(secretId, "my-value")
			mockBackend.EXPECT().Get(ctx, secretId).Return(secret, nil).Once()

			result, err := cachedBackend.Get(ctx, secretId)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Value()).To(Equal("my-value"))
		})

		It("should return an error if the backend fails", func() {
			ctx := context.Background()

			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id")

			mockBackend.EXPECT().Get(ctx, secretId).Return(backend.DefaultSecret[*mocks.MockSecretId]{}, backend.ErrSecretNotFound(secretId)).Once()

			res, err := cachedBackend.Get(ctx, secretId)
			Expect(err).To(HaveOccurred())
			Expect(res.Value()).To(BeEmpty())
			Expect(backend.IsNotFoundErr(err)).To(BeTrue())
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

		It("should delete the secret from cache and backend", func() {
			ctx := context.Background()
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id")

			mockBackend.EXPECT().Delete(ctx, secretId).Return(nil).Once()

			err := cachedBackend.Delete(ctx, secretId)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Singleflight deduplication", func() {

		It("should deduplicate concurrent Get requests for the same key", func() {
			ctx := context.Background()
			const numGoroutines = 10

			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("my-secret-id")

			mockBackend := &mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]{}
			mockBackend.Test(GinkgoT())

			cachedBackend := v2.NewCachedBackend(mockBackend, 10*time.Second)

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
			secretId.EXPECT().String().Return("err-secret-id")

			mockBackend := &mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]{}
			mockBackend.Test(GinkgoT())

			cachedBackend := v2.NewCachedBackend(mockBackend, 10*time.Second)

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
			secretId.EXPECT().String().Return("forget-secret-id")
			secretId.EXPECT().Copy().Return(secretId).Maybe()

			mockBackend := &mocks.MockBackend[*mocks.MockSecretId, backend.DefaultSecret[*mocks.MockSecretId]]{}
			mockBackend.Test(GinkgoT())

			cachedBackend := v2.NewCachedBackend(mockBackend, 10*time.Second)

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
	})
})
