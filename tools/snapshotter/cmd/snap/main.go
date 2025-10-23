// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/orchestrator"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/source"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	sourceKey  string
	routeId    string
	consumerId string
	storePath  string
	configPath string
)

func init() {
	flag.StringVar(&sourceKey, "source", "", "Source to snapshot from (only required if multiple sources are configured)")
	flag.StringVar(&routeId, "route", "", "ID of the route to snapshot")
	flag.StringVar(&consumerId, "consumer", "", "ID of the consumer to snapshot")
	flag.StringVar(&storePath, "store", "./snapshots", "Path to the snapshot store")
	flag.StringVar(&configPath, "config", "", "Path to the configuration file")
}

func main() {
	flag.Parse()
	rootCtx := signals.SetupSignalHandler()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		panic(err)
	}

	zap.ReplaceGlobals(zap.Must(zap.NewDevelopment()))

	if routeId != "" && consumerId != "" {
		zap.L().Fatal("only one of route or consumer ID can be provided")
	}

	store := store.NewFileStore(storePath)

	instances := map[string]*orchestrator.Orchestrator{}

	for key, sourceCfg := range cfg.Sources {
		zap.L().Info("setting up source", zap.String("key", key))
		kongSource, err := source.NewKongSourceFromConfig(sourceCfg)
		if err != nil {
			panic(err)
		}
		kongSource.SetTags(sourceCfg.Tags)
		instances[key] = orchestrator.NewOrchestrator(kongSource, store, sourceCfg.Obfuscators)
	}

	var instance *orchestrator.Orchestrator
	var exists bool
	if sourceKey != "" {
		instance, exists = instances[sourceKey]
	} else if len(instances) == 1 {
		for _, inst := range instances {
			instance = inst
		}
		exists = true
	} else {
		zap.L().Fatal("multiple sources configured, but no source key provided")
	}

	if !exists {
		zap.L().Fatal("no orchestrator found for source key", zap.String("sourceKey", sourceKey))
	}

	resourceType := "route"
	routeId := routeId
	if consumerId != "" {
		resourceType = "consumer"
		routeId = consumerId
	}

	instance.ReportResult = func(result diffmatcher.Result, snapID string) {
		zap.L().Info("snapshot result", zap.String("snapshotID", snapID), zap.Bool("changed", result.Changed))
		if result.Changed {
			_, _ = fmt.Fprint(os.Stdout, result.Text)
		}
	}

	_, err = instance.Do(rootCtx, resourceType, routeId)
	if err != nil {
		zap.L().Fatal("snapshotting failed", zap.Error(err))
	}
}
