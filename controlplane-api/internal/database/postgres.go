// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/migrate"
)

// NewEntClient creates a new ent client connected to PostgreSQL.
func NewEntClient(ctx context.Context, databaseURL string) (*ent.Client, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	drv := sql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))

	if err := client.Schema.Create(ctx,
		migrate.WithGlobalUniqueID(true),
	); err != nil {
		if closeErr := client.Close(); closeErr != nil {
			logr.FromContextOrDiscard(ctx).Error(closeErr, "failed to close database client after migration error")
		}
		return nil, fmt.Errorf("running schema migration: %w", err)
	}

	return client, nil
}
