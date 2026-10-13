// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package inmemory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/go-logr/logr"
)

const (
	// valueLogGCInterval is the fixed maintenance interval between value log GC attempts.
	valueLogGCInterval = 5 * time.Minute
	// valueLogGCDiscardRatio is the minimum discardable fraction of a value log file to rewrite it.
	valueLogGCDiscardRatio = 0.5
)

// startValueLogGC starts periodic value log GC for disk-backed databases.
// It stops when ctx is cancelled or the database is closed.
// It returns false without starting anything for in-memory databases.
func startValueLogGC(ctx context.Context, db *badger.DB, log logr.Logger) bool {
	if db.Opts().InMemory {
		return false
	}
	ticker := time.NewTicker(valueLogGCInterval)
	go func() {
		defer ticker.Stop()
		runValueLogGCLoop(ctx, ticker.C, db.IsClosed, func() error { return runValueLogGCOnce(db) }, log)
	}()
	return true
}

// runValueLogGCLoop runs one GC attempt per tick until ctx is cancelled or isClosed reports true.
func runValueLogGCLoop(ctx context.Context, tick <-chan time.Time, isClosed func() bool, run func() error, log logr.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			// A tick and a cancellation can be ready at the same time; select picks randomly.
			if ctx.Err() != nil || isClosed() {
				return
			}
			if err := run(); err != nil {
				log.Error(err, "value log GC failed")
			}
		}
	}
}

// runValueLogGCOnce runs a single bounded value log GC attempt.
// badger.ErrNoRewrite means nothing was eligible and is not an error.
func runValueLogGCOnce(db *badger.DB) error {
	err := db.RunValueLogGC(valueLogGCDiscardRatio)
	if err == nil || errors.Is(err, badger.ErrNoRewrite) {
		return nil
	}
	return fmt.Errorf("running value log GC: %w", err)
}
