// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

type StatusHandler interface {
	Status(ctx context.Context, name string) (types.ObjectStatus, error)
}

type StatusEvalFunc func(ctx context.Context, status types.ObjectStatus) (continuePolling bool, err error)

var defaultStatusEvalFunc StatusEvalFunc = func(_ context.Context, status types.ObjectStatus) (continuePolling bool, err error) {
	switch status.GetOverallStatus() {
	case types.OverallStatusFailed:
		return false, fmt.Errorf("resource processing failed")
	case types.OverallStatusComplete, types.OverallStatusDone:
		return false, nil
	case types.OverallStatusBlocked:
		return false, nil // system cannot progress without external action
	default:
		return true, nil // processing, pending, none, invalid → keep polling
	}
}

type StatusPoller struct {
	logger   logr.Logger
	handler  StatusHandler
	timeout  time.Duration
	interval time.Duration
	evalFunc StatusEvalFunc
}

func NewStatusPoller(handler StatusHandler, evalFunc StatusEvalFunc, timeout, interval time.Duration) *StatusPoller {
	if evalFunc == nil {
		evalFunc = defaultStatusEvalFunc
	}
	return &StatusPoller{
		logger:   log.L().WithName("status-poller"),
		handler:  handler,
		evalFunc: evalFunc,
		timeout:  timeout,
		interval: interval,
	}
}

func (p *StatusPoller) Start(ctx context.Context, name string) (types.ObjectStatus, error) {

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	var lastStatus types.ObjectStatus

	for {
		select {
		case <-ctx.Done():
			return lastStatus, ctx.Err()
		case <-ticker.C:
			status, err := p.handler.Status(ctx, name)
			if err != nil {
				return lastStatus, err
			}
			lastStatus = status
			continuePolling, err := p.evalFunc(ctx, status)
			if err != nil {
				return lastStatus, err
			}
			if !continuePolling {
				return status, nil
			}
		}
	}
}
