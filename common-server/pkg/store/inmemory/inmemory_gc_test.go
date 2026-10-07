// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package inmemory

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Value log GC", func() {
	Context("runValueLogGCLoop", func() {
		var (
			tick  chan time.Time
			calls atomic.Int32
			run   func() error
		)

		BeforeEach(func() {
			tick = make(chan time.Time)
			calls.Store(0)
			run = func() error {
				calls.Add(1)
				return nil
			}
		})

		startLoop := func(ctx context.Context, isClosed func() bool, fn func() error) <-chan struct{} {
			done := make(chan struct{})
			go func() {
				defer close(done)
				runValueLogGCLoop(ctx, tick, isClosed, fn, logr.Discard())
			}()
			return done
		}

		It("runs once per tick", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := startLoop(ctx, func() bool { return false }, run)

			tick <- time.Now()
			tick <- time.Now()
			Eventually(calls.Load).WithTimeout(time.Second).WithPolling(10 * time.Millisecond).Should(BeEquivalentTo(2))

			cancel()
			Eventually(done).WithTimeout(time.Second).WithPolling(10 * time.Millisecond).Should(BeClosed())
		})

		It("keeps running after run errors", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := startLoop(ctx, func() bool { return false }, func() error {
				calls.Add(1)
				return errors.New("boom")
			})

			tick <- time.Now()
			tick <- time.Now()
			Eventually(calls.Load).WithTimeout(time.Second).WithPolling(10 * time.Millisecond).Should(BeEquivalentTo(2))

			cancel()
			Eventually(done).WithTimeout(time.Second).WithPolling(10 * time.Millisecond).Should(BeClosed())
		})

		It("does not run when the context is already cancelled", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			buffered := make(chan time.Time, 1)
			buffered <- time.Now()
			done := make(chan struct{})
			go func() {
				defer close(done)
				runValueLogGCLoop(ctx, buffered, func() bool { return false }, run, logr.Discard())
			}()

			Eventually(done).WithTimeout(time.Second).WithPolling(10 * time.Millisecond).Should(BeClosed())
			Expect(calls.Load()).To(BeZero())
		})

		It("stops without running when the database is closed", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := startLoop(ctx, func() bool { return true }, run)

			tick <- time.Now()
			Eventually(done).WithTimeout(time.Second).WithPolling(10 * time.Millisecond).Should(BeClosed())
			Expect(calls.Load()).To(BeZero())
		})
	})

	Context("startValueLogGC", func() {
		It("does not start for in-memory databases", func() {
			db, err := badger.Open(newBadgerOptsInMemory().WithLogger(nil))
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(db.Close()).To(Succeed()) }()

			Expect(startValueLogGC(context.Background(), db, logr.Discard())).To(BeFalse())
		})

		It("starts for disk databases", func() {
			db, err := badger.Open(newBadgerOptsDisk(GinkgoT().TempDir()).WithLogger(nil))
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(db.Close()).To(Succeed()) }()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			Expect(startValueLogGC(ctx, db, logr.Discard())).To(BeTrue())
		})
	})

	Context("runValueLogGCOnce", func() {
		It("treats ErrNoRewrite as success on disk", func() {
			db, err := badger.Open(newBadgerOptsDisk(GinkgoT().TempDir()).WithLogger(nil))
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(db.Close()).To(Succeed()) }()

			Expect(errors.Is(db.RunValueLogGC(valueLogGCDiscardRatio), badger.ErrNoRewrite)).To(BeTrue())
			Expect(runValueLogGCOnce(db)).To(Succeed())
		})

		It("wraps unexpected errors", func() {
			db, err := badger.Open(newBadgerOptsInMemory().WithLogger(nil))
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(db.Close()).To(Succeed()) }()

			err = runValueLogGCOnce(db)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix("running value log GC: "))
		})
	})
})
