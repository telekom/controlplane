// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Team member removal with Kubernetes", Ordered, func() {
	var k8sClient client.Client

	BeforeAll(func() {
		assets, err := envtest.SetupEnvtestDefaultBinaryAssetsDirectory()
		Expect(err).NotTo(HaveOccurred())
		testEnv := &envtest.Environment{
			CRDDirectoryPaths:           []string{filepath.Join("..", "..", "..", "organization", "config", "crd", "bases")},
			ErrorIfCRDPathMissing:       true,
			DownloadBinaryAssets:        os.Getenv("KUBEBUILDER_ASSETS") == "",
			DownloadBinaryAssetsVersion: "1.32.0",
			BinaryAssetsDirectory:       assets,
		}
		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(testEnv.Stop()).To(Succeed()) })
		k8sClient, err = client.New(cfg, client.Options{Scheme: newTestScheme()})
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("removes the normalized identity without changing other members or contact email",
		func(storedEmail, requestEmail string) {
			ctx := context.Background()
			team := &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name: "group-a--members", Namespace: "default",
					Labels: map[string]string{config.EnvironmentLabelKey: "poc"},
				},
				Spec: organizationv1.TeamSpec{
					Name: "members", Group: "group-a", Email: "Contact@Example.com",
					Category: organizationv1.TeamCategoryCustomer,
					Members: []organizationv1.Member{
						{Name: "Member", Email: storedEmail},
						{Name: "Other Alice", Email: "alice.other@example.com"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, team)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, team)).To(Succeed()) })
			svc := service.NewTeamK8sService(cc.NewScopedClient(k8sClient, "poc"), service.NewNoopResourceChecker())
			ref := service.ResourceRef{Namespace: team.Namespace, Name: team.Name, Group: "group-a", TeamName: team.Name}
			for range 2 {
				result, err := svc.RemoveTeamMember(adminCtx(), ref, requestEmail)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Errors).To(BeEmpty())
				stored := &organizationv1.Team{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(team), stored)).To(Succeed())
				Expect(stored.Spec.Members).To(Equal([]organizationv1.Member{{Name: "Other Alice", Email: "alice.other@example.com"}}))
				Expect(stored.Spec.Email).To(Equal("Contact@Example.com"))
			}
		},
		Entry("lowercase projected email", "alice@example.com", "alice@example.com"),
		Entry("mixed-case request email", "alice@example.com", "ALICE@example.com"),
		Entry("Unicode request", "üser@bücher.example", "ÜSER@BÜCHER.EXAMPLE"),
		Entry("legacy Unicode source", "Üser@BÜCHER.example", "üser@bücher.example"),
	)

	It("removes all legacy lowercase-equivalent members while preserving distinct identities", func() {
		ctx := context.Background()
		team := &organizationv1.Team{
			ObjectMeta: metav1.ObjectMeta{Name: "group-a--unicode", Namespace: "default", Labels: map[string]string{config.EnvironmentLabelKey: "poc"}},
			Spec: organizationv1.TeamSpec{
				Name: "unicode", Group: "group-a", Email: "Contact@Example.com", Category: organizationv1.TeamCategoryCustomer,
				Members: []organizationv1.Member{{Name: "Upper", Email: "Üser@example.com"}, {Name: "Lower", Email: "üser@example.com"}, {Name: "Decomposed", Email: "u\u0308ser@example.com"}},
			},
		}
		// This envtest has no webhooks, allowing legacy keys that admission now rejects.
		Expect(k8sClient.Create(ctx, team)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, team)).To(Succeed()) })
		svc := service.NewTeamK8sService(cc.NewScopedClient(k8sClient, "poc"), service.NewNoopResourceChecker())
		ref := service.ResourceRef{Namespace: team.Namespace, Name: team.Name, Group: "group-a", TeamName: team.Name}
		result, err := svc.RemoveTeamMember(adminCtx(), ref, "ÜSER@EXAMPLE.COM")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Errors).To(BeEmpty())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)).To(Succeed())
		Expect(team.Spec.Members).To(Equal([]organizationv1.Member{{Name: "Decomposed", Email: "u\u0308ser@example.com"}}))
	})
})
