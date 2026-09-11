// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package ctrlerrors

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
)

type BlockedError interface {
	error
	IsBlocked() bool
}

type RetryableError interface {
	error
	IsRetryable() bool
}

type RetryableWithDelayError interface {
	error
	RetryableError
	RetryDelay() time.Duration
}

// HandleError analyzes the given error and updates the object's conditions accordingly.
// It uses errors.As to unwrap the error chain, which supports both pkg/errors.Wrap and
// standard fmt.Errorf %w wrapping.
// It returns whether conditions were updated, a result for explicit delayed retries, and an error
// for controller-runtime's rate-limited retry queue.
func HandleError(ctx context.Context, obj types.Object, err error, recorder record.EventRecorder) (bool, reconcile.Result, error) {
	var be BlockedError
	if errors.As(err, &be) && be.IsBlocked() {
		log.FromContext(ctx).WithName("controller.error-handler").V(0).Info("Handling error", "reason", "Blocked", "error", be.Error())
		recordError(obj, be, "Blocked", recorder)
		updated := obj.SetCondition(condition.NewBlockedCondition(be.Error()))
		return updated, reconcile.Result{
			// Its blocked but we still want to recheck later
			// However, with the longer interval for normal requeues
			RequeueAfter: config.RequeueWithJitter(),
		}, nil
	}

	var rde RetryableWithDelayError
	if errors.As(err, &rde) {
		recordError(obj, rde, "Retryable", recorder)
		if rde.IsRetryable() {
			delay := rde.RetryDelay()
			if delay <= 0 {
				return false, reconcile.Result{}, err
			}
			log.FromContext(ctx).WithName("controller.error-handler").V(0).Info("Handling error", "reason", "Retryable", "error", rde.Error())
			return false, reconcile.Result{RequeueAfter: config.Jitter(delay)}, nil
		} else {
			log.FromContext(ctx).WithName("controller.error-handler").V(0).Info("Handling error", "reason", "Retryable", "error", rde.Error())
			return false, reconcile.Result{}, nil
		}
	}

	var re RetryableError
	if errors.As(err, &re) {
		recordError(obj, re, "Retryable", recorder)
		if re.IsRetryable() {
			return false, reconcile.Result{}, err
		} else {
			// Not retryable, treat as Blocked
			log.FromContext(ctx).WithName("controller.error-handler").V(0).Info("Handling error", "reason", "Retryable", "error", re.Error())
			return false, reconcile.Result{RequeueAfter: config.RequeueWithJitter()}, nil
		}
	}

	recordError(obj, err, "Unknown", recorder)
	return false, reconcile.Result{}, err
}

func recordError(obj types.Object, err error, reason string, recorder record.EventRecorder) {
	if err != nil && recorder != nil {
		recorder.Event(obj, "Warning", reason, err.Error())
	}
}

var (
	_ error                   = &CtrlError{}
	_ BlockedError            = &CtrlError{}
	_ RetryableError          = &CtrlError{}
	_ RetryableWithDelayError = &CtrlError{}
)

type CtrlError struct {
	msg        string
	retryable  bool
	retryDelay time.Duration
	blocked    bool
}

func BlockedErrorf(format string, a ...any) *CtrlError {
	return &CtrlError{
		msg:     fmt.Sprintf(format, a...),
		blocked: true,
	}
}

func RetryableErrorf(format string, a ...any) *CtrlError {
	return &CtrlError{
		msg:       fmt.Sprintf(format, a...),
		retryable: true,
	}
}

func RetryableWithDelayErrorf(delay time.Duration, format string, a ...any) *CtrlError {
	return &CtrlError{
		msg:        fmt.Sprintf(format, a...),
		retryable:  true,
		retryDelay: delay,
	}
}

func (e *CtrlError) Error() string {
	return e.msg
}

func (e *CtrlError) IsBlocked() bool {
	return e.blocked
}

func (e *CtrlError) IsRetryable() bool {
	return e.retryable
}

func (e *CtrlError) RetryDelay() time.Duration {
	return e.retryDelay
}
