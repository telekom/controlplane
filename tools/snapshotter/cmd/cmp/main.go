// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	storePath string
	aId, bId  string
)

func init() {
	flag.StringVar(&storePath, "store", "./snapshots", "Path to the snapshot store")
	flag.StringVar(&aId, "a", "", "ID of the first snapshot to compare")
	flag.StringVar(&bId, "b", "", "ID of the second snapshot to compare")
}

func main() {
	flag.Parse()
	rootCtx := signals.SetupSignalHandler()

	zap.ReplaceGlobals(zap.Must(zap.NewDevelopment()))

	s := store.NewFileStore(storePath)

	a, err := s.GetLatest(rootCtx, aId)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			zap.L().Fatal("failed to get snapshot A", zap.Error(err))
		}
	}

	b, err := s.GetLatest(rootCtx, bId)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			zap.L().Fatal("failed to get snapshot B", zap.Error(err))
		}
	}

	diff := diffmatcher.Compare(&a, &b)
	if err != nil {
		zap.L().Fatal("failed to compare snapshots", zap.Error(err))
	}

	zap.L().Info("snapshot comparison result", zap.Bool("changed", diff.Changed))
	if diff.Changed {
		_, _ = fmt.Fprint(os.Stdout, diff.Text)
	}

}
