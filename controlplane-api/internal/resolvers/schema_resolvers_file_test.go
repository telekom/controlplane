// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"context"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/fileexposure"
	"github.com/telekom/controlplane/controlplane-api/ent/filesubscription"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	gqlmodel "github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FileExposure/FileSubscription.Sftp", func() {
	var client *ent.Client
	var s *testutil.SeedData

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should store and return sftp public keys on FileExposure", func() {
		ctx := testutil.AllowContext()

		exposure, err := client.FileExposure.Create().
			SetNamespace("default").
			SetFileType("invoice").
			SetZoneName(s.ZoneEU.Name).
			SetOwner(s.AppAlpha).
			SetZone(s.ZoneEU).
			SetSftp(&model.FileSFTP{
				PublicKeys: []model.SSHPublicKeySpec{{Key: "ssh-ed25519 AAA"}, {Key: "ssh-ed25519 BBB"}},
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.FileExposure.Get(ctx, exposure.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.Sftp).NotTo(BeNil())
		Expect(fetched.Sftp.PublicKeys).To(Equal([]model.SSHPublicKeySpec{{Key: "ssh-ed25519 AAA"}, {Key: "ssh-ed25519 BBB"}}))
	})

	It("should store and return sftp public keys on FileSubscription", func() {
		ctx := testutil.AllowContext()

		subscription, err := client.FileSubscription.Create().
			SetNamespace("default").
			SetName("filesub-invoice").
			SetFileType("invoice").
			SetZoneName(s.ZoneEU.Name).
			SetOwner(s.AppBeta).
			SetZone(s.ZoneEU).
			SetSftp(&model.FileSFTP{
				PublicKeys: []model.SSHPublicKeySpec{{Key: "ssh-ed25519 CCC"}},
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.FileSubscription.Get(ctx, subscription.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.Sftp).NotTo(BeNil())
		Expect(fetched.Sftp.PublicKeys).To(Equal([]model.SSHPublicKeySpec{{Key: "ssh-ed25519 CCC"}}))
	})
})

var _ = Describe("FileSubscription approval resolvers", func() {
	var (
		client           *ent.Client
		resolver         *resolvers.Resolver
		seed             *testutil.SeedData
		fileExposure     *ent.FileExposure
		fileSubscription *ent.FileSubscription
		fileApproval     *ent.Approval
		fileRequest      *ent.ApprovalRequest
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		resolver = resolvers.NewResolver(client, service.Services{}, nil, "")
		seed = testutil.SeedStandard(client)
		ctx := testutil.AllowContext()

		var err error
		fileExposure, err = client.FileExposure.Create().
			SetNamespace("default").
			SetFileType("invoice").
			SetZoneName(seed.ZoneEU.Name).
			SetVisibility(fileexposure.VisibilityEnterprise).
			SetActive(true).
			SetOwner(seed.AppAlpha).
			SetZone(seed.ZoneEU).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fileSubscription, err = client.FileSubscription.Create().
			SetNamespace("default").
			SetName("filesub-invoice").
			SetFileType("invoice").
			SetZoneName(seed.ZoneEU.Name).
			SetStatusPhase(filesubscription.StatusPhaseReady).
			SetOwner(seed.AppBeta).
			SetTarget(fileExposure).
			SetZone(seed.ZoneEU).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fileApproval, err = client.Approval.Create().
			SetNamespace("prod").
			SetName("filesubscription--filesub-invoice").
			SetAction("ALLOW").
			SetRequester(model.RequesterInfo{TeamName: seed.TeamBeta.Name}).
			SetDecider(model.DeciderInfo{TeamName: seed.TeamAlpha.Name}).
			SetDeciderTeamName(seed.TeamAlpha.Name).
			SetFileSubscription(fileSubscription).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fileRequest, err = client.ApprovalRequest.Create().
			SetNamespace("prod").
			SetName("filesubscription--filesub-invoice--req-1").
			SetAction("ALLOW").
			SetRequester(model.RequesterInfo{TeamName: seed.TeamBeta.Name}).
			SetDecider(model.DeciderInfo{TeamName: seed.TeamAlpha.Name}).
			SetDeciderTeamName(seed.TeamAlpha.Name).
			SetFileSubscription(fileSubscription).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		client.Close()
	})

	It("returns FileSubscriptionInfo from an approval", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Admin: true})
		info, err := resolver.Approval().Subscription(ctx, fileApproval)
		Expect(err).NotTo(HaveOccurred())

		fileInfo, ok := info.(*gqlmodel.FileSubscriptionInfo)
		Expect(ok).To(BeTrue(), "expected FileSubscriptionInfo implementation")
		Expect(fileInfo.FileType).To(Equal("invoice"))
		Expect(fileInfo.OwnerApplicationName).To(Equal(seed.AppBeta.Name))
		Expect(fileInfo.OwnerTeam.Name).To(Equal(seed.TeamBeta.Name))
		Expect(fileInfo.OwnerApplication.ID).To(Equal(seed.AppBeta.ID))
	})

	It("returns FileSubscriptionInfo from an approval request", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Admin: true})
		info, err := resolver.ApprovalRequest().Subscription(ctx, fileRequest)
		Expect(err).NotTo(HaveOccurred())

		fileInfo, ok := info.(*gqlmodel.FileSubscriptionInfo)
		Expect(ok).To(BeTrue(), "expected FileSubscriptionInfo implementation")
		Expect(fileInfo.ID).To(Equal(fileSubscription.ID))
		Expect(fileInfo.FileType).To(Equal("invoice"))
	})

	It("returns the approval related through a file subscription", func() {
		ctx := viewer.NewContext(context.Background(), &viewer.Viewer{Teams: []string{seed.TeamAlpha.Name}})
		approval, err := resolver.ApprovalRequest().Approval(ctx, fileRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(approval.ID).To(Equal(fileApproval.ID))
	})

	It("converts the reduced status phase", func() {
		phase := string(filesubscription.StatusPhaseReady)
		status, err := resolver.FileSubscriptionInfo().StatusPhase(context.Background(), &gqlmodel.FileSubscriptionInfo{StatusPhase: &phase})
		Expect(err).NotTo(HaveOccurred())
		Expect(status).NotTo(BeNil())
		Expect(*status).To(Equal(filesubscription.StatusPhaseReady))
	})

	It("returns reduced cross-tenant subscriptions from a file exposure", func() {
		infos, err := resolver.FileExposure().Subscriptions(context.Background(), fileExposure)
		Expect(err).NotTo(HaveOccurred())
		Expect(infos).To(HaveLen(1))
		Expect(infos[0].ID).To(Equal(fileSubscription.ID))
		Expect(infos[0].FileType).To(Equal("invoice"))
		Expect(infos[0].OwnerApplicationName).To(Equal(seed.AppBeta.Name))
		Expect(infos[0].OwnerTeam.Name).To(Equal(seed.TeamBeta.Name))
	})

	It("returns a reduced cross-tenant target from a file subscription", func() {
		info, err := resolver.FileSubscription().Target(context.Background(), fileSubscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.ID).To(Equal(fileExposure.ID))
		Expect(info.FileType).To(Equal("invoice"))
		Expect(info.OwnerApplicationName).To(Equal(seed.AppAlpha.Name))
		Expect(info.OwnerTeam.Name).To(Equal(seed.TeamAlpha.Name))

		visibility, err := resolver.FileExposureInfo().Visibility(context.Background(), info)
		Expect(err).NotTo(HaveOccurred())
		Expect(visibility).To(Equal(fileexposure.VisibilityEnterprise))
	})
})
