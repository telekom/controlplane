// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fileexposure_test

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	entfileexposure "github.com/telekom/controlplane/controlplane-api/ent/fileexposure"
	entfilesubscription "github.com/telekom/controlplane/controlplane-api/ent/filesubscription"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/fileexposure"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

type mockFileExposureDeps struct {
	appIDs      map[string]int
	zoneIDs     map[string]int
	fileTypeIDs map[string]int
	appErr      error
	zoneErr     error
	fileTypeErr error
}

func (m *mockFileExposureDeps) FindApplicationID(_ context.Context, name, teamName string) (int, error) {
	if m.appErr != nil {
		return 0, m.appErr
	}
	if id, ok := m.appIDs[name+":"+teamName]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("application %q (team %q): %w", name, teamName, infrastructure.ErrEntityNotFound)
}

func (m *mockFileExposureDeps) FindZoneID(_ context.Context, name string) (int, error) {
	if m.zoneErr != nil {
		return 0, m.zoneErr
	}
	if id, ok := m.zoneIDs[name]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("zone %q: %w", name, infrastructure.ErrEntityNotFound)
}

func (m *mockFileExposureDeps) FindFileTypeID(_ context.Context, fileType string) (int, error) {
	if m.fileTypeErr != nil {
		return 0, m.fileTypeErr
	}
	if id, ok := m.fileTypeIDs[fileType]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("file_type %q: %w", fileType, infrastructure.ErrEntityNotFound)
}

