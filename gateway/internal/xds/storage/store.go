// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

// Package storage provides the single-writer SQLite persistence layer for xDS bundles.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/protobuf/proto"

	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"

	_ "modernc.org/sqlite"
)

const (
	DefaultHistoryLimit = 10
	schemaVersion       = 2
)

var ErrGenerationConflict = errors.New("publisher generation already contains different content")

// ActiveBundle is one target's durable active generation.
type ActiveBundle struct {
	Generation uint64
	Bundle     *xdsapi.Bundle
}

// Activation is the durable publication result.
type Activation struct {
	Generation uint64
	Idempotent bool
	Active     bool
}

// Store owns one SQLite connection and therefore serializes all POC writes.
type Store struct {
	db           *sql.DB
	historyLimit int
}

// Open creates or opens a private SQLite database, enables WAL, and applies migrations.
func Open(ctx context.Context, path string, historyLimit int) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if historyLimit <= 0 {
		historyLimit = DefaultHistoryLimit
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // Operator-configured path.
	if err != nil {
		return nil, fmt.Errorf("creating database file: %w", err)
	}
	if closeErr := file.Close(); closeErr != nil {
		return nil, fmt.Errorf("closing database file: %w", closeErr)
	}
	if chmodErr := os.Chmod(path, 0o600); chmodErr != nil {
		return nil, fmt.Errorf("setting database permissions: %w", chmodErr)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{db: db, historyLimit: historyLimit}
	if err := store.initialize(ctx); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, errors.Join(err, fmt.Errorf("closing failed database: %w", closeErr))
		}
		return nil, err
	}
	return store, nil
}

func (s *Store) initialize(ctx context.Context) error {
	statements := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initializing database: %w", err)
		}
	}

	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	if version == 0 {
		if err := s.createSchema(ctx); err != nil {
			return err
		}
		return nil
	}
	if version != schemaVersion {
		return fmt.Errorf("unsupported database schema version %d", version)
	}
	return nil
}

func (s *Store) createSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS bundles (
			target_id TEXT NOT NULL,
			generation INTEGER NOT NULL,
			publisher_generation TEXT NOT NULL,
			digest TEXT NOT NULL,
			envelope BLOB NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (target_id, generation),
			UNIQUE (target_id, digest),
			UNIQUE (target_id, publisher_generation)
		)`,
		`CREATE TABLE IF NOT EXISTS digest_records (
			target_id TEXT NOT NULL,
			digest TEXT NOT NULL,
			generation INTEGER NOT NULL,
			PRIMARY KEY (target_id, digest)
		)`,
		`CREATE TABLE IF NOT EXISTS publisher_records (
			target_id TEXT NOT NULL,
			publisher_generation TEXT NOT NULL,
			digest TEXT NOT NULL,
			generation INTEGER NOT NULL,
			PRIMARY KEY (target_id, publisher_generation)
		)`,
		`CREATE TABLE IF NOT EXISTS active_targets (
			target_id TEXT PRIMARY KEY,
			generation INTEGER NOT NULL,
			digest TEXT NOT NULL,
			FOREIGN KEY (target_id, generation) REFERENCES bundles(target_id, generation)
		)`,
		`CREATE TABLE IF NOT EXISTS delivery_observations (
			target_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			type_url TEXT NOT NULL,
			generation INTEGER NOT NULL,
			state INTEGER NOT NULL,
			nonce TEXT NOT NULL,
			error_detail TEXT NOT NULL,
			observed_at INTEGER NOT NULL,
			PRIMARY KEY (target_id, node_id, type_url, generation)
		)`,
		`INSERT OR IGNORE INTO schema_migrations(version) VALUES (2)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initializing database: %w", err)
		}
	}

	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("closing database: %w", err)
	}
	return nil
}

// JournalMode returns SQLite's effective journal mode.
func (s *Store) JournalMode(ctx context.Context) (string, error) {
	var mode string
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return "", fmt.Errorf("reading journal mode: %w", err)
	}
	return mode, nil
}

