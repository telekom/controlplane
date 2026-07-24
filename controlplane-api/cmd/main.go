// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	cc "github.com/telekom/controlplane/common/pkg/client"
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

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/controlplane-api/cmd/config"
	"github.com/telekom/controlplane/controlplane-api/ent"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/internal/database"
	gqlcontroller "github.com/telekom/controlplane/controlplane-api/internal/graphql"
	"github.com/telekom/controlplane/controlplane-api/internal/interceptor"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	"github.com/telekom/controlplane/controlplane-api/internal/secrets"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
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

	var services service.Services
	if cfg.Kubernetes.Enabled {
		k8sClient, err := newK8sClient(cfg.Kubernetes)
		if err != nil {
			log.Error(err, "failed to create Kubernetes client")
			os.Exit(1)
		}
		scopedClient := cc.NewScopedClient(k8sClient, cfg.Kubernetes.Environment)
		services = service.Services{
			Team:        service.NewTeamK8sService(scopedClient),
			Application: service.NewApplicationK8sService(scopedClient),
			Approval:    service.NewApprovalK8sService(scopedClient),
		}
		log.Info("Kubernetes integration enabled")
	} else {
		log.Info("Kubernetes integration disabled, mutations will be unavailable")
	}

	secretResolver := secrets.NewResolver(secretsapi.NewSecrets())
	srv := newGraphQLServer(client, services, secretResolver, cfg.FileManager.BaseURL)
	appCfg := cserver.NewAppConfig()
	appCfg.CtxLog = log
	appCfg.EnableCors = true

	gqlCtrl := gqlcontroller.NewController(srv, cfg.GraphQL.PlaygroundEnabled)

	// jwtOpts turns the listener's jwt block into full SecurityOpts. controlplane-api
	// has no server-specific check-access templates (GraphQL guards via business
	// context downstream), so this only wires the logger.
	jwtOpts := func(jc security.JWTConfig) security.SecurityOpts {
		opts := jc.ToSecurityOpts()
		opts.Log = log.WithName("security")
		opts.BusinessContextOpts = append(opts.BusinessContextOpts, security.WithLog(log.WithName("security")))
		return opts
	}

	// buildListener turns a listener config into a Listener, choosing JWT vs
	// K8s from the config block. Internal listeners get admin-context.
	buildListener := func(lc *cserver.ListenerConfig, internal bool) *cserver.Listener {
		if lc == nil {
			return nil
		}
		var opts []cserver.FamilyOption
		if internal {
			opts = append(opts, cserver.WithAdminContext())
		}
		fam, err := cserver.FamilyFromListenerConfig(*lc, jwtOpts, opts...)
		if err != nil {
			log.Error(err, "failed to build security family for listener", "address", lc.Address)
			os.Exit(1)
		}
		return &cserver.Listener{Address: lc.Address, Family: fam}
	}

	if cfg.Listeners.External == nil {
		log.Error(fmt.Errorf("no external listener configured"), "controlplane-api requires listeners.external")
		os.Exit(1)
	}

	ms := &cserver.MultiServer{
		AppConfig: appCfg,
		TLS:       cfg.TLS.ToServerTLS(),
		Listeners: cserver.Listeners{
			Internal: buildListener(cfg.Listeners.Internal, true),
			External: buildListener(cfg.Listeners.External, false),
		},
		Register: gqlCtrl.RegisterRoutes,
	}

	ctx = logr.NewContext(ctx, log.WithName("server"))
	go func() {
		if err := ms.Run(ctx); err != nil {
			log.Error(err, "server exited with error")
			os.Exit(1)
		}
	}()
	log.Info("server started", "external", cfg.Listeners.External.Address, "internal", cfg.Listeners.Internal, "tls", cfg.TLS != nil)

	<-ctx.Done()
	log.Info("shutting down server")

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
	utilruntime.Must(applicationv1.AddToScheme(scheme))
	utilruntime.Must(approvalv1.AddToScheme(scheme))
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

func newGraphQLServer(entClient *ent.Client, services service.Services, secretResolver *secrets.Resolver, fileManagerBaseURL string) *handler.Server {
	srv := handler.New(resolvers.NewSchema(entClient, services, secretResolver, fileManagerBaseURL))
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})
	srv.SetErrorPresenter(gqlcontroller.ErrorPresenter)

	srv.AroundOperations(gqlcontroller.ViewerFromBusinessContext(entClient))
	srv.AroundOperations(gqlcontroller.LogMutationUser())

	return srv
}
