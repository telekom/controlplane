// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package ctrlerrors

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
// It returns a boolean indicating whether the object's conditions were updated and a reconcile.Result
// that suggests whether to requeue the reconciliation and after what duration.
func HandleError(obj types.Object, err error, recorder record.EventRecorder) (bool, reconcile.Result) {
	err = errors.Cause(err)

	if be, ok := err.(BlockedError); ok && be.IsBlocked() {
		recordError(obj, err, "Blocked", recorder)
		updatd := obj.SetCondition(condition.NewBlockedCondition(err.Error()))
		return updatd, reconcile.Result{
			// Its blocked but we still want to recheck later
			// However, with the longer interval for normal requeues
			RequeueAfter: config.RequeueWithJitter(),
		}
	}

	if re, ok := err.(RetryableWithDelayError); ok {
		recordError(obj, err, "Retryable", recorder)
		if re.IsRetryable() {
			deley := re.RetryDelay()
			if deley <= 0 {
				deley = config.RetryWithJitterOnError()
			}
			return false, reconcile.Result{RequeueAfter: config.Jitter(deley)}
		} else {
			return false, reconcile.Result{}
		}
	}

	if re, ok := err.(RetryableError); ok {
		recordError(obj, err, "Retryable", recorder)
		if re.IsRetryable() {
			return false, reconcile.Result{RequeueAfter: config.RetryWithJitterOnError()}
		} else {
			return false, reconcile.Result{}
		}
	}

	recordError(obj, err, "Unknown", recorder)
	return false, reconcile.Result{RequeueAfter: config.RetryWithJitterOnError()}
}

func recordError(obj types.Object, err error, reason string, recorder record.EventRecorder) {
	if err != nil && recorder != nil {
		recorder.Event(obj, "Warning", reason, err.Error())
	}
}

var _ error = &CtrlError{}
var _ BlockedError = &CtrlError{}
var _ RetryableError = &CtrlError{}
var _ RetryableWithDelayError = &CtrlError{}

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
