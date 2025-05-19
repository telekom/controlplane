// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package bouncer_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur/bouncer"
)

var _ = Describe("Locker", func() {
	BeforeEach(func() {

	})

	Context("Handling the lock lifecycle", func() {

		var ctx context.Context

		BeforeEach(func() {
			ctx = context.Background()
		})

		It("should acquire the lock", func() {
			locker := bouncer.NewDefaultLocker()

			err := locker.AcquireLock(ctx, "test")
			Expect(err).ToNot(HaveOccurred())
			locker.ReleaseLock(ctx, "test")
		})

		It("should not acquire the lock if already held", func() {
			locker := bouncer.NewDefaultLocker()

			ctxT, cancel := context.WithTimeout(ctx, 1)
			defer cancel()

			err := locker.AcquireLock(ctx, "test")
			Expect(err).ToNot(HaveOccurred())
			err = locker.AcquireLock(ctxT, "test")
			Expect(errors.Is(err, bouncer.ErrLockNotAcquired)).To(BeTrue())
			Expect(err.Error()).To(Equal("lock not acquired"))
		})

		It("should release the lock", func() {
			locker := bouncer.NewDefaultLocker()

			locker.ReleaseLock(ctx, "test")
		})

		It("should try to acquire the lock", func() {
			locker := bouncer.NewDefaultLocker()

			err := locker.TryAcquireLock(ctx, "test")
			Expect(err).ToNot(HaveOccurred())
			locker.ReleaseLock(ctx, "test")
		})

		It("should fail trying to acquire the lock if already held", func() {
			locker := bouncer.NewDefaultLocker()

			err := locker.AcquireLock(ctx, "test")
			Expect(err).ToNot(HaveOccurred())
			err = locker.TryAcquireLock(ctx, "test")
			Expect(err.Error()).To(Equal("lock not acquired"))
		})

		It("should run a runnable with the lock", func() {
			locker := bouncer.NewDefaultLocker()

			runnable := func(ctx context.Context) error {
				return nil
			}

			err := locker.RunB(ctx, "test", runnable)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail to run a runnable if the lock is not acquired", func() {
			locker := bouncer.NewDefaultLocker()

			err := locker.AcquireLock(ctx, "test")
			Expect(err).ToNot(HaveOccurred())

			runnable := func(ctx context.Context) error {
				return nil
			}

			ctxT, cancel := context.WithTimeout(ctx, 1)
			defer cancel()

			err = locker.RunB(ctxT, "test", runnable)
			Expect(errors.Is(err, bouncer.ErrLockNotAcquired)).To(BeTrue())
		})

		It("should run enqueued runnables in order", func() {
			locker := bouncer.NewDefaultLocker()

			var completedAt1 time.Time
			var completedAt2 time.Time

			runnable1 := func(ctx context.Context) error {
				completedAt1 = time.Now()
				return nil
			}

			runnable2 := func(ctx context.Context) error {
				completedAt2 = time.Now()
				return nil
			}

			err := locker.RunB(ctx, "test", runnable1)
			Expect(err).ToNot(HaveOccurred())

			err = locker.RunB(ctx, "test", runnable2)
			Expect(err).ToNot(HaveOccurred())

			Expect(completedAt1).To(BeTemporally("~", completedAt2, 1*time.Second))
		})

	})
})
