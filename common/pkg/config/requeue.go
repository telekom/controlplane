// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"math/rand/v2"
	"time"
)

func RetryNWithJitterOnError(attempt int) time.Duration {
	return ExponentialBackoffWithJitter(RequeueAfterOnError, attempt)
}

// RetryWithJitterOnError returns a duration used in result.RequeueAfter.
// Its a wrapper around `RetryNWithJitterOnError(0)`.
// This is indented to be used in the controller's Reconcile function.
//
// ! in case of error
func RetryWithJitterOnError() time.Duration {
	return RetryNWithJitterOnError(0)
}

// RequeueWithJitter returns a duration used in result.RequeueAfter.
// This is indented to be used in the controller's Reconcile function.
//
// ! in case of success
func RequeueWithJitter() time.Duration {
	return Jitter(RequeueAfter)
}

func ExponentialBackoffWithJitter(base time.Duration, attempt int) time.Duration {
	backoff := base * (1 << attempt)
	if backoff > MaxBackoff {
		backoff = MaxBackoff
	}
	return Jitter(backoff)
}

// Jitter adds a random factor to the duration.
// This is used to prevent the thundering herd problem. See https://en.wikipedia.org/wiki/Thundering_herd_problem
func Jitter(d time.Duration) time.Duration {
	// #nosec G404 -- This is not a cryptographic use case
	return time.Duration(float64(d) * (1 + JitterFactor*rand.Float64()))
}
