// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package config provides configuration loading and defaults for the
// projector.
package config

import (
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// Config holds all configuration values for the projector.
type Config struct {
	// --- Database ---

	// DatabaseURL is the PostgreSQL connection string.
	// Env: DATABASE_URL
	DatabaseURL string `mapstructure:"database_url" validate:"required"`

	// MaxOpenConns is the maximum number of open connections to the database.
	// Env: DB_MAX_OPEN_CONNS
	MaxOpenConns int `mapstructure:"db_max_open_conns" validate:"required,gt=0"`

	// MaxIdleConns is the maximum number of idle connections in the pool.
	// Env: DB_MAX_IDLE_CONNS
	MaxIdleConns int `mapstructure:"db_max_idle_conns" validate:"required,gt=0"`

	// ConnMaxLifetime is the maximum lifetime of a database connection.
	// Env: DB_CONN_MAX_LIFETIME
	ConnMaxLifetime time.Duration `mapstructure:"db_conn_max_lifetime" validate:"required,gt=0"`

	// --- Concurrency ---

	// MaxConcurrentReconciles is the global default number of concurrent
	// reconciles per controller. Per-module overrides are resolved via
	// ConcurrencyFor().
	// Env: MAX_CONCURRENT_RECONCILES
	MaxConcurrentReconciles int `mapstructure:"max_concurrent_reconciles" validate:"required,gt=0"`

	// ReconcileTimeout is the maximum duration a single Reconcile call may
	// run before its context is cancelled. Passed to controller-runtime's
	// ReconciliationTimeout option. Set to 0 to disable (no timeout).
	// Env: RECONCILE_TIMEOUT
	ReconcileTimeout time.Duration `mapstructure:"reconcile_timeout" validate:"gte=0"`

	// --- Resync ---

	// PeriodicResync is the interval at which successful reconciles are
	// re-queued to ensure eventual consistency. Set to 0 (the default) for
	// purely event-driven operation.
	// Env: PERIODIC_RESYNC
	PeriodicResync time.Duration `mapstructure:"periodic_resync" validate:"gte=0"`

	// --- Error Policy ---

	// DependencyDelay is the base requeue delay when a dependency is missing.
	// Env: DEPENDENCY_DELAY
	DependencyDelay time.Duration `mapstructure:"dependency_delay" validate:"required,gt=0"`

	// DependencyDelayJitter is the maximum random jitter added to
	// DependencyDelay to spread retry storms.
	// Env: DEPENDENCY_DELAY_JITTER
	DependencyDelayJitter time.Duration `mapstructure:"dependency_delay_jitter" validate:"gte=0"`

	// SkipRequeue is the delay before retrying CRs that were skipped
	// (e.g. missing required labels).
	// Env: SKIP_REQUEUE
	SkipRequeue time.Duration `mapstructure:"skip_requeue" validate:"required,gt=0"`

	// --- Rate Limiter ---

	// RateLimiterBaseDelay is the initial delay for the exponential backoff
	// rate limiter applied to errored reconciles.
	// Env: RATE_LIMITER_BASE_DELAY
	RateLimiterBaseDelay time.Duration `mapstructure:"rate_limiter_base_delay" validate:"required,gt=0"`

	// RateLimiterMaxDelay is the ceiling for the exponential backoff.
	// Env: RATE_LIMITER_MAX_DELAY
	RateLimiterMaxDelay time.Duration `mapstructure:"rate_limiter_max_delay" validate:"required,gt=0"`

	// RateLimiterQPS is the token-bucket refill rate (queries per second).
	// Env: RATE_LIMITER_QPS
	RateLimiterQPS float64 `mapstructure:"rate_limiter_qps" validate:"required,gt=0"`

	// RateLimiterBurst is the token-bucket burst size.
	// Env: RATE_LIMITER_BURST
	RateLimiterBurst int `mapstructure:"rate_limiter_burst" validate:"required,gt=0"`

	// --- Edge Cache ---

	// EdgeCacheNumCounters is the number of counters used by Ristretto's
	// admission policy. Should be ~10x expected max items.
	// Env: EDGE_CACHE_NUM_COUNTERS
	EdgeCacheNumCounters int64 `mapstructure:"edge_cache_num_counters" validate:"required,gt=0"`

	// EdgeCacheMaxCost is the maximum cache memory budget in bytes.
	// Env: EDGE_CACHE_MAX_COST
	EdgeCacheMaxCost int64 `mapstructure:"edge_cache_max_cost" validate:"required,gt=0"`

	// EdgeCacheBufferItems is Ristretto's internal per-Get buffer size.
	// Env: EDGE_CACHE_BUFFER_ITEMS
	EdgeCacheBufferItems int64 `mapstructure:"edge_cache_buffer_items" validate:"required,gt=0"`

	// --- IDResolver ---

	// IDResolverNegTTL is the TTL for negative cache entries (entity not
	// found). Keeps repeated "not found" lookups from hitting the database.
	// Env: IDR_NEG_TTL
	IDResolverNegTTL time.Duration `mapstructure:"idr_neg_ttl"`

	// IDResolverSingleflight enables request coalescing for concurrent
	// identical FK lookups on cache-miss paths.
	// Env: IDR_SINGLEFLIGHT_ENABLED
	IDResolverSingleflight bool `mapstructure:"idr_singleflight_enabled"`

	// --- Manager ---

	// MetricsBindAddress is the address to bind the metrics endpoint.
	// Env: METRICS_BIND_ADDRESS
	MetricsBindAddress string `mapstructure:"metrics_bind_address" validate:"required"`

	// HealthProbeBindAddress is the address to bind the health probe endpoint.
	// Env: HEALTH_PROBE_BIND_ADDRESS
	HealthProbeBindAddress string `mapstructure:"health_probe_bind_address" validate:"required"`

	// LeaderElection enables leader election for the controller manager.
	// Env: LEADER_ELECTION
	LeaderElection bool `mapstructure:"leader_election"`

	// LeaderElectionID is the name of the leader election resource.
	// Env: LEADER_ELECTION_ID
	LeaderElectionID string `mapstructure:"leader_election_id" validate:"required"`

	// v is the viper instance used for dynamic lookups (e.g. per-module
	// concurrency overrides). Not exported; not part of the serialised config.
	v *viper.Viper
}

// ConcurrencyFor resolves the effective MaxConcurrentReconciles for the given
// module. It checks the viper key max_concurrent_reconciles_{moduleName}
// (env MAX_CONCURRENT_RECONCILES_{UPPER(moduleName)}) first; if that is unset
// or invalid, it falls back to the global MaxConcurrentReconciles.
func (c *Config) ConcurrencyFor(moduleName string) int {
	if c.v != nil {
		key := "max_concurrent_reconciles_" + moduleName
		if v := c.v.GetInt(key); v > 0 {
			return v
		}
	}
	return c.MaxConcurrentReconciles
}

// Load reads configuration from environment variables, falling back to
// defaults for any value that is not set. It returns an error if the
// configuration cannot be parsed or fails validation.
func Load() (*Config, error) {
	v := viper.New()
	setDefaults(v)
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	if err := validator.New().Struct(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	cfg.v = v

	return &cfg, nil
}

// setDefaults registers the default values for every configuration key.
func setDefaults(v *viper.Viper) {
	// Database
	v.SetDefault("database_url", "postgres://localhost:5432/controlplane?sslmode=disable")
	v.SetDefault("db_max_open_conns", 100)
	v.SetDefault("db_max_idle_conns", 30)
	v.SetDefault("db_conn_max_lifetime", "5m")

	// Concurrency
	v.SetDefault("max_concurrent_reconciles", 10)
	v.SetDefault("reconcile_timeout", "7s")

	// Resync
	v.SetDefault("periodic_resync", "0s")

	// Error Policy
	v.SetDefault("dependency_delay", "2s")
	v.SetDefault("dependency_delay_jitter", "3s")
	v.SetDefault("skip_requeue", "5m")

	// Rate Limiter
	v.SetDefault("rate_limiter_base_delay", "5ms")
	v.SetDefault("rate_limiter_max_delay", "1000s")
	v.SetDefault("rate_limiter_qps", 10.0)
	v.SetDefault("rate_limiter_burst", 100)

	// Edge Cache
	v.SetDefault("edge_cache_num_counters", int64(1_000_000))
	v.SetDefault("edge_cache_max_cost", int64(104_857_600))
	v.SetDefault("edge_cache_buffer_items", int64(64))

	// IDResolver
	v.SetDefault("idr_neg_ttl", "5s")
	v.SetDefault("idr_singleflight_enabled", true)

	// Manager
	v.SetDefault("metrics_bind_address", ":8090")
	v.SetDefault("health_probe_bind_address", ":8081")
	v.SetDefault("leader_election", false)
	v.SetDefault("leader_election_id", "projector.cp.ei.telekom.de")
}
