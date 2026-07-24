// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"

	"github.com/go-logr/logr"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store/inmemory"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
	roverin "github.com/telekom/controlplane/rover-server/internal/mapper/rover/in"
	"github.com/telekom/controlplane/rover-server/internal/oaslint"
	kconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/telekom/controlplane/rover-server/internal/config"
	"github.com/telekom/controlplane/rover-server/internal/controller"
	"github.com/telekom/controlplane/rover-server/internal/server"
	"github.com/telekom/controlplane/rover-server/pkg/log"
	"github.com/telekom/controlplane/rover-server/pkg/store"
)

func main() {
	var configFile string
	flag.StringVar(&configFile, "configfile", "", "path to config file")
	flag.Parse()

	cfg := config.LoadConfig(configFile)
	roverin.MigrationActive = cfg.Migration.Active

	log.Init(cfg.Log)
	rootCtx := logr.NewContext(context.Background(), log.Log)

	stores := store.NewStores(rootCtx, kconfig.GetConfigOrDie(),
		inmemory.DatabaseOpts{Filepath: cfg.Database.Filepath, ReduceMemory: cfg.Database.ReduceMemory},
		inmemory.InformerOpts{DisableCache: cfg.Informer.DisableCache},
	)

	appCfg := cserver.NewAppConfig()
	appCfg.CtxLog = log.Log

	file.AppendOption(
		filesapi.WithSkipTLSVerify(cfg.FileManager.SkipTLS),
	)

	linter := oaslint.NewLinter(cfg.OasLinting)

	s := server.Server{
		Config:              cfg,
		Log:                 log.Log,
		ApiSpecifications:   controller.NewApiSpecificationController(stores, linter),
		Rovers:              controller.NewRoverController(stores),
		Roadmaps:            controller.NewRoadmapController(stores),
		EventSpecifications: controller.NewEventSpecificationController(stores),
		ApiChangelogs:       controller.NewApiChangelogController(stores),
		McpSpecifications:   controller.NewMcpSpecificationController(stores),
	}

	// jwtOpts injects rover's server-specific check-access templates into the
	// JWT SecurityOpts derived from each listener's jwt block.
	jwtOpts := func(jc security.JWTConfig) security.SecurityOpts {
		opts := jc.ToSecurityOpts()
		opts.Log = log.Log
		opts.BusinessContextOpts = append(opts.BusinessContextOpts, security.WithLog(log.Log))
		opts.CheckAccessOpts = []security.Option[*security.CheckAccessOpts]{
			security.WithPathParamKey("resourceId"),
			security.WithTemplates(server.SecurityTemplates),
		}
		return opts
	}

	buildListener := func(lc *cserver.ListenerConfig, internal bool) *cserver.Listener {
		if lc == nil {
			return nil
		}
		// rover is a JWT-server: an internal k8s listener gets admin-context
		// (which also marks it internal for open-access). External listeners
		// are JWT; the k8s-only options are ignored there.
		var opts []cserver.FamilyOption
		if internal {
			opts = append(opts, cserver.WithAdminContext())
		}
		fam, err := cserver.FamilyFromListenerConfig(*lc, jwtOpts, opts...)
		if err != nil {
			log.Log.Error(err, "Failed to build security family for listener", "address", lc.Address)
			panic(err)
		}
		return &cserver.Listener{Address: lc.Address, Family: fam}
	}

	ms := &cserver.MultiServer{
		AppConfig: appCfg,
		TLS:       cfg.TLS.ToServerTLS(),
		Listeners: cserver.Listeners{
			Internal: buildListener(cfg.Listeners.Internal, true),
			External: buildListener(cfg.Listeners.External, false),
		},
		Register: s.RegisterRoutes,
	}

	if err := ms.Run(rootCtx); err != nil {
		log.Log.Error(err, "server exited with error")
		panic(err)
	}

	log.Log.Info("Server gracefully stopped")
}
