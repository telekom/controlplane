// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/metrics"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ControllerOpts struct {
	Prefix         string
	AllowedMethods []string
	Security       security.SecurityOpts
}

func Default() ControllerOpts {
	return ControllerOpts{
		AllowedMethods: []string{"HEAD", "GET", "POST", "PUT", "PATCH", "DELETE"},
	}
}

func ReadOnly() ControllerOpts {
	return ControllerOpts{
		AllowedMethods: []string{"HEAD", "GET"},
	}
}

func (o *ControllerOpts) IsAllowed(method string) bool {
	if len(o.AllowedMethods) == 0 {
		return true
	}
	return slices.ContainsFunc(o.AllowedMethods, func(s string) bool {
		return strings.EqualFold(s, method)
	})
}

type Controller interface {
	Register(fiber.Router, ControllerOpts)
}

type Server struct {
	controllers []Controller
	App         *fiber.App
}

func (s *Server) RegisterController(controller Controller, opts ControllerOpts) {
	s.controllers = append(s.controllers, controller)
	controller.Register(s.App.Group(opts.Prefix), opts)
}

// NewApp creates a new fiber app with default configuration

type AppConfig struct {
	fiber.Config
	CtxLog        *logr.Logger
	EnableLogging bool
	EnableMetrics bool
	EnableCors    bool
	Timeout       time.Duration
}

func NewAppConfig() AppConfig {
	return AppConfig{
		Config: fiber.Config{
			ReadTimeout:  2 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
			ErrorHandler: ReturnWithError,
			JSONEncoder:  sonic.Marshal,
			JSONDecoder:  sonic.Unmarshal,

			EnablePrintRoutes:     false,
			DisableStartupMessage: true,
		},
		EnableLogging: true,
		EnableMetrics: true,
		EnableCors:    false,
		Timeout:       10 * time.Second,
	}
}

func NewAppWithConfig(cfg AppConfig) *fiber.App {
	app := fiber.New(cfg.Config)
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
	}))
	if cfg.EnableLogging {
		if cfg.CtxLog != nil {
			app.Use(middleware.NewContextLogger(cfg.CtxLog))
		}
		app.Use(middleware.NewLogger())
	}
	if cfg.EnableCors {
		app.Use(cors.New(cors.Config{
			AllowOrigins:  "*",
			AllowMethods:  "GET,POST,PUT,PATCH,DELETE",
			ExposeHeaders: "*",
		}))
	}
	if cfg.EnableMetrics {
		metrics.NewForApp(app, prometheus.DefaultRegisterer, metrics.DefaultSkipper)
	}
	if cfg.Timeout > 0 {
		app.Use(middleware.NewTimeout(cfg.Timeout))
	}

	return app
}

func NewApp() *fiber.App {
	return NewAppWithConfig(NewAppConfig())
}

func NewServer() *Server {
	return NewServerWithApp(NewApp())
}

func NewServerWithApp(app *fiber.App) *Server {
	s := &Server{
		App: app,
	}
	return s
}

func (s *Server) Start(addr string) error {
	return s.App.Listen(addr)
}

func CalculatePrefix(gvr schema.GroupVersionResource, addGroup bool) string {
	if addGroup {
		return fmt.Sprintf("/%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)
	}
	return fmt.Sprintf("/%s/%s", gvr.Version, gvr.Resource)
}

func Return(c *fiber.Ctx, code int, body any) error {
	if c.Accepts("application/json") != "" {
		return c.Status(code).JSON(body)
	}

	// fallback
	return c.Status(code).JSON(body)
}
