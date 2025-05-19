// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package bouncer

import (
	"context"
	"sync"
	"time"
)

var _ Bouncer = &Locker{}

// Locker manages the locking mechnism for the bouncer.
// It locks based on a key and allows only one runnable to run at a time for that key.
type Locker struct {
	mu        sync.Mutex
	locks     map[string]chan struct{}
	queueName string
}

func NewDefaultLocker() *Locker {
	return &Locker{
		locks:     make(map[string]chan struct{}),
		queueName: "default",
	}
}

// Acquire the lock for the given key. This method will block until the lock is acquired.
// It will respect the lifecycle of the context and will return an error if the context is done.
// The acquired lock must be released by calling ReleaseLock.
func (l *Locker) AcquireLock(ctx context.Context, key string) error {
	l.mu.Lock()
	lock, ok := l.locks[key]
	if !ok {
		lock = make(chan struct{}, 1)
		l.locks[key] = lock
	}
	l.mu.Unlock()

	start := time.Now()
	select {
	case lock <- struct{}{}:
		timeInQueue.WithLabelValues(l.queueName, "acquired").Observe(time.Since(start).Seconds())
		return nil
	case <-ctx.Done():
		timeInQueue.WithLabelValues(l.queueName, "cancelled").Observe(time.Since(start).Seconds())
		return ErrLockNotAcquired
	}
}

// TryAcquireLock tries to acquire the lock for the given key.
// It will return immediately with an error if the lock is not acquired.
func (l *Locker) TryAcquireLock(ctx context.Context, key string) error {
	l.mu.Lock()
	lock, ok := l.locks[key]
	if !ok {
		lock = make(chan struct{}, 1)
		l.locks[key] = lock
	}
	l.mu.Unlock()

	start := time.Now()
	select {
	case lock <- struct{}{}:
		timeInQueue.WithLabelValues(l.queueName, "acquired").Observe(time.Since(start).Seconds())
		return nil
	default:
		timeInQueue.WithLabelValues(l.queueName, "cancelled").Observe(time.Since(start).Seconds())
		return ErrLockNotAcquired
	}
}

func (l *Locker) ReleaseLock(ctx context.Context, key string) {
	l.mu.Lock()
	lock, ok := l.locks[key]
	l.mu.Unlock()
	if !ok {
		return
	}
	select {
	case <-lock:
		return
	default:
		return
	}
}

func (l *Locker) RunB(ctx context.Context, key string, run Runnable) error {
	queueLength.WithLabelValues(l.queueName).Inc()
	defer queueLength.WithLabelValues(l.queueName).Dec()

	if err := l.AcquireLock(ctx, key); err != nil {
		return err
	}
	defer l.ReleaseLock(ctx, key)

	return run(ctx)
}
