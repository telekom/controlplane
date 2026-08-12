// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filetype_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	entfiletype "github.com/telekom/controlplane/controlplane-api/ent/filetype"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/projector/internal/domain/filetype"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

type mockFileTypeDeps struct {
	teamIDs map[string]int
	teamErr error
}

func (m *mockFileTypeDeps) FindTeamID(_ context.Context, name string) (int, error) {
	if m.teamErr != nil {
		return 0, m.teamErr
	}
	if id, ok := m.teamIDs[name]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("team %q: %w", name, infrastructure.ErrEntityNotFound)
}

func skipIfSQLiteUnavailable() {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		Skip(fmt.Sprintf("sqlite3 is unavailable in this environment: %v", err))
	}
	defer func() {
		_ = db.Close()
	}()

	if err := db.Ping(); err != nil {
		Skip(fmt.Sprintf("sqlite3 is unavailable in this environment: %v", err))
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		Skip(fmt.Sprintf("sqlite3 is unavailable in this environment (likely CGO disabled): %v", err))
	}
}

var _ = Describe("FileType Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		deps   *mockFileTypeDeps
		repo   *filetype.Repository
		ctx    context.Context
		teamID int
	)

	BeforeEach(func() {
		skipIfSQLiteUnavailable()
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")

		team, err := client.Team.Create().SetName("platform--narvi").SetEmail("narvi@example.com").SetNamespace("platform--narvi").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		teamID = team.ID

		deps = &mockFileTypeDeps{teamIDs: map[string]int{"platform--narvi": teamID}}
		repo = filetype.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create file type with optional fields", func() {
			variant := "csv"
			sftpName := "sftp-a"
			sftpNs := "sftp-ns"
			data := &filetype.FileTypeData{
				Meta:                  shared.NewMetadata("prod--platform--narvi", "invoice", nil),
				StatusPhase:           "READY",
				StatusMessage:         "ok",
				FileType:              "invoice",
				Description:           "Invoice files",
				Variant:               &variant,
				Active:                true,
				SFTPInstanceName:      &sftpName,
				SFTPInstanceNamespace: &sftpNs,
				TeamName:              "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			ft, err := client.FileType.Query().Where(entfiletype.FileTypeEQ("invoice")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ft.Description).To(Equal("Invoice files"))
			Expect(ft.Variant).NotTo(BeNil())
			Expect(*ft.Variant).To(Equal("csv"))
			Expect(ft.Active).NotTo(BeNil())
			Expect(ft.Active).To(BeTrue())
			Expect(ft.SftpInstanceName).NotTo(BeNil())
			Expect(*ft.SftpInstanceName).To(Equal("sftp-a"))

			owner, err := ft.QueryOwner().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.ID).To(Equal(teamID))
		})

		It("should return dependency missing when team is missing", func() {
			err := repo.Upsert(ctx, &filetype.FileTypeData{Meta: shared.NewMetadata("prod--unknown", "invoice", nil), FileType: "invoice", TeamName: "unknown"})
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
		})

		It("should propagate non-not-found team errors", func() {
			dbErr := errors.New("resolver timeout")
			failRepo := filetype.NewRepository(client, cache, &mockFileTypeDeps{teamErr: dbErr})
			err := failRepo.Upsert(ctx, &filetype.FileTypeData{Meta: shared.NewMetadata("prod--platform--narvi", "invoice", nil), FileType: "invoice", TeamName: "platform--narvi"})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should populate cache entries after upsert", func() {
			data := &filetype.FileTypeData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "invoice", nil),
				StatusPhase: "READY",
				FileType:    "invoice",
				Description: "Invoice files",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			id, found := cache.Get("filetype", "invoice")
			Expect(found).To(BeTrue())
			Expect(id).To(BeNumerically(">", 0))

			_, activeFound := cache.Get("filetype_active", "invoice")
			Expect(activeFound).To(BeTrue())
		})
	})

	Describe("Delete", func() {
		It("should delete by file type and evict cache", func() {
			data := &filetype.FileTypeData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "invoice", nil),
				StatusPhase: "READY",
				FileType:    "invoice",
				Description: "Invoice files",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			Expect(repo.Delete(ctx, filetype.FileTypeKey{FileType: "invoice"})).To(Succeed())
			cache.Wait()

			count, err := client.FileType.Query().Where(entfiletype.FileTypeEQ("invoice")).Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))

			_, found := cache.Get("filetype", "invoice")
			Expect(found).To(BeFalse())
		})

		It("should be idempotent for missing file type", func() {
			Expect(repo.Delete(ctx, filetype.FileTypeKey{FileType: "missing"})).To(Succeed())
		})
	})
})
