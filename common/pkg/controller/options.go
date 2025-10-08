// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"
)

type ControllerOptions struct {
	StartupWindow   time.Duration
	CheckConditions bool

	startupDeadline time.Time
}

type ControllerOption func(*ControllerOptions)

// WithStartupWindow sets a duration during which the controller treats all reconciliations as if they were the first one.
// All reconciliations in this window will be imminently requeued with a short delay. This is done to avoid the thundering herd problem
// when a controller is started or restarted and many resources need to be reconciled at once.
func WithStartupWindow(d time.Duration, checkConditions bool) ControllerOption {
	return func(o *ControllerOptions) {
		o.CheckConditions = checkConditions
		o.StartupWindow = d
		if d > 0 {
			o.startupDeadline = time.Now().Add(d)
		}
	}
}
