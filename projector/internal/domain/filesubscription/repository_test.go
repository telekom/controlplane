// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filesubscription_test

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
	entfilesubscription "github.com/telekom/controlplane/controlplane-api/ent/filesubscription"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/projector/internal/domain/filesubscription"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

type mockFileSubscriptionDeps struct {
	appIDs      map[string]int
	zoneIDs     map[string]int
	fileTypeIDs map[string]int
	exposureIDs map[string]int
	appErr      error
	zoneErr     error
	fileTypeErr error
	exposureErr error
}

func (m *mockFileSubscriptionDeps) FindApplicationID(_ context.Context, name, teamName string) (int, error) {
	if m.appErr != nil {
		return 0, m.appErr
	}
	if id, ok := m.appIDs[name+":"+teamName]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("application %q (team %q): %w", name, teamName, infrastructure.ErrEntityNotFound)
}

func (m *mockFileSubscriptionDeps) FindZoneID(_ context.Context, name string) (int, error) {
	if m.zoneErr != nil {
		return 0, m.zoneErr
	}
	if id, ok := m.zoneIDs[name]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("zone %q: %w", name, infrastructure.ErrEntityNotFound)
}

func (m *mockFileSubscriptionDeps) FindFileTypeID(_ context.Context, fileType string) (int, error) {
	if m.fileTypeErr != nil {
		return 0, m.fileTypeErr
	}
	if id, ok := m.fileTypeIDs[fileType]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("file_type %q: %w", fileType, infrastructure.ErrEntityNotFound)
}

func (m *mockFileSubscriptionDeps) FindActiveFileExposureByFileType(_ context.Context, fileType string) (int, error) {
	if m.exposureErr != nil {
		return 0, m.exposureErr
	}
	if id, ok := m.exposureIDs[fileType]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("active file_exposure %q: %w", fileType, infrastructure.ErrEntityNotFound)
}

