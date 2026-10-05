// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fileexposure_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	entfileexposure "github.com/telekom/controlplane/controlplane-api/ent/fileexposure"
	entfilesubscription "github.com/telekom/controlplane/controlplane-api/ent/filesubscription"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/fileexposure"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

type mockFileExposureDeps struct {
	appIDs            map[string]int
	zoneIDs           map[string]int
	activeFileTypeIDs map[string]int
	appErr            error
	zoneErr           error
	activeFileTypeErr error
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

func (m *mockFileExposureDeps) FindActiveFileTypeID(_ context.Context, fileType string) (int, error) {
	if m.activeFileTypeErr != nil {
		return 0, m.activeFileTypeErr
	}
	if id, ok := m.activeFileTypeIDs[fileType]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("active file_type %q: %w", fileType, infrastructure.ErrEntityNotFound)
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
			appIDs:            map[string]int{"provider-app:platform--narvi": appID},
			zoneIDs:           map[string]int{"caas": zoneID},
			activeFileTypeIDs: map[string]int{"invoice": fileTypeID},
		}
		repo = fileexposure.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create file exposure and set optional file type edge", func() {
			data := &fileexposure.FileExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-a", nil),
				StatusPhase:    "READY",
				StatusMessage:  "ok",
				Visibility:     "ENTERPRISE",
				Active:         true,
				Zone:           "caas",
				FileSFTP:       &model.FileSFTP{PublicKeys: []model.SSHPublicKeySpec{{Key: "ssh-rsa AAA", Label: "label value"}}},
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
			Expect(exp.Sftp).To(Equal(&model.FileSFTP{PublicKeys: []model.SSHPublicKeySpec{{Key: "ssh-rsa AAA", Label: "label value"}}}))
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
				Zone:           "caas",
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

		It("should not link an inactive exposure to the active file type", func() {
			data := &fileexposure.FileExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "inactive-exp", nil),
				StatusPhase:    "PENDING",
				Visibility:     "WORLD",
				Active:         false,
				Zone:           "caas",
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "provider-app",
				TeamName:       "platform--narvi",
				TargetFileType: "invoice",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exposure, err := client.FileExposure.Query().Where(entfileexposure.FileTypeEQ("invoice")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = exposure.QueryFileTypeDef().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})

		It("should clear a stale file_type_def edge when the active file type is no longer resolvable", func() {
			data := &fileexposure.FileExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "stale-exp", nil),
				StatusPhase:    "READY",
				Visibility:     "WORLD",
				Active:         true,
				Zone:           "caas",
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "provider-app",
				TeamName:       "platform--narvi",
				TargetFileType: "invoice",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exposure, err := client.FileExposure.Query().Where(entfileexposure.FileTypeEQ("invoice")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			linked, err := exposure.QueryFileTypeDef().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(linked.ID).To(Equal(fileTypeID))

			deps.activeFileTypeIDs = map[string]int{}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exposure, err = client.FileExposure.Query().Where(entfileexposure.FileTypeEQ("invoice")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = exposure.QueryFileTypeDef().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})

		It("should return dependency missing when application is missing", func() {
			data := &fileexposure.FileExposureData{Meta: shared.NewMetadata("prod--platform--narvi", "exp-c", nil), Zone: "caas", AppName: "missing", TeamName: "platform--narvi", TargetFileType: "invoice"}
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
		})

		It("should propagate non-not-found application errors", func() {
			dbErr := errors.New("db down")
			failRepo := fileexposure.NewRepository(client, cache, &mockFileExposureDeps{appErr: dbErr})
			err := failRepo.Upsert(ctx, &fileexposure.FileExposureData{Meta: shared.NewMetadata("prod--platform--narvi", "exp-d", nil), Zone: "caas", AppName: "provider-app", TeamName: "platform--narvi", TargetFileType: "invoice"})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should populate cache entries", func() {
			data := &fileexposure.FileExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-e", nil),
				StatusPhase:    "READY",
				Visibility:     "ZONE",
				Active:         true,
				Zone:           "caas",
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

			// The active lookup is populated lazily by the resolver, never by Upsert.
			_, activeFound := cache.Get(cachekeys.ActiveFileExposure("invoice"))
			Expect(activeFound).To(BeFalse())
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
				Zone:           "caas",
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
				Zone:           "caas",
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

	Describe("active file-type lookup cache invalidation", func() {
		const fileType = "invoice"
		var resolver *infrastructure.IDResolver

		exposure := func(appName string, active bool) *fileexposure.FileExposureData {
			return &fileexposure.FileExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-"+appName, nil),
				StatusPhase:    "READY",
				Visibility:     "ZONE",
				Active:         active,
				Zone:           "caas",
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        appName,
				TeamName:       "platform--narvi",
				TargetFileType: fileType,
			}
		}
		key := fileexposure.FileExposureKey{FileType: fileType, AppName: "provider-app", TeamName: "platform--narvi"}

		// primeActive resolves the active exposure through the real resolver and
		// proves the resolved ID is now served from the edge cache.
		primeActive := func() int {
			id, err := resolver.FindActiveFileExposureByFileType(ctx, fileType)
			Expect(err).NotTo(HaveOccurred())
			cache.Wait()
			cached, found := cache.Get(cachekeys.ActiveFileExposure(fileType))
			Expect(found).To(BeTrue())
			Expect(cached).To(Equal(id))
			return id
		}

		// failOn makes every ENT mutation of the given operation fail
		// deterministically, simulating a DB error at that write.
		failOn := func(op ent.Op) error {
			injected := errors.New("injected db failure")
			client.FileExposure.Use(func(next ent.Mutator) ent.Mutator {
				return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
					if m.Op().Is(op) {
						return nil, injected
					}
					return next.Mutate(ctx, m)
				})
			})
			return injected
		}

		BeforeEach(func() {
			resolver = infrastructure.NewIDResolver(client, cache)
			Expect(repo.Upsert(ctx, exposure("provider-app", true))).To(Succeed())
		})

		It("stops resolving an exposure once it is deactivated", func() {
			primeActive()
			Expect(repo.Upsert(ctx, exposure("provider-app", false))).To(Succeed())
			cache.Wait()

			_, err := resolver.FindActiveFileExposureByFileType(ctx, fileType)
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})

		It("stops resolving an exposure once it is deleted, also on idempotent re-delete", func() {
			primeActive()
			Expect(repo.Delete(ctx, key)).To(Succeed())
			cache.Wait()
			_, err := resolver.FindActiveFileExposureByFileType(ctx, fileType)
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			// A stale active entry left behind must be evicted by a count=0 delete.
			aet, alk := cachekeys.ActiveFileExposure(fileType)
			cache.Set(aet, alk, 4242)
			cache.Wait()
			Expect(repo.Delete(ctx, key)).To(Succeed())
			cache.Wait()
			_, found := cache.Get(aet, alk)
			Expect(found).To(BeFalse())
		})

		It("resolves the newly active owner instead of the stale inactive one", func() {
			other, err := client.Application.Create().
				SetName("other-app").
				SetNamespace("prod--platform--narvi").
				SetOwnerTeamID(client.Team.Query().OnlyIDX(ctx)).
				SetZoneID(zoneID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			deps.appIDs["other-app:platform--narvi"] = other.ID

			oldID := primeActive()
			Expect(repo.Upsert(ctx, exposure("provider-app", false))).To(Succeed())
			Expect(repo.Upsert(ctx, exposure("other-app", true))).To(Succeed())
			cache.Wait()

			id, err := resolver.FindActiveFileExposureByFileType(ctx, fileType)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(Equal(oldID))
			Expect(id).To(Equal(client.FileExposure.Query().
				Where(entfileexposure.HasOwnerWith(application.IDEQ(other.ID))).OnlyIDX(ctx)))
		})

		It("keeps the cached active lookup when the primary upsert fails", func() {
			id := primeActive()
			injected := failOn(ent.OpCreate)

			Expect(errors.Is(repo.Upsert(ctx, exposure("provider-app", false)), injected)).To(BeTrue())
			cache.Wait()
			cached, found := cache.Get(cachekeys.ActiveFileExposure(fileType))
			Expect(found).To(BeTrue())
			Expect(cached).To(Equal(id))
		})

		It("evicts the active lookup when a write after the primary upsert fails", func() {
			primeActive()
			// The edge FK update (UpdateOne) runs after the primary upsert.
			injected := failOn(ent.OpUpdateOne)

			Expect(errors.Is(repo.Upsert(ctx, exposure("provider-app", false)), injected)).To(BeTrue())
			cache.Wait()
			Expect(client.FileExposure.Query().Where(entfileexposure.ActiveEQ(false)).CountX(ctx)).To(Equal(1))
			_, err := resolver.FindActiveFileExposureByFileType(ctx, fileType)
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})

		It("keeps the cached active lookup when the delete fails", func() {
			id := primeActive()
			injected := failOn(ent.OpDelete)

			Expect(errors.Is(repo.Delete(ctx, key), injected)).To(BeTrue())
			cache.Wait()
			cached, found := cache.Get(cachekeys.ActiveFileExposure(fileType))
			Expect(found).To(BeTrue())
			Expect(cached).To(Equal(id))
		})

		It("resolves a reactivated exposure once no negative entry applies", func() {
			id := primeActive()
			Expect(repo.Upsert(ctx, exposure("provider-app", false))).To(Succeed())
			cache.Wait()
			_, err := resolver.FindActiveFileExposureByFileType(ctx, fileType)
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			Expect(repo.Upsert(ctx, exposure("provider-app", true))).To(Succeed())
			cache.Wait()
			// The original resolver keeps its bounded negative entry; a fresh
			// resolver sharing the edge cache reads the reactivated row.
			fresh := infrastructure.NewIDResolver(client, cache)
			Expect(fresh.FindActiveFileExposureByFileType(ctx, fileType)).To(Equal(id))
		})

		It("resolves a reactivated exposure on the same resolver after the negative TTL expires", func() {
			now := time.Now()
			clocked := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(5*time.Second),
				infrastructure.WithNowFunc(func() time.Time { return now }))
			id, err := clocked.FindActiveFileExposureByFileType(ctx, fileType)
			Expect(err).NotTo(HaveOccurred())
			cache.Wait()

			Expect(repo.Upsert(ctx, exposure("provider-app", false))).To(Succeed())
			cache.Wait()
			_, err = clocked.FindActiveFileExposureByFileType(ctx, fileType)
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			Expect(repo.Upsert(ctx, exposure("provider-app", true))).To(Succeed())
			cache.Wait()
			// Within the negative TTL the miss is still served.
			_, err = clocked.FindActiveFileExposureByFileType(ctx, fileType)
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			now = now.Add(6 * time.Second)
			Expect(clocked.FindActiveFileExposureByFileType(ctx, fileType)).To(Equal(id))
		})
	})

	Describe("Delete", func() {
		It("should delete existing exposure and evict cache", func() {
			data := &fileexposure.FileExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-del", nil),
				StatusPhase:    "READY",
				Visibility:     "ZONE",
				Active:         true,
				Zone:           "caas",
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
