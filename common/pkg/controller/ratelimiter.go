// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/telekom/controlplane/common/pkg/config"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewRateLimiter[T reconcile.Request]() workqueue.TypedRateLimiter[T] {
	return workqueue.NewTypedItemExponentialFailureRateLimiter[T](config.RequeueAfterOnError, config.MaxBackoff)
}
