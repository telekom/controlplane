// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"
	"time"

	"github.com/go-playground/validator/v10"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

var _ = Describe("Load", func() {
	It("returns defaults when no env vars are set", func() {
		cfg, err := Load()
		Expect(err).NotTo(HaveOccurred())

		// Database
		Expect(cfg.DatabaseURL).To(Equal("postgres://localhost:5432/controlplane?sslmode=disable"))
		Expect(cfg.MaxOpenConns).To(Equal(100))
		Expect(cfg.MaxIdleConns).To(Equal(30))
		Expect(cfg.ConnMaxLifetime).To(Equal(5 * time.Minute))

		// Concurrency
		Expect(cfg.MaxConcurrentReconciles).To(Equal(10))
		Expect(cfg.ReconcileTimeout).To(Equal(7 * time.Second))

		// Resync
		Expect(cfg.PeriodicResync).To(Equal(time.Duration(0)))

		// Error Policy
		Expect(cfg.DependencyDelay).To(Equal(2 * time.Second))
		Expect(cfg.DependencyDelayJitter).To(Equal(3 * time.Second))
		Expect(cfg.SkipRequeue).To(Equal(5 * time.Minute))

		// Rate Limiter
		Expect(cfg.RateLimiterBaseDelay).To(Equal(5 * time.Millisecond))
		Expect(cfg.RateLimiterMaxDelay).To(Equal(1000 * time.Second))
		Expect(cfg.RateLimiterQPS).To(Equal(10.0))
		Expect(cfg.RateLimiterBurst).To(Equal(100))

		// Edge Cache
		Expect(cfg.EdgeCacheNumCounters).To(Equal(int64(1_000_000)))
		Expect(cfg.EdgeCacheMaxCost).To(Equal(int64(104_857_600)))
		Expect(cfg.EdgeCacheBufferItems).To(Equal(int64(64)))

		// IDResolver
		Expect(cfg.IDResolverNegTTL).To(Equal(5 * time.Second))
		Expect(cfg.IDResolverSingleflight).To(BeTrue())

		// Manager
		Expect(cfg.MetricsBindAddress).To(Equal(":8090"))
		Expect(cfg.HealthProbeBindAddress).To(Equal(":8081"))
		Expect(cfg.LeaderElection).To(BeFalse())
		Expect(cfg.LeaderElectionID).To(Equal("projector.cp.ei.telekom.de"))
	})

	It("reads values from environment variables", func() {
		// Database
		GinkgoT().Setenv("DATABASE_URL", "postgres://test:test@db:5432/test")
		GinkgoT().Setenv("DB_MAX_OPEN_CONNS", "50")
		GinkgoT().Setenv("DB_MAX_IDLE_CONNS", "20")
		GinkgoT().Setenv("DB_CONN_MAX_LIFETIME", "2m")

		// Concurrency
		GinkgoT().Setenv("MAX_CONCURRENT_RECONCILES", "5")
		GinkgoT().Setenv("RECONCILE_TIMEOUT", "15s")

		// Resync
		GinkgoT().Setenv("PERIODIC_RESYNC", "30s")

		// Error Policy
		GinkgoT().Setenv("DEPENDENCY_DELAY", "5s")
		GinkgoT().Setenv("DEPENDENCY_DELAY_JITTER", "10s")
		GinkgoT().Setenv("SKIP_REQUEUE", "10m")

		// Rate Limiter
		GinkgoT().Setenv("RATE_LIMITER_BASE_DELAY", "10ms")
		GinkgoT().Setenv("RATE_LIMITER_MAX_DELAY", "500s")
		GinkgoT().Setenv("RATE_LIMITER_QPS", "20.5")
		GinkgoT().Setenv("RATE_LIMITER_BURST", "200")

		// Edge Cache
		GinkgoT().Setenv("EDGE_CACHE_NUM_COUNTERS", "2000000")
		GinkgoT().Setenv("EDGE_CACHE_MAX_COST", "209715200")
		GinkgoT().Setenv("EDGE_CACHE_BUFFER_ITEMS", "128")

		// IDResolver
		GinkgoT().Setenv("IDR_NEG_TTL", "10s")
		GinkgoT().Setenv("IDR_SINGLEFLIGHT_ENABLED", "false")

		// Manager
		GinkgoT().Setenv("METRICS_BIND_ADDRESS", ":9090")
		GinkgoT().Setenv("HEALTH_PROBE_BIND_ADDRESS", ":9091")
		GinkgoT().Setenv("LEADER_ELECTION", "true")
		GinkgoT().Setenv("LEADER_ELECTION_ID", "custom-id")

		cfg, err := Load()
		Expect(err).NotTo(HaveOccurred())

		// Database
		Expect(cfg.DatabaseURL).To(Equal("postgres://test:test@db:5432/test"))
		Expect(cfg.MaxOpenConns).To(Equal(50))
		Expect(cfg.MaxIdleConns).To(Equal(20))
		Expect(cfg.ConnMaxLifetime).To(Equal(2 * time.Minute))

		// Concurrency
		Expect(cfg.MaxConcurrentReconciles).To(Equal(5))
		Expect(cfg.ReconcileTimeout).To(Equal(15 * time.Second))

		// Resync
		Expect(cfg.PeriodicResync).To(Equal(30 * time.Second))

		// Error Policy
		Expect(cfg.DependencyDelay).To(Equal(5 * time.Second))
		Expect(cfg.DependencyDelayJitter).To(Equal(10 * time.Second))
		Expect(cfg.SkipRequeue).To(Equal(10 * time.Minute))

		// Rate Limiter
		Expect(cfg.RateLimiterBaseDelay).To(Equal(10 * time.Millisecond))
		Expect(cfg.RateLimiterMaxDelay).To(Equal(500 * time.Second))
		Expect(cfg.RateLimiterQPS).To(Equal(20.5))
		Expect(cfg.RateLimiterBurst).To(Equal(200))

		// Edge Cache
		Expect(cfg.EdgeCacheNumCounters).To(Equal(int64(2_000_000)))
		Expect(cfg.EdgeCacheMaxCost).To(Equal(int64(209_715_200)))
		Expect(cfg.EdgeCacheBufferItems).To(Equal(int64(128)))

		// IDResolver
		Expect(cfg.IDResolverNegTTL).To(Equal(10 * time.Second))
		Expect(cfg.IDResolverSingleflight).To(BeFalse())

		// Manager
		Expect(cfg.MetricsBindAddress).To(Equal(":9090"))
		Expect(cfg.HealthProbeBindAddress).To(Equal(":9091"))
		Expect(cfg.LeaderElection).To(BeTrue())
		Expect(cfg.LeaderElectionID).To(Equal("custom-id"))
	})

	It("falls back to default when env var is set to empty string", func() {
		// Viper treats empty env vars as unset, so defaults apply.
		GinkgoT().Setenv("LEADER_ELECTION_ID", "")

		cfg, err := Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.LeaderElectionID).To(Equal("projector.cp.ei.telekom.de"))
	})

	It("returns a validation error when a numeric field is zero", func() {
		GinkgoT().Setenv("DB_MAX_OPEN_CONNS", "0")

		_, err := Load()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("validating config"))
		Expect(err.Error()).To(ContainSubstring("MaxOpenConns"))
	})

	It("returns a validation error when a numeric field is negative", func() {
		GinkgoT().Setenv("DB_MAX_IDLE_CONNS", "-1")

		_, err := Load()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("validating config"))
		Expect(err.Error()).To(ContainSubstring("MaxIdleConns"))
	})

	It("validates the struct correctly", func() {
		validate := validator.New()

		invalid := Config{
			DatabaseURL:    "",
			MaxOpenConns:   0,
			LeaderElection: false,
		}
		err := validate.Struct(invalid)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("DatabaseURL"))
		Expect(err.Error()).To(ContainSubstring("MaxOpenConns"))
		Expect(err.Error()).To(ContainSubstring("LeaderElectionID"))
	})

	It("accepts PeriodicResync of zero (event-driven)", func() {
		GinkgoT().Setenv("PERIODIC_RESYNC", "0s")

		cfg, err := Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.PeriodicResync).To(Equal(time.Duration(0)))
	})

	It("accepts DependencyDelayJitter of zero (no jitter)", func() {
		GinkgoT().Setenv("DEPENDENCY_DELAY_JITTER", "0s")

		cfg, err := Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.DependencyDelayJitter).To(Equal(time.Duration(0)))
	})
})

