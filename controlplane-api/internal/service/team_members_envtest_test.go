// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
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

	Describe("UpdateTeam membership replacement", func() {
		var team *organizationv1.Team
		var svc service.TeamService
		var ref service.ResourceRef

		BeforeEach(func() {
			team = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: "group-a--replacement", Namespace: "default", Labels: map[string]string{config.EnvironmentLabelKey: "poc"}},
				Spec: organizationv1.TeamSpec{
					Name: "replacement", Group: "group-a", Email: "contact@example.com", Category: organizationv1.TeamCategoryCustomer,
					Members: []organizationv1.Member{{Name: "Alice", Email: "alice@example.com"}},
				},
			}
			Expect(k8sClient.Create(context.Background(), team)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(context.Background(), team)).To(Succeed()) })
			svc = service.NewTeamK8sService(cc.NewScopedClient(k8sClient, "poc"), service.NewNoopResourceChecker())
			ref = service.ResourceRef{Namespace: team.Namespace, Name: team.Name, Group: "group-a", TeamName: team.Name}
		})

		DescribeTable("persists the desired membership and supports retries", func(members []model.MemberInput, expected []organizationv1.Member) {
			for range 2 {
				result, err := svc.UpdateTeam(adminCtx(), ref, model.UpdateTeamInput{Members: members})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Errors).To(BeEmpty())
				Expect(result.Accepted).To(BeTrue())
				Expect(result.Team).To(BeNil(), "acceptance does not imply the projection is updated")
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(team), team)).To(Succeed())
				Expect(team.Spec.Members).To(ConsistOf(expected))
				Expect(team.Spec.Email).To(Equal("contact@example.com"))
			}
		},
			Entry("add and remove", []model.MemberInput{{Name: "Bob", Email: "bob@example.com"}}, []organizationv1.Member{{Name: "Bob", Email: "bob@example.com"}}),
			Entry("add while retaining", []model.MemberInput{{Name: "Alice", Email: "alice@example.com"}, {Name: "Bob", Email: "bob@example.com"}}, []organizationv1.Member{{Name: "Alice", Email: "alice@example.com"}, {Name: "Bob", Email: "bob@example.com"}}),
			Entry("nil preserves", nil, []organizationv1.Member{{Name: "Alice", Email: "alice@example.com"}}),
			Entry("unchanged", []model.MemberInput{{Name: "Alice", Email: "alice@example.com"}}, []organizationv1.Member{{Name: "Alice", Email: "alice@example.com"}}),
			Entry("name change", []model.MemberInput{{Name: "Alice Updated", Email: "alice@example.com"}}, []organizationv1.Member{{Name: "Alice Updated", Email: "alice@example.com"}}),
		)

		DescribeTable("rejects an invalid replacement without partially changing email or members", func(members []model.MemberInput) {
			original := team.Spec.DeepCopy()
			result, err := svc.UpdateTeam(adminCtx(), ref, model.UpdateTeamInput{
				Email: strPtr("new@example.com"), Members: members,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Accepted).To(BeFalse())
			Expect(result.Errors).To(ContainElement(HaveField("Code", model.ErrorCodeValidationFailed)))
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(team), team)).To(Succeed())
			Expect(team.Spec).To(Equal(*original))
		},
			Entry("invalid email", []model.MemberInput{{Name: "Invalid", Email: ""}}),
			Entry("empty list", []model.MemberInput{}),
		)

		It("preserves members when authorization fails", func() {
			result, err := svc.UpdateTeam(noViewerCtx(), ref, model.UpdateTeamInput{Members: []model.MemberInput{}})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Accepted).To(BeFalse())
			Expect(result.Errors).To(ContainElement(HaveField("Code", model.ErrorCodeForbidden)))
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(team), team)).To(Succeed())
			Expect(team.Spec.Members).To(HaveLen(1))
		})
	})

	Describe("Team metadata", func() {
		var team *organizationv1.Team
		var svc service.TeamService
		var ref service.ResourceRef

		BeforeEach(func() {
			team = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: "group-a--metadata", Namespace: "default", Labels: map[string]string{config.EnvironmentLabelKey: "poc"}},
				Spec: organizationv1.TeamSpec{
					Name: "metadata", Group: "group-a", Email: "contact@example.com", Category: organizationv1.TeamCategoryInfrastructure,
					DisplayName: "Old Name", Description: "Old description", Secret: "secret-ref", TeamToken: "token-ref",
					Members: []organizationv1.Member{{Name: "Alice", Email: "alice@example.com"}},
				},
			}
			Expect(k8sClient.Create(context.Background(), team)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(context.Background(), team)).To(Succeed()) })
			svc = service.NewTeamK8sService(cc.NewScopedClient(k8sClient, "poc"), service.NewNoopResourceChecker())
			ref = service.ResourceRef{Namespace: team.Namespace, Name: team.Name, Group: "group-a", TeamName: team.Name}
		})

		DescribeTable("UpdateTeam applies only provided metadata", func(input model.UpdateTeamInput, displayName, description string) {
			expected := team.Spec.DeepCopy()
			expected.DisplayName = displayName
			expected.Description = description
			result, err := svc.UpdateTeam(adminCtx(), ref, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Errors).To(BeEmpty())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(team), team)).To(Succeed())
			Expect(team.Spec).To(Equal(*expected))
		},
			Entry("displayName only", model.UpdateTeamInput{DisplayName: strPtr("New Name")}, "New Name", "Old description"),
			Entry("description only", model.UpdateTeamInput{Description: strPtr("New description")}, "Old Name", "New description"),
			Entry("both", model.UpdateTeamInput{DisplayName: strPtr("New Name"), Description: strPtr("New description")}, "New Name", "New description"),
			Entry("empty description clears", model.UpdateTeamInput{Description: strPtr("")}, "Old Name", ""),
			Entry("omitted preserves", model.UpdateTeamInput{}, "Old Name", "Old description"),
		)

		DescribeTable("UpdateTeam rejects invalid input without partial writes", func(input model.UpdateTeamInput) {
			original := team.Spec.DeepCopy()
			input.Email = strPtr("new@example.com")
			input.Members = []model.MemberInput{{Name: "Bob", Email: "bob@example.com"}}
			result, err := svc.UpdateTeam(adminCtx(), ref, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Accepted).To(BeFalse())
			Expect(result.Errors).To(ContainElement(HaveField("Code", model.ErrorCodeValidationFailed)))
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(team), team)).To(Succeed())
			Expect(team.Spec).To(Equal(*original))
		},
			Entry("empty displayName", model.UpdateTeamInput{DisplayName: strPtr("")}),
			Entry("too long displayName", model.UpdateTeamInput{DisplayName: strPtr(strings.Repeat("a", 201))}),
			Entry("too long description", model.UpdateTeamInput{Description: strPtr(strings.Repeat("a", 257))}),
		)

		DescribeTable("CreateTeam on an existing team preserves omitted metadata and server-managed fields",
			func(displayName, description *string, expectedDisplayName, expectedDescription string) {
				expected := team.Spec.DeepCopy()
				expected.Email = "new@example.com"
				expected.Members = []organizationv1.Member{{Name: "Bob", Email: "bob@example.com"}}
				expected.DisplayName = expectedDisplayName
				expected.Description = expectedDescription
				result, err := svc.CreateTeam(adminCtx(), model.CreateTeamInput{
					Environment: "default", Group: "group-a", Name: "metadata", Email: "new@example.com",
					Members:     []model.MemberInput{{Name: "Bob", Email: "bob@example.com"}},
					DisplayName: displayName, Description: description,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Errors).To(BeEmpty())
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(team), team)).To(Succeed())
				Expect(team.Spec).To(Equal(*expected))
			},
			Entry("metadata omitted", nil, nil, "Old Name", "Old description"),
			Entry("metadata provided", strPtr("New Name"), strPtr(""), "New Name", ""),
			Entry("empty description only clears", nil, strPtr(""), "Old Name", ""),
		)
	})

	Describe("CreateTeam for a new team", func() {
		var svc service.TeamService

		BeforeEach(func() {
			svc = service.NewTeamK8sService(cc.NewScopedClient(k8sClient, "poc"), service.NewNoopResourceChecker())
		})

		It("persists metadata and leaves omitted metadata unset", func() {
			for name, displayName := range map[string]*string{"with-metadata": strPtr("With Metadata"), "without-metadata": nil} {
				result, err := svc.CreateTeam(adminCtx(), model.CreateTeamInput{
					Environment: "default", Group: "group-a", Name: name, Email: "team@example.com",
					Members:     []model.MemberInput{{Name: "Alice", Email: "alice@example.com"}},
					DisplayName: displayName, Description: displayName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Errors).To(BeEmpty())
				created := &organizationv1.Team{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "group-a--" + name}, created)).To(Succeed())
				DeferCleanup(func() { Expect(k8sClient.Delete(context.Background(), created)).To(Succeed()) })
				Expect(created.Spec.Category).To(Equal(organizationv1.TeamCategoryCustomer))
				if displayName == nil {
					Expect(created.Spec.DisplayName).To(BeEmpty())
					Expect(created.Spec.Description).To(BeEmpty())
				} else {
					Expect(created.Spec.DisplayName).To(Equal(*displayName))
					Expect(created.Spec.Description).To(Equal(*displayName))
				}
			}
		})

		DescribeTable("rejects missing required data without creating the team", func(input model.CreateTeamInput, message string) {
			input.Environment, input.Group, input.Name = "default", "group-a", "invalid"
			result, err := svc.CreateTeam(adminCtx(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Accepted).To(BeFalse())
			Expect(result.Errors).To(ConsistOf(SatisfyAll(
				HaveField("Code", model.ErrorCodeValidationFailed), HaveField("Message", ContainSubstring(message)))))
			err = k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "group-a--invalid"}, &organizationv1.Team{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		},
			Entry("missing email", model.CreateTeamInput{Members: []model.MemberInput{{Name: "Alice", Email: "alice@example.com"}}}, "email is required"),
			Entry("missing members", model.CreateTeamInput{Email: "team@example.com"}, "at least one member"),
			Entry("empty displayName", model.CreateTeamInput{
				Email: "team@example.com", Members: []model.MemberInput{{Name: "Alice", Email: "alice@example.com"}}, DisplayName: strPtr(""),
			}, "displayName must not be empty"),
		)
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

	Describe("Group description", func() {
		var group *organizationv1.Group
		var svc service.GroupService
		var ref service.ResourceRef

		BeforeEach(func() {
			group = &organizationv1.Group{
				ObjectMeta: metav1.ObjectMeta{Name: "described", Namespace: "default", Labels: map[string]string{config.EnvironmentLabelKey: "poc"}},
				Spec:       organizationv1.GroupSpec{DisplayName: "Old Name", Description: "Old description"},
			}
			Expect(k8sClient.Create(context.Background(), group)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(context.Background(), group)).To(Succeed()) })
			svc = service.NewGroupK8sService(cc.NewScopedClient(k8sClient, "poc"), service.NewNoopTeamChecker())
			ref = service.ResourceRef{Namespace: group.Namespace, Name: group.Name}
		})

		DescribeTable("UpdateGroup applies only a provided description", func(description *string, expected string) {
			result, err := svc.UpdateGroup(adminCtx(), ref, model.UpdateGroupInput{DisplayName: strPtr("New Name"), Description: description})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Errors).To(BeEmpty())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(group), group)).To(Succeed())
			Expect(group.Spec).To(Equal(organizationv1.GroupSpec{DisplayName: "New Name", Description: expected}))
		},
			Entry("omitted or null preserves", nil, "Old description"),
			Entry("empty clears", strPtr(""), ""),
			Entry("value changes", strPtr("New description"), "New description"),
			Entry("maximum length is accepted", strPtr(strings.Repeat("a", 256)), strings.Repeat("a", 256)),
		)

		It("UpdateGroup rejects a too long description without partial writes", func() {
			result, err := svc.UpdateGroup(adminCtx(), ref, model.UpdateGroupInput{
				DisplayName: strPtr("New Name"), Description: strPtr(strings.Repeat("a", 257)),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Accepted).To(BeFalse())
			Expect(result.Errors).NotTo(BeEmpty())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(group), group)).To(Succeed())
			Expect(group.Spec).To(Equal(organizationv1.GroupSpec{DisplayName: "Old Name", Description: "Old description"}))
		})

		DescribeTable("CreateGroup on an existing group preserves an omitted description", func(description *string, expected string) {
			result, err := svc.CreateGroup(adminCtx(), model.CreateGroupInput{
				Environment: "default", Name: "described", DisplayName: "New Name", Description: description,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Errors).To(BeEmpty())
			Expect(result.Accepted).To(BeTrue())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(group), group)).To(Succeed())
			Expect(group.Spec).To(Equal(organizationv1.GroupSpec{DisplayName: "New Name", Description: expected}))
		},
			Entry("omitted", nil, "Old description"),
			Entry("empty clears", strPtr(""), ""),
			Entry("provided", strPtr("New description"), "New description"),
		)

		DescribeTable("CreateGroup creates a new group with an optional description", func(name string, description *string, expected string) {
			result, err := svc.CreateGroup(adminCtx(), model.CreateGroupInput{
				Environment: "default", Name: name, DisplayName: "Fresh", Description: description,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Errors).To(BeEmpty())
			created := &organizationv1.Group{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: name}, created)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(context.Background(), created)).To(Succeed()) })
			Expect(created.Spec).To(Equal(organizationv1.GroupSpec{DisplayName: "Fresh", Description: expected}))
		},
			Entry("omitted", "fresh-omitted", nil, ""),
			Entry("explicit empty", "fresh-empty", strPtr(""), ""),
			Entry("provided", "fresh-provided", strPtr("Desc"), "Desc"),
		)

		It("CreateGroup rejects a too long description without creating the group", func() {
			result, err := svc.CreateGroup(adminCtx(), model.CreateGroupInput{
				Environment: "default", Name: "too-long", DisplayName: "Fresh", Description: strPtr(strings.Repeat("a", 257)),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Accepted).To(BeFalse())
			Expect(result.Errors).NotTo(BeEmpty())
			err = k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "too-long"}, &organizationv1.Group{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})
})