var _ = Describe("FileSubscription Repository", func() {
	var (
		client     *ent.Client
		cache      *infrastructure.EdgeCache
		deps       *mockFileSubscriptionDeps
		repo       *filesubscription.Repository
		ctx        context.Context
		appID      int
		zoneID     int
		fileTypeID int
		exposureID int
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")

		z, err := client.Zone.Create().SetName("caas").SetVisibility(zone.VisibilityEnterprise).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		zoneID = z.ID

		team, err := client.Team.Create().SetName("platform--narvi").SetEmail("narvi@example.com").SetNamespace("platform--narvi").Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		app, err := client.Application.Create().SetName("consumer-app").SetNamespace("prod--platform--narvi").SetOwnerTeamID(team.ID).SetZoneID(zoneID).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		appID = app.ID

		ft, err := client.FileType.Create().SetFileType("invoice").SetDescription("Invoice files").SetNamespace("prod--platform--narvi").SetOwnerID(team.ID).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		fileTypeID = ft.ID

		exp, err := client.FileExposure.Create().SetFileType("invoice").SetVisibility("WORLD").SetZoneName("caas").SetNamespace("prod--platform--narvi").SetOwnerID(appID).SetZoneID(zoneID).SetFileTypeDefID(fileTypeID).SetActive(true).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		exposureID = exp.ID

		deps = &mockFileSubscriptionDeps{
			appIDs:      map[string]int{"consumer-app:platform--narvi": appID},
			zoneIDs:     map[string]int{"caas": zoneID},
			fileTypeIDs: map[string]int{"invoice": fileTypeID},
			exposureIDs: map[string]int{"invoice": exposureID},
		}
		repo = filesubscription.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create subscription and link optional edges", func() {
			zoneNamespace := "zone-ns"
			data := &filesubscription.FileSubscriptionData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "sub-a", nil),
				StatusPhase:    "READY",
				StatusMessage:  "ok",
				ZoneName:       "caas",
				ZoneNamespace:  &zoneNamespace,
				SFTPPublicKeys: []string{"ssh-rsa AAA"},
				OwnerAppName:   "consumer-app",
				OwnerTeamName:  "platform--narvi",
				TargetFileType: "invoice",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			sub, err := client.FileSubscription.Query().Where(entfilesubscription.FileTypeEQ("invoice")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sub.ZoneName).To(Equal("caas"))
			Expect(sub.ZoneNamespace).NotTo(BeNil())
			Expect(*sub.ZoneNamespace).To(Equal("zone-ns"))
			Expect(sub.SftpPublicKeys).To(Equal([]string{"ssh-rsa AAA"}))

			owner, err := sub.QueryOwner().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.ID).To(Equal(appID))

			zoneNode, err := sub.QueryZone().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(zoneNode.ID).To(Equal(zoneID))

			ft, err := sub.QueryFileTypeDef().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ft.ID).To(Equal(fileTypeID))

			target, err := sub.QueryTarget().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(target.ID).To(Equal(exposureID))
		})

		It("should create subscription even if optional edges are unresolved", func() {
			data := &filesubscription.FileSubscriptionData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "sub-b", nil),
				StatusPhase:    "READY",
				ZoneName:       "caas",
				OwnerAppName:   "consumer-app",
				OwnerTeamName:  "platform--narvi",
				TargetFileType: "unknown",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			sub, err := client.FileSubscription.Query().Where(entfilesubscription.FileTypeEQ("unknown")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			hasFT, err := sub.QueryFileTypeDef().Exist(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasFT).To(BeFalse())
			hasTarget, err := sub.QueryTarget().Exist(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasTarget).To(BeFalse())
		})

		It("should return dependency missing when application is missing", func() {
			err := repo.Upsert(ctx, &filesubscription.FileSubscriptionData{Meta: shared.NewMetadata("prod--platform--narvi", "sub-c", nil), ZoneName: "caas", OwnerAppName: "missing", OwnerTeamName: "platform--narvi", TargetFileType: "invoice"})
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
		})

		It("should propagate non-not-found app resolver errors", func() {
			dbErr := errors.New("resolver unavailable")
			failRepo := filesubscription.NewRepository(client, cache, &mockFileSubscriptionDeps{appErr: dbErr})
			err := failRepo.Upsert(ctx, &filesubscription.FileSubscriptionData{Meta: shared.NewMetadata("prod--platform--narvi", "sub-d", nil), ZoneName: "caas", OwnerAppName: "consumer-app", OwnerTeamName: "platform--narvi", TargetFileType: "invoice"})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should populate metadata cache on upsert", func() {
			data := &filesubscription.FileSubscriptionData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "sub-cache", nil),
				StatusPhase:    "READY",
				ZoneName:       "caas",
				OwnerAppName:   "consumer-app",
				OwnerTeamName:  "platform--narvi",
				TargetFileType: "invoice",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			id, found := cache.Get("filesubscription", "meta:prod--platform--narvi:sub-cache")
			Expect(found).To(BeTrue())
			Expect(id).To(BeNumerically(">", 0))
		})
	})

	Describe("Delete", func() {
		It("should delete subscription and evict metadata cache", func() {
			data := &filesubscription.FileSubscriptionData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "sub-del", nil),
				StatusPhase:    "READY",
				ZoneName:       "caas",
				OwnerAppName:   "consumer-app",
				OwnerTeamName:  "platform--narvi",
				TargetFileType: "invoice",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			key := filesubscription.FileSubscriptionKey{FileType: "invoice", OwnerAppName: "consumer-app", OwnerTeamName: "platform--narvi", Namespace: "prod--platform--narvi", Name: "sub-del"}
			Expect(repo.Delete(ctx, key)).To(Succeed())
			cache.Wait()

			count, err := client.FileSubscription.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))

			_, found := cache.Get("filesubscription", "meta:prod--platform--narvi:sub-del")
			Expect(found).To(BeFalse())
		})

		It("should be idempotent for missing subscription", func() {
			Expect(repo.Delete(ctx, filesubscription.FileSubscriptionKey{FileType: "missing", OwnerAppName: "consumer-app", OwnerTeamName: "platform--narvi"})).To(Succeed())
		})
	})
})
