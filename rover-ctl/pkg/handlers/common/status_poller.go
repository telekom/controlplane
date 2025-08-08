// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"context"
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
	if status.GetProcessingState() == "done" {
		return false, nil
	}
	return true, nil
}

type StatusPoller struct {
	logger   logr.Logger
	handler  StatusHandler
	timeout  time.Duration
	interval time.Duration
	evalFunc StatusEvalFunc
}

func NewStatusPoller(handler StatusHandler, evalFunc StatusEvalFunc) *StatusPoller {
	if evalFunc == nil {
		evalFunc = defaultStatusEvalFunc
	}
	return &StatusPoller{
		logger:   log.L().WithName("status-poller"),
		handler:  handler,
		evalFunc: evalFunc,
		timeout:  30 * time.Second,
		interval: 2 * time.Second,
	}
}

func (p *StatusPoller) Start(ctx context.Context, name string) (types.ObjectStatus, error) {

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			status, err := p.handler.Status(ctx, name)
			if err != nil {
				return nil, err
			}
			continuePolling, err := p.evalFunc(ctx, status)
			if err != nil {
				return nil, err
			}
			if !continuePolling {
				return status, nil
			}
		}
	}
}
