// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package main is the entrypoint for the controlplane projector.
package main

import (
	"os"

	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/telekom/controlplane/projector/internal/bootstrap"
)

func main() {
	opts := zap.Options{Development: true}
	opts.Level = zapcore.DebugLevel
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog := ctrl.Log.WithName("setup")
	if err := bootstrap.Run(); err != nil {
		setupLog.Error(err, "operator failed")
		os.Exit(1)
	}
}
