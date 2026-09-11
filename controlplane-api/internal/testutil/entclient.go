// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"context"

	"entgo.io/ent/privacy"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"

	// SQLite driver for in-memory testing.
	_ "github.com/mattn/go-sqlite3"
)

// NewTestClient creates an ent client backed by an in-memory SQLite database.
// Migrations are run automatically. Additional ent options (e.g. interceptors)
// can be passed via enttest.WithOptions.
func NewTestClient(t enttest.TestingT, opts ...enttest.Option) *ent.Client {
	return enttest.Open(t, "sqlite3", "file:ent?mode=memory&_fk=1", opts...)
}

// AllowContext returns a context that bypasses all privacy rules.
// Use this for seeding test data, since the PrivacyMixin denies all mutations.
func AllowContext() context.Context {
	return privacy.DecisionContext(context.Background(), privacy.Allow)
}
