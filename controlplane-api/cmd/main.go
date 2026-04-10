// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/server/serve"
	"github.com/vektah/gqlparser/v2/ast"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	rest "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/controlplane-api/cmd/config"
	"github.com/telekom/controlplane/controlplane-api/ent"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/internal/database"
	gqlcontroller "github.com/telekom/controlplane/controlplane-api/internal/graphql"
	"github.com/telekom/controlplane/controlplane-api/internal/interceptor"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
)

var configFile string

func init() {
	flag.StringVar(&configFile, "configfile", "", "path to config file")
}

func main() {
	flag.Parse()

	cfg := config.GetConfigOrDie(configFile)
	log := setupLogger(cfg.Log.Level)
	crlog.SetLogger(log)
	ctx := logr.NewContext(context.Background(), log)
	ctx = cserver.SignalHandler(ctx)

	log.Info("loaded configuration", "configfile", configFile)

	client, err := database.NewEntClient(ctx, cfg.Database.URL)
	if err != nil {
		log.Error(err, "failed to create ent client")
		os.Exit(1)
	}
	client.Intercept(interceptor.TeamFilterInterceptor())

	var teamService service.TeamService
	if cfg.Kubernetes.Enabled {
		k8sClient, err := newK8sClient(cfg.Kubernetes)
		if err != nil {
			log.Error(err, "failed to create Kubernetes client")
			os.Exit(1)
		}
		teamService = service.NewTeamK8sService(k8sClient)
		log.Info("Kubernetes integration enabled")
	} else {
		log.Info("Kubernetes integration disabled, mutations will be unavailable")
	}

	srv := newGraphQLServer(client, teamService, cfg.Security.Enabled)

	appCfg := cserver.NewAppConfig()
	appCfg.CtxLog = log
	appCfg.EnableCors = true
	s := cserver.NewServerWithApp(cserver.NewAppWithConfig(appCfg))

	probesCtrl := cserver.NewProbesController()
	probesCtrl.Register(s.App, cserver.ControllerOpts{})

	gqlCtrl := gqlcontroller.NewController(srv, cfg.GraphQL.PlaygroundEnabled)
	gqlCtrl.RegisterPlayground(s.App, "/graphql")
	secOpts := security.SecurityOpts{
		Enabled: cfg.Security.Enabled,
		Log:     log.WithName("security"),
	}
	if len(cfg.Security.TrustedIssuers) > 0 {
		secOpts.JWTOpts = []security.Option[*security.JWTOpts]{
			security.WithTrustedIssuers(cfg.Security.TrustedIssuers),
		}
	}
	s.RegisterController(gqlCtrl, cserver.ControllerOpts{
		Prefix:         "/graphql",
		AllowedMethods: []string{http.MethodHead, http.MethodGet, http.MethodPost, http.MethodOptions},
		Security:       secOpts,
	})

	go func() {
		if !cfg.Server.TLS.Enabled {
			fmt.Println("WARNING: Using HTTP instead of HTTPS. This is not secure.")
			if err := s.App.Listen(cfg.Server.Address); err != nil {
				log.Error(err, "failed to start server")
				os.Exit(1)
			}
			return
		}

		tlsCtx := logr.NewContext(ctx, log.WithName("server"))
		if err := serve.ServeTLS(tlsCtx, s.App, cfg.Server.Address, cfg.Server.TLS.Cert, cfg.Server.TLS.Key); err != nil {
			log.Error(err, "failed to start server")
			os.Exit(1)
		}
	}()
	log.Info("server started", "addr", cfg.Server.Address, "tls", cfg.Server.TLS.Enabled)

	<-ctx.Done()
	log.Info("shutting down server")

	if err := s.App.Shutdown(); err != nil {
		log.Error(err, "failed to gracefully shutdown server")
	}

	if err := client.Close(); err != nil {
		log.Error(err, "failed to close database client")
	}
}

func setupLogger(level string) logr.Logger {
	logCfg := zap.NewProductionConfig()
	logCfg.DisableStacktrace = true
	logCfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	zapLogLevel, err := zapcore.ParseLevel(level)
	if err != nil {
		zapLogLevel = zapcore.InfoLevel
	}
	logCfg.Level.SetLevel(zapLogLevel)
	return zapr.NewLogger(zap.Must(logCfg.Build()))
}

func newK8sClient(cfg config.KubernetesConfig) (client.Client, error) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(organizationv1.AddToScheme(scheme))

	var restConfig *rest.Config
	var err error
	if cfg.Kubeconfig != "" {
		restConfig, err = clientcmd.BuildConfigFromFlags("", cfg.Kubeconfig)
	} else {
		restConfig, err = ctrl.GetConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	return client.New(restConfig, client.Options{Scheme: scheme})
}

func newGraphQLServer(entClient *ent.Client, teamService service.TeamService, securityEnabled bool) *handler.Server {
	srv := handler.New(resolvers.NewSchema(entClient, teamService))
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	srv.AroundOperations(gqlcontroller.ViewerFromBusinessContext(entClient, securityEnabled))

	return srv
}
