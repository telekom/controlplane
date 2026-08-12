// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filetype_test

import (
	"context"

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
)

var _ = Describe("FileType Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		repo   *filetype.Repository
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")
		repo = filetype.NewRepository(client, cache)
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
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			ft, err := client.FileType.Query().Where(entfiletype.FileTypeEQ("invoice")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ft.Description).To(Equal("Invoice files"))
			Expect(ft.Variant).NotTo(BeNil())
			Expect(*ft.Variant).To(Equal("csv"))
			Expect(ft.Active).To(BeTrue())
			Expect(ft.SftpInstanceName).NotTo(BeNil())
			Expect(*ft.SftpInstanceName).To(Equal("sftp-a"))
		})

		It("should populate cache entries after upsert", func() {
			data := &filetype.FileTypeData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "invoice", nil),
				StatusPhase: "READY",
				FileType:    "invoice",
				Description: "Invoice files",
				Active:      true,
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
