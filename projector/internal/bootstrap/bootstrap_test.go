// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/controlplane-api/ent/migrate"
	"github.com/telekom/controlplane/projector/internal/config"
	"github.com/telekom/controlplane/projector/internal/domain/agentcard"
	"github.com/telekom/controlplane/projector/internal/domain/eventtype"
	"github.com/telekom/controlplane/projector/internal/domain/filetype"
	"github.com/telekom/controlplane/projector/internal/domain/group"
	"github.com/telekom/controlplane/projector/internal/domain/permissionset"
	"github.com/telekom/controlplane/projector/internal/domain/team"
	"github.com/telekom/controlplane/projector/internal/domain/zone"
	"github.com/telekom/controlplane/projector/internal/module"
)

func TestBootstrap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bootstrap Suite")
}

// moduleNames returns the Name() of every module in the slice, for assertions.
func moduleNames(mods []module.Module) []string {
	names := make([]string, len(mods))
	for i, m := range mods {
		names[i] = m.Name()
	}
	return names
}

var _ = Describe("openDatabase", func() {
	var (
		ctx context.Context
		cfg *config.Config
	)

	BeforeEach(func() {
		url := os.Getenv("CP_TEST_POSTGRES_URL")
		if url == "" {
			Skip("set CP_TEST_POSTGRES_URL to run PostgreSQL integration tests")
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), time.Minute)
		DeferCleanup(cancel)
		cfg = &config.Config{
			DatabaseURL:      url,
			EnvironmentFiler: fmt.Sprintf("projector-test-%d", time.Now().UnixNano()),
			MaxOpenConns:     2,
			MaxIdleConns:     1,
			ConnMaxLifetime:  time.Minute,
		}
	})

	It("creates the environment schema before migration and reuses it on restart", func() {
		for i := range 2 {
			db, err := openDatabase(ctx, cfg)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { Expect(db.Close()).To(Succeed()) })
			if i == 0 {
				DeferCleanup(func() {
					_, err := db.ExecContext(ctx, "DROP SCHEMA "+pgx.Identifier{cfg.EnvironmentFiler}.Sanitize()+" CASCADE")
					Expect(err).NotTo(HaveOccurred())
				})
			}

			var schemaName string
			Expect(db.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schemaName)).To(Succeed())
			Expect(schemaName).To(Equal(cfg.EnvironmentFiler))
			Expect(migrate.NewSchema(entsql.OpenDB(dialect.Postgres, db)).Create(ctx,
				migrate.WithGlobalUniqueID(true),
				migrate.WithDropIndex(true),
				migrate.WithDropColumn(true),
			)).To(Succeed())
			var count int
			Expect(db.QueryRowContext(ctx,
				"SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = 'zones'",
				cfg.EnvironmentFiler).Scan(&count)).To(Succeed())
			Expect(count).To(Equal(1))
		}
	})

	It("uses public with the default search_path when the environment filter is unset", func() {
		cfg.EnvironmentFiler = ""
		db, err := openDatabase(ctx, cfg)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(db.Close()).To(Succeed()) })

		var schemaName string
		Expect(db.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schemaName)).To(Succeed())
		Expect(schemaName).To(Equal("public"))
	})

	It("returns schema creation failures instead of continuing to migration", func() {
		cfg.EnvironmentFiler = "pg_projector_test"
		db, err := openDatabase(ctx, cfg)
		Expect(db).To(BeNil())
		Expect(err).To(MatchError(ContainSubstring(`creating PostgreSQL schema "pg_projector_test"`)))
	})
})