// Activate atomically inserts an immutable candidate and advances its target's active pointer.
func (s *Store) Activate(ctx context.Context, bundle *xdsapi.Bundle) (Activation, error) {
	if bundle == nil {
		return Activation{}, fmt.Errorf("bundle is required")
	}
	payload, err := xdsapi.MarshalDeterministic(bundle)
	if err != nil {
		return Activation{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Activation{}, fmt.Errorf("beginning activation: %w", err)
	}
	defer func() {
		_ = tx.Rollback() //nolint:errcheck // Commit makes rollback return sql.ErrTxDone.
	}()

	if replay, found, replayErr := replayActivation(ctx, tx, bundle); replayErr != nil {
		return Activation{}, replayErr
	} else if found {
		return replay, nil
	}

	var existingDigest string
	err = tx.QueryRowContext(ctx,
		"SELECT digest FROM publisher_records WHERE target_id = ? AND publisher_generation = ?",
		bundle.TargetId, bundle.PublisherGeneration,
	).Scan(&existingDigest)
	if err == nil && existingDigest != bundle.Digest {
		return Activation{}, ErrGenerationConflict
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Activation{}, fmt.Errorf("checking publisher generation: %w", err)
	}

	var generation uint64
	if err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(generation), 0) + 1 FROM digest_records WHERE target_id = ?",
		bundle.TargetId,
	).Scan(&generation); err != nil {
		return Activation{}, fmt.Errorf("allocating generation: %w", err)
	}
	if _, insertErr := tx.ExecContext(ctx,
		`INSERT INTO bundles(target_id, generation, publisher_generation, digest, envelope, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		bundle.TargetId, generation, bundle.PublisherGeneration, bundle.Digest, payload, time.Now().UTC().UnixNano(),
	); insertErr != nil {
		return Activation{}, fmt.Errorf("inserting bundle: %w", insertErr)
	}
	if _, insertErr := tx.ExecContext(ctx,
		"INSERT INTO digest_records(target_id, digest, generation) VALUES (?, ?, ?)",
		bundle.TargetId, bundle.Digest, generation); insertErr != nil {
		return Activation{}, fmt.Errorf("recording bundle digest: %w", insertErr)
	}
	if _, insertErr := tx.ExecContext(ctx,
		"INSERT INTO publisher_records(target_id, publisher_generation, digest, generation) VALUES (?, ?, ?, ?)",
		bundle.TargetId, bundle.PublisherGeneration, bundle.Digest, generation); insertErr != nil {
		return Activation{}, fmt.Errorf("recording publisher generation: %w", insertErr)
	}
	if _, activeErr := tx.ExecContext(ctx,
		`INSERT INTO active_targets(target_id, generation, digest) VALUES (?, ?, ?)
		 ON CONFLICT(target_id) DO UPDATE SET generation=excluded.generation, digest=excluded.digest`,
		bundle.TargetId, generation, bundle.Digest,
	); activeErr != nil {
		return Activation{}, fmt.Errorf("updating active generation: %w", activeErr)
	}
	if _, pruneErr := tx.ExecContext(ctx,
		`DELETE FROM bundles WHERE target_id = ? AND generation NOT IN (
		 SELECT generation FROM bundles WHERE target_id = ? ORDER BY generation DESC LIMIT ?
		)`, bundle.TargetId, bundle.TargetId, s.historyLimit,
	); pruneErr != nil {
		return Activation{}, fmt.Errorf("pruning bundle history: %w", pruneErr)
	}
	if commitErr := tx.Commit(); commitErr != nil {
		return Activation{}, fmt.Errorf("committing activation: %w", commitErr)
	}
	return Activation{Generation: generation, Active: true}, nil
}

func replayActivation(
	ctx context.Context,
	tx *sql.Tx,
	bundle *xdsapi.Bundle,
) (Activation, bool, error) {
	var generation uint64
	err := tx.QueryRowContext(ctx,
		"SELECT generation FROM digest_records WHERE target_id = ? AND digest = ?",
		bundle.TargetId, bundle.Digest,
	).Scan(&generation)
	if errors.Is(err, sql.ErrNoRows) {
		return Activation{}, false, nil
	}
	if err != nil {
		return Activation{}, false, fmt.Errorf("looking up bundle digest: %w", err)
	}
	if _, recordErr := tx.ExecContext(ctx,
		`INSERT INTO publisher_records(target_id, publisher_generation, digest, generation) VALUES (?, ?, ?, ?)
		 ON CONFLICT(target_id, publisher_generation) DO NOTHING`,
		bundle.TargetId, bundle.PublisherGeneration, bundle.Digest, generation); recordErr != nil {
		return Activation{}, false, fmt.Errorf("recording idempotent publisher generation: %w", recordErr)
	}
	var recordedDigest string
	if recordErr := tx.QueryRowContext(ctx,
		"SELECT digest FROM publisher_records WHERE target_id = ? AND publisher_generation = ?",
		bundle.TargetId, bundle.PublisherGeneration).Scan(&recordedDigest); recordErr != nil {
		return Activation{}, false, fmt.Errorf("checking idempotent publisher generation: %w", recordErr)
	}
	if recordedDigest != bundle.Digest {
		return Activation{}, false, ErrGenerationConflict
	}
	var activeGeneration uint64
	err = tx.QueryRowContext(ctx,
		"SELECT generation FROM active_targets WHERE target_id = ?", bundle.TargetId,
	).Scan(&activeGeneration)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Activation{}, false, fmt.Errorf("reading active generation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Activation{}, false, fmt.Errorf("committing idempotent activation: %w", err)
	}
	return Activation{
		Generation: generation,
		Idempotent: true,
		Active:     activeGeneration == generation,
	}, true, nil
}

// LoadActive loads and verifies every durable active bundle.
func (s *Store) LoadActive(ctx context.Context) (map[string]ActiveBundle, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.target_id, a.generation, a.digest, b.envelope
		FROM active_targets a JOIN bundles b
		ON b.target_id = a.target_id AND b.generation = a.generation ORDER BY a.target_id`)
	if err != nil {
		return nil, fmt.Errorf("querying active bundles: %w", err)
	}
	defer func() {
		_ = rows.Close() //nolint:errcheck // Explicit close below surfaces errors.
	}()

	active := make(map[string]ActiveBundle)
	for rows.Next() {
		var targetID, storedDigest string
		var generation uint64
		var payload []byte
		if err := rows.Scan(&targetID, &generation, &storedDigest, &payload); err != nil {
			return nil, fmt.Errorf("scanning active bundle: %w", err)
		}
		bundle := &xdsapi.Bundle{}
		if err := proto.Unmarshal(payload, bundle); err != nil {
			return nil, fmt.Errorf("decoding active bundle for target %q: %w", targetID, err)
		}
		digest, err := xdsapi.Digest(bundle)
		if err != nil {
			return nil, fmt.Errorf("digesting active bundle for target %q: %w", targetID, err)
		}
		if bundle.TargetId != targetID || bundle.Digest != storedDigest || digest != storedDigest {
			return nil, fmt.Errorf("active bundle for target %q is corrupt", targetID)
		}
		active[targetID] = ActiveBundle{Generation: generation, Bundle: bundle}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating active bundles: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing active bundle rows: %w", err)
	}
	return active, nil
}

