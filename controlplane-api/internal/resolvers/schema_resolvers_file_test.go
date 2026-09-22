// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
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
