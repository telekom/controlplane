// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"

	"github.com/go-logr/logr"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/telekom/controlplane/rover-server/internal/controller"
	"github.com/telekom/controlplane/rover-server/internal/server"
	"github.com/telekom/controlplane/rover-server/pkg/log"
	"github.com/telekom/controlplane/rover-server/pkg/store"
)

var rootCtx = logr.NewContext(context.Background(), log.Log)

func main() {
	store.InitOrDie(rootCtx, config.GetConfigOrDie())

	app := cserver.NewApp()

	probesCtrl := cserver.NewProbesController()
	probesCtrl.Register(app, cserver.ControllerOpts{})

	s := server.Server{
		Log:                 log.Log,
		ApiSpecifications:   controller.NewApiSpecificationController(),
		Rovers:              controller.NewRoverController(),
		EventSpecifications: controller.NewEventSpecificationController(),
	}

	s.RegisterRoutes(app)

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
