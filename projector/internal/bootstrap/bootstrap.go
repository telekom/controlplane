// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package bootstrap provides the projector entry point.
// It wires shared infrastructure and registers all resource modules with the
// controller-runtime manager.
//
// This package is intentionally minimal — all domain logic lives in the
// domain packages, and all generic pipeline logic lives in the runtime package.
package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	appv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/migrate"
	orgv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/projector/internal/config"
	"github.com/telekom/controlplane/projector/internal/domain/apiexposure"
	"github.com/telekom/controlplane/projector/internal/domain/apisubscription"
	"github.com/telekom/controlplane/projector/internal/domain/application"
	"github.com/telekom/controlplane/projector/internal/domain/approval"
	"github.com/telekom/controlplane/projector/internal/domain/approvalrequest"
	"github.com/telekom/controlplane/projector/internal/domain/group"
	"github.com/telekom/controlplane/projector/internal/domain/team"
	"github.com/telekom/controlplane/projector/internal/domain/zone"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/module"

	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	// Register all CR schemes used by the projector modules.
	_ = adminv1.AddToScheme(scheme)
	_ = apiv1.AddToScheme(scheme)
	_ = appv1.AddToScheme(scheme)
	_ = approvalv1.AddToScheme(scheme)
	_ = orgv1.AddToScheme(scheme)
}

// modules is the ordered list of resource modules to register.
var modules = []module.Module{
	zone.Module,
	group.Module,
	team.Module,
	application.Module,
	apiexposure.Module,
	apisubscription.Module,
	approval.Module,
	approvalrequest.Module,
}

// Run is the projector entry point. It sets up the database, caches,
// controller manager, registers all modules, and starts the manager.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	setupLog.Info("configuration loaded",
		"maxConcurrentReconciles", cfg.MaxConcurrentReconciles,
		"periodicResync", cfg.PeriodicResync,
		"leaderElection", cfg.LeaderElection,
	)

	// --- Database ---
	ctx := context.Background()
	db, err := openDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("opening database connection: %w", err)
	}
	defer func() { _ = db.Close() }()

	drv := entsql.OpenDB(dialect.Postgres, db)
	entClient := ent.NewClient(ent.Driver(drv))
	defer func() {
		if closeErr := entClient.Close(); closeErr != nil {
			setupLog.Error(closeErr, "error closing ent client")
		}
	}()

	// --- Schema Migration ---
	setupLog.Info("running schema migration")
	if err = entClient.Schema.Create(ctx,
		migrate.WithGlobalUniqueID(true),
		migrate.WithDropIndex(true),
		migrate.WithDropColumn(true),
	); err != nil {
		return fmt.Errorf("running schema migration: %w", err)
	}
	setupLog.Info("schema migration completed")

	// --- Shared Infrastructure ---
	edgeCache, err := infrastructure.NewEdgeCache(cfg.EdgeCacheNumCounters, cfg.EdgeCacheMaxCost, cfg.EdgeCacheBufferItems)
	if err != nil {
		return fmt.Errorf("creating edge cache: %w", err)
	}
	defer edgeCache.Close()

	deleteCache := &infrastructure.DeleteCache{}
	idResolver := infrastructure.NewIDResolver(entClient, edgeCache,
		infrastructure.WithNegativeCacheTTL(cfg.IDResolverNegTTL),
		infrastructure.WithSingleflight(cfg.IDResolverSingleflight),
	)

	deps := module.ModuleDeps{
		DeleteCache: deleteCache,
		EntClient:   entClient,
		EdgeCache:   edgeCache,
		IDResolver:  idResolver,
		Config:      cfg,
	}

	// --- Controller Manager ---
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: cfg.MetricsBindAddress,
		},
		HealthProbeBindAddress: cfg.HealthProbeBindAddress,
		LeaderElection:         cfg.LeaderElection,
		LeaderElectionID:       cfg.LeaderElectionID,
	})
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	// --- Health Checks ---
	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("setting up health check: %w", err)
	}
	if err = mgr.AddReadyzCheck("readyz", newDBReadyCheck(db)); err != nil {
		return fmt.Errorf("setting up ready check: %w", err)
	}

	// --- Register Modules ---
	for _, m := range modules {
		if err = m.Register(mgr, deps); err != nil {
			return fmt.Errorf("register %s: %w", m.Name(), err)
		}
		setupLog.Info("module registered", "module", m.Name())
	}

	// --- Start ---
	setupLog.Info("starting manager", "modules", len(modules))
	return mgr.Start(ctrl.SetupSignalHandler())
}

// openDatabase creates a pgx connection pool and returns the underlying *sql.DB
// with pool settings from the config.
func openDatabase(ctx context.Context, cfg *config.Config) (*sql.DB, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.MaxOpenConns)
	poolConfig.MinConns = int32(cfg.MaxIdleConns)
	poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	return db, nil
}

// newDBReadyCheck returns a healthz.Checker that pings the database.
func newDBReadyCheck(db *sql.DB) healthz.Checker {
	return func(_ *http.Request) error {
		return db.Ping()
	}
}