// Active returns one target's active bundle, if present.
func (s *Store) Active(ctx context.Context, targetID string) (ActiveBundle, bool, error) {
	all, err := s.LoadActive(ctx)
	if err != nil {
		return ActiveBundle{}, false, err
	}
	active, ok := all[targetID]
	return active, ok, nil
}

// RecordObservation upserts the latest delivery result for one node and resource type.
func (s *Store) RecordObservation(ctx context.Context, targetID string, observation *xdsapi.DeliveryObservation) error {
	if observation == nil || observation.ObservedAt == nil {
		return fmt.Errorf("complete observation is required")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO delivery_observations(
		target_id, node_id, type_url, generation, state, nonce, error_detail, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(target_id, node_id, type_url, generation) DO UPDATE SET
		generation=excluded.generation, state=excluded.state, nonce=excluded.nonce,
		error_detail=excluded.error_detail, observed_at=excluded.observed_at`,
		targetID, observation.NodeId, observation.TypeUrl, observation.Generation, observation.State,
		observation.Nonce, observation.ErrorDetail, observation.ObservedAt.AsTime().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("recording delivery observation: %w", err)
	}
	return nil
}

// Observations returns the latest observations for a target in stable order.
func (s *Store) Observations(ctx context.Context, targetID string) ([]*xdsapi.DeliveryObservation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT node_id, type_url, generation, state, nonce, error_detail, observed_at
		FROM delivery_observations WHERE target_id = ? ORDER BY node_id, type_url`, targetID)
	if err != nil {
		return nil, fmt.Errorf("querying observations: %w", err)
	}
	defer func() {
		_ = rows.Close() //nolint:errcheck // Explicit close below surfaces errors.
	}()

	var observations []*xdsapi.DeliveryObservation
	for rows.Next() {
		observation := &xdsapi.DeliveryObservation{}
		var observedAt int64
		if err := rows.Scan(&observation.NodeId, &observation.TypeUrl, &observation.Generation,
			&observation.State, &observation.Nonce, &observation.ErrorDetail, &observedAt); err != nil {
			return nil, fmt.Errorf("scanning observation: %w", err)
		}
		observation.ObservedAt = timestampFromUnixNano(observedAt)
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating observations: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing observation rows: %w", err)
	}
	return observations, nil
}
