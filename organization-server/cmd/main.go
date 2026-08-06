// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"

	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"

	"github.com/telekom/controlplane/organization-server/internal/client"
	"github.com/telekom/controlplane/organization-server/internal/config"
	"github.com/telekom/controlplane/organization-server/internal/controller"
	"github.com/telekom/controlplane/organization-server/internal/server"
	"github.com/telekom/controlplane/organization-server/pkg/log"
)

func main() {
	var configFile string
	flag.StringVar(&configFile, "configfile", "", "path to config file")
	flag.Parse()

	cfg := config.LoadConfig(configFile)
	log.Init(cfg.Log)
	logger := log.Log
	rootCtx := logr.NewContext(context.Background(), logger)

	logger.Info("Starting organization-server",
		"cpapiEndpoint", cfg.CPAPI.Endpoint,
		"roverEndpoint", cfg.Rover.Endpoint,
	)

	// Upstream clients. Both talk to the upstream's internal (Kubernetes-authz)
	// listener, authenticating with a ServiceAccount token projected into the
	// pod for that upstream's audience.
	cpapiClient := client.NewCPAPIClient(cfg.CPAPI.Endpoint, accesstoken.NewAccessToken(cfg.CPAPI.TokenFilePath), cfg.CPAPI.CaFilePath)
	roverClient := client.NewRoverClient(cfg.Rover.Endpoint, accesstoken.NewAccessToken(cfg.Rover.TokenFilePath), cfg.Rover.CaFilePath)

	appCfg := cserver.NewAppConfig()
	appCfg.CtxLog = logger

	ctrl := controller.New(cpapiClient, roverClient)
	srv := server.New(ctrl, logger)

	jwtOpts := func(jc security.JWTConfig) security.SecurityOpts {
		opts := jc.ToSecurityOpts()
		opts.Log = logger
		opts.BusinessContextOpts = append(opts.BusinessContextOpts,
			security.WithLog(logger),
		)
		opts.CheckAccessOpts = []security.Option[*security.CheckAccessOpts]{
			security.WithPathParamKey("hub", "team"),
			security.WithTemplates(server.SecurityTemplates),
		}
		return opts
	}

	buildListener := func(lc *cserver.ListenerConfig, internal bool) *cserver.Listener {
		if lc == nil {
			return nil
		}
		var opts []cserver.FamilyOption
		if internal {
			opts = append(opts, cserver.WithAdminContext())
		}
		family, err := cserver.FamilyFromListenerConfig(*lc, jwtOpts, opts...)
		if err != nil {
			logger.Error(err, "Failed to build security family for listener", "address", lc.Address)
			panic(err)
		}
		return &cserver.Listener{Address: lc.Address, Family: family}
	}

	ms := &cserver.MultiServer{
		AppConfig: appCfg,
		TLS:       cfg.TLS.ToServerTLS(),
		Listeners: cserver.Listeners{
			External: buildListener(cfg.Listeners.External, false),
			Internal: buildListener(cfg.Listeners.Internal, true),
		},
		Register: func(router fiber.Router, guard fiber.Handler) {
			srv.RegisterRoutes(router.Group("/organization/v1"), guard)
		},
	}

	if err := ms.Run(rootCtx); err != nil {
		logger.Error(err, "server exited with error")
		panic(err)
	}
	logger.Info("Server gracefully stopped")
}
