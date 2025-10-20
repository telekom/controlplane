// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package ctrlerrors

import (
	"fmt"
	"time"

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

func HandleError(obj types.Object, err error, recorder record.EventRecorder) reconcile.Result {
	if err == nil {
		return reconcile.Result{}
	}

	if be, ok := err.(BlockedError); ok && be.IsBlocked() {
		obj.SetCondition(condition.NewBlockedCondition(err.Error()))
		return reconcile.Result{
			// Its blocked but we still want to recheck later
			// However, with the longer interval for normal requeues
			RequeueAfter: config.RequeueWithJitter(),
		}
	}

	if re, ok := err.(RetryableWithDelayError); ok {
		if re.IsRetryable() {
			deley := re.RetryDelay()
			if deley <= 0 {
				deley = config.RetryWithJitterOnError()
			}
			return reconcile.Result{RequeueAfter: config.Jitter(deley)}
		} else {
			return reconcile.Result{}
		}
	}

	if re, ok := err.(RetryableError); ok {
		if re.IsRetryable() {
			return reconcile.Result{RequeueAfter: config.RetryWithJitterOnError()}
		} else {
			return reconcile.Result{}
		}
	}

	if recorder != nil {
		recorder.Event(obj, "Warning", "UnknownError", err.Error())
	}

	return reconcile.Result{RequeueAfter: config.RetryWithJitterOnError()}
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