var _ = Describe("ConcurrencyFor", func() {
	var cfg *Config

	BeforeEach(func() {
		var err error
		cfg, err = Load()
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns the global default when no per-module override is set", func() {
		Expect(cfg.ConcurrencyFor("zone")).To(Equal(10))
	})

	It("returns the per-module override when set", func() {
		GinkgoT().Setenv("MAX_CONCURRENT_RECONCILES_ZONE", "20")

		Expect(cfg.ConcurrencyFor("zone")).To(Equal(20))
	})

	It("uppercases the module name for the env var lookup", func() {
		GinkgoT().Setenv("MAX_CONCURRENT_RECONCILES_APISUBSCRIPTION", "15")

		Expect(cfg.ConcurrencyFor("apisubscription")).To(Equal(15))
	})

	It("falls back to global when per-module override is invalid", func() {
		GinkgoT().Setenv("MAX_CONCURRENT_RECONCILES_ZONE", "not-a-number")

		Expect(cfg.ConcurrencyFor("zone")).To(Equal(10))
	})

	It("falls back to global when per-module override is zero", func() {
		GinkgoT().Setenv("MAX_CONCURRENT_RECONCILES_ZONE", "0")

		Expect(cfg.ConcurrencyFor("zone")).To(Equal(10))
	})

	It("falls back to global when per-module override is negative", func() {
		GinkgoT().Setenv("MAX_CONCURRENT_RECONCILES_ZONE", "-5")

		Expect(cfg.ConcurrencyFor("zone")).To(Equal(10))
	})

	It("uses the overridden global default", func() {
		GinkgoT().Setenv("MAX_CONCURRENT_RECONCILES", "3")

		cfg, err := Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.ConcurrencyFor("team")).To(Equal(3))
	})

	It("per-module override takes precedence over overridden global", func() {
		GinkgoT().Setenv("MAX_CONCURRENT_RECONCILES", "3")
		GinkgoT().Setenv("MAX_CONCURRENT_RECONCILES_TEAM", "7")

		cfg, err := Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.ConcurrencyFor("team")).To(Equal(7))
	})
})