var _ = Describe("registerSchemesAndModules", func() {
	var (
		originalPermission bool
		originalPubSub     bool
		originalFile       bool
		originalAiGateway  bool
		baseModules        []module.Module
	)

	BeforeEach(func() {
		originalPermission = cconfig.FeaturePermission.IsEnabled()
		originalPubSub = cconfig.FeaturePubSub.IsEnabled()
		originalFile = cconfig.FeatureFile.IsEnabled()
		originalAiGateway = cconfig.FeatureAiGateway.IsEnabled()
		baseModules = []module.Module{zone.Module, group.Module, team.Module}
	})

	AfterEach(func() {
		cconfig.SetFeatureEnabled(cconfig.FeaturePermission, originalPermission)
		cconfig.SetFeatureEnabled(cconfig.FeaturePubSub, originalPubSub)
		cconfig.SetFeatureEnabled(cconfig.FeatureFile, originalFile)
		cconfig.SetFeatureEnabled(cconfig.FeatureAiGateway, originalAiGateway)
	})

	It("should not register the permissionset module when FeaturePermission is disabled", func() {
		cconfig.SetFeatureEnabled(cconfig.FeaturePermission, false)
		cconfig.SetFeatureEnabled(cconfig.FeaturePubSub, false)

		result := registerSchemesAndModules(runtime.NewScheme(), append([]module.Module{}, baseModules...))

		Expect(moduleNames(result)).NotTo(ContainElement(permissionset.Module.Name()))
	})

	It("should register the permissionset module when FeaturePermission is enabled", func() {
		cconfig.SetFeatureEnabled(cconfig.FeaturePermission, true)

		result := registerSchemesAndModules(runtime.NewScheme(), append([]module.Module{}, baseModules...))

		Expect(moduleNames(result)).To(ContainElement(permissionset.Module.Name()))
	})

	It("should not register the agentic modules when FeatureAiGateway is disabled", func() {
		cconfig.SetFeatureEnabled(cconfig.FeatureAiGateway, false)

		result := registerSchemesAndModules(runtime.NewScheme(), append([]module.Module{}, baseModules...))

		Expect(moduleNames(result)).NotTo(ContainElement(agentcard.Module.Name()))
	})

	It("should register the agentic modules when FeatureAiGateway is enabled", func() {
		cconfig.SetFeatureEnabled(cconfig.FeatureAiGateway, true)

		result := registerSchemesAndModules(runtime.NewScheme(), append([]module.Module{}, baseModules...))

		Expect(moduleNames(result)).To(ContainElements(
			"mcpserver", "agentcard", "agenticexposure", "agenticsubscription",
		))
	})

	DescribeTable("feature flag matrix",
		func(pubSubEnabled, fileEnabled, permissionEnabled, aiGatewayEnabled bool) {
			cconfig.SetFeatureEnabled(cconfig.FeaturePubSub, pubSubEnabled)
			cconfig.SetFeatureEnabled(cconfig.FeatureFile, fileEnabled)
			cconfig.SetFeatureEnabled(cconfig.FeaturePermission, permissionEnabled)
			cconfig.SetFeatureEnabled(cconfig.FeatureAiGateway, aiGatewayEnabled)

			result := registerSchemesAndModules(runtime.NewScheme(), baseModules)

			names := moduleNames(result)
			Expect(names).To(ContainElements(zone.Module.Name(), group.Module.Name(), team.Module.Name()),
				"base modules must always be present")

			if pubSubEnabled {
				Expect(names).To(ContainElement(eventtype.Module.Name()))
			} else {
				Expect(names).NotTo(ContainElement(eventtype.Module.Name()))
			}

			if fileEnabled {
				Expect(names).To(ContainElement(filetype.Module.Name()))
			} else {
				Expect(names).NotTo(ContainElement(filetype.Module.Name()))
			}

			if permissionEnabled {
				Expect(names).To(ContainElement(permissionset.Module.Name()))
			} else {
				Expect(names).NotTo(ContainElement(permissionset.Module.Name()))
			}

			if aiGatewayEnabled {
				Expect(names).To(ContainElement(agentcard.Module.Name()))
			} else {
				Expect(names).NotTo(ContainElement(agentcard.Module.Name()))
			}

			// baseModules must not be mutated by the append inside registerSchemesAndModules.
			Expect(baseModules).To(HaveLen(3))
			Expect(moduleNames(baseModules)).To(Equal([]string{zone.Module.Name(), group.Module.Name(), team.Module.Name()}))
		},
		Entry("all disabled", false, false, false, false),
		Entry("pubsub only", true, false, false, false),
		Entry("file only", false, true, false, false),
		Entry("permission only", false, false, true, false),
		Entry("ai_gateway only", false, false, false, true),
		Entry("all enabled", true, true, true, true),
	)
})
