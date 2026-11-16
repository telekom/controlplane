// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	readAttempts        = 3
	readRetryInitial    = 100 * time.Millisecond
	readRetryMaximum    = time.Second
	readRetryMultiplier = 2
	readRetryJitter     = 0.2
)

type retryingReadAPI struct {
	ConjurAPI
	sleep func(time.Duration)
}

// NewRetryingReadAPI retries transient failures of Conjur read operations.
func NewRetryingReadAPI(client ConjurAPI) ConjurAPI {
	return &retryingReadAPI{ConjurAPI: client, sleep: time.Sleep}
}

func (c *retryingReadAPI) RetrieveSecret(variableID string) ([]byte, error) {
	return retryRead(c.sleep, func() ([]byte, error) {
		return c.ConjurAPI.RetrieveSecret(variableID)
	})
}

func (c *retryingReadAPI) RetrieveBatchSecrets(variableIDs []string) (map[string][]byte, error) {
	return retryRead(c.sleep, func() (map[string][]byte, error) {
		return c.ConjurAPI.RetrieveBatchSecrets(variableIDs)
	})
}

func (c *retryingReadAPI) RetrieveSecretWithVersion(variableID string, version int) ([]byte, error) {
	return retryRead(c.sleep, func() ([]byte, error) {
		return c.ConjurAPI.RetrieveSecretWithVersion(variableID, version)
	})
}

func retryRead[T any](sleep func(time.Duration), read func() (T, error)) (T, error) {
	backoff := wait.Backoff{
		Duration: readRetryInitial,
		Factor:   readRetryMultiplier,
		Jitter:   readRetryJitter,
		Steps:    readAttempts - 1,
		Cap:      readRetryMaximum,
	}
	for attempt := 1; ; attempt++ {
		result, err := read()
		if err == nil || attempt == readAttempts || !isRetryableReadError(err) {
			return result, err
		}
		sleep(min(backoff.Step(), readRetryMaximum))
	}
}

func isRetryableReadError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if conjurErr, ok := AsError(err); ok {
		return conjurErr.Code == http.StatusRequestTimeout ||
			conjurErr.Code == http.StatusTooManyRequests
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return !dnsErr.IsNotFound && (dnsErr.IsTimeout || dnsErr.IsTemporary)
	}
	if errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNABORTED) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