var _ = Describe("FileExposure Repository", func() {
	var (
		client     *ent.Client
		cache      *infrastructure.EdgeCache
		deps       *mockFileExposureDeps
		repo       *fileexposure.Repository
		ctx        context.Context
		appID      int
		zoneID     int
		fileTypeID int
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")

		z, err := client.Zone.Create().
			SetName("caas").
			SetVisibility(zone.VisibilityEnterprise).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		zoneID = z.ID

		team, err := client.Team.Create().
			SetName("platform--narvi").
			SetEmail("narvi@example.com").
			SetNamespace("platform--narvi").
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		app, err := client.Application.Create().
			SetName("provider-app").
			SetNamespace("prod--platform--narvi").
			SetOwnerTeamID(team.ID).
			SetZoneID(zoneID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		appID = app.ID

		ft, err := client.FileType.Create().
			SetFileType("invoice").
			SetDescription("Invoice files").
			SetNamespace("prod--platform--narvi").
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		fileTypeID = ft.ID

		deps = &mockFileExposureDeps{
			appIDs:      map[string]int{"provider-app:platform--narvi": appID},
			zoneIDs:     map[string]int{"caas": zoneID},
			fileTypeIDs: map[string]int{"invoice": fileTypeID},
		}
		repo = fileexposure.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create file exposure and set optional file type edge", func() {
			provider := "provider-app"
			zoneNamespace := "zone-ns"
			data := &fileexposure.FileExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-a", nil),
				StatusPhase:    "READY",
				StatusMessage:  "ok",
				Provider:       &provider,
				Visibility:     "ENTERPRISE",
				Active:         true,
				ZoneName:       "caas",
				ZoneNamespace:  &zoneNamespace,
				SFTPPublicKeys: []string{"ssh-rsa AAA"},
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO", TrustedTeams: []string{"team-a"}},
				AppName:        "provider-app",
				TeamName:       "platform--narvi",
				TargetFileType: "invoice",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.FileExposure.Query().Where(entfileexposure.FileTypeEQ("invoice")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.Visibility.String()).To(Equal("ENTERPRISE"))
			Expect(exp.Active).NotTo(BeNil())
			Expect(*exp.Active).To(BeTrue())
			Expect(exp.ZoneName).To(Equal("caas"))
			Expect(exp.ZoneNamespace).NotTo(BeNil())
			Expect(*exp.ZoneNamespace).To(Equal("zone-ns"))
			Expect(exp.SftpPublicKeys).To(Equal([]string{"ssh-rsa AAA"}))
			Expect(exp.ApprovalConfig.Strategy).To(Equal("AUTO"))

			owner, err := exp.QueryOwner().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.ID).To(Equal(appID))

			expZone, err := exp.QueryZone().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(expZone.ID).To(Equal(zoneID))

			ft, err := exp.QueryFileTypeDef().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ft.ID).To(Equal(fileTypeID))
		})

		It("should not fail when target file type is missing", func() {
			data := &fileexposure.FileExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-b", nil),
				StatusPhase:    "READY",
				Visibility:     "WORLD",
				ZoneName:       "caas",
				ApprovalConfig: model.ApprovalConfig{Strategy: "SIMPLE"},
				AppName:        "provider-app",
				TeamName:       "platform--narvi",
				TargetFileType: "unknown-filetype",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.FileExposure.Query().Where(entfileexposure.FileTypeEQ("unknown-filetype")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			hasFT, err := exp.QueryFileTypeDef().Exist(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasFT).To(BeFalse())
		})

		It("should return dependency missing when application is missing", func() {
			data := &fileexposure.FileExposureData{Meta: shared.NewMetadata("prod--platform--narvi", "exp-c", nil), ZoneName: "caas", AppName: "missing", TeamName: "platform--narvi", TargetFileType: "invoice"}
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
		})

		It("should propagate non-not-found application errors", func() {
			dbErr := errors.New("db down")
			failRepo := fileexposure.NewRepository(client, cache, &mockFileExposureDeps{appErr: dbErr})
			err := failRepo.Upsert(ctx, &fileexposure.FileExposureData{Meta: shared.NewMetadata("prod--platform--narvi", "exp-d", nil), ZoneName: "caas", AppName: "provider-app", TeamName: "platform--narvi", TargetFileType: "invoice"})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should populate cache entries", func() {
			data := &fileexposure.FileExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-e", nil),
				StatusPhase:    "READY",
				Visibility:     "ZONE",
				Active:         true,
				ZoneName:       "caas",
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "provider-app",
				TeamName:       "platform--narvi",
				TargetFileType: "invoice",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			id, found := cache.Get("fileexposure", "invoice:provider-app:platform--narvi")
			Expect(found).To(BeTrue())
			Expect(id).To(BeNumerically(">", 0))

			_, activeFound := cache.Get("fileexposure_active", "invoice")
			Expect(activeFound).To(BeTrue())
		})

		It("should back-link orphaned subscriptions when exposure is active", func() {
			sub, err := client.FileSubscription.Create().
				SetFileType("invoice").
				SetZoneName("caas").
				SetEnvironment("prod").
				SetNamespace("prod--platform--narvi").
				SetName("orphan-sub").
				SetOwnerID(appID).
				SetZoneID(zoneID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = sub.QueryTarget().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())

			data := &fileexposure.FileExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "orphan-exp", nil),
				StatusPhase:    "READY",
				Visibility:     "WORLD",
				Active:         true,
				ZoneName:       "caas",
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "provider-app",
				TeamName:       "platform--narvi",
				TargetFileType: "invoice",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			target, err := sub.QueryTarget().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(target.FileType).To(Equal("invoice"))
		})

		It("should not back-link orphaned subscriptions when exposure is inactive", func() {
			sub, err := client.FileSubscription.Create().
				SetFileType("orders").
				SetZoneName("caas").
				SetEnvironment("prod").
				SetNamespace("prod--platform--narvi").
				SetName("inactive-sub").
				SetOwnerID(appID).
				SetZoneID(zoneID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			data := &fileexposure.FileExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "inactive-exp", nil),
				StatusPhase:    "READY",
				Visibility:     "WORLD",
				Active:         false,
				ZoneName:       "caas",
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "provider-app",
				TeamName:       "platform--narvi",
				TargetFileType: "orders",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			_, err = sub.QueryTarget().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
			count, err := client.FileSubscription.Query().Where(entfilesubscription.FileTypeEQ("orders")).Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))
		})
	})

	Describe("Delete", func() {
		It("should delete existing exposure and evict cache", func() {
			data := &fileexposure.FileExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-del", nil),
				StatusPhase:    "READY",
				Visibility:     "ZONE",
				Active:         true,
				ZoneName:       "caas",
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "provider-app",
				TeamName:       "platform--narvi",
				TargetFileType: "invoice",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			key := fileexposure.FileExposureKey{FileType: "invoice", AppName: "provider-app", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())
			cache.Wait()

			count, err := client.FileExposure.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))

			_, found := cache.Get("fileexposure", "invoice:provider-app:platform--narvi")
			Expect(found).To(BeFalse())
		})

		It("should be idempotent for missing row", func() {
			Expect(repo.Delete(ctx, fileexposure.FileExposureKey{FileType: "missing", AppName: "provider-app", TeamName: "platform--narvi"})).To(Succeed())
		})
	})
})
