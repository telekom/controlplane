// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(applicationv1.AddToScheme(s))
	utilruntime.Must(approvalv1.AddToScheme(s))
	utilruntime.Must(organizationv1.AddToScheme(s))
	return s
}

func newFakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(objs...).Build()
}

func adminCtx() context.Context {
	return viewer.NewContext(context.Background(), &viewer.Viewer{Admin: true})
}

func groupCtx(group string, teams ...string) context.Context {
	return viewer.NewContext(context.Background(), &viewer.Viewer{Group: group, Teams: teams})
}

func teamCtx(teams ...string) context.Context {
	return viewer.NewContext(context.Background(), &viewer.Viewer{Teams: teams})
}

func noViewerCtx() context.Context {
	return context.Background()
}

var _ = Describe("TeamK8sService", func() {
	var (
		svc       service.TeamService
		k8sClient client.Client
	)

	createInput := model.CreateTeamInput{
		Environment: "dev",
		Group:       "group-a",
		Name:        "team-alpha",
		Email:       "alpha@example.com",
		Members: []model.MemberInput{
			{Name: "Alice", Email: "alice@example.com"},
		},
	}

	updateInput := model.UpdateTeamInput{
		Environment: "dev",
		Group:       "group-a",
		Name:        "team-alpha",
		Email:       strPtr("newemail@example.com"),
	}

	BeforeEach(func() {
		k8sClient = newFakeClient()
		svc = service.NewTeamK8sService(k8sClient)
	})

	Describe("CreateTeam", func() {
		Describe("Authorization", func() {
			It("should allow admin to create any team", func() {
				result, err := svc.CreateTeam(adminCtx(), createInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should allow group viewer to create team in their group", func() {
				result, err := svc.CreateTeam(groupCtx("group-a"), createInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should deny group viewer creating team in a different group", func() {
				_, err := svc.CreateTeam(groupCtx("group-b"), createInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("forbidden"))
			})

			It("should deny team viewer from creating a team", func() {
				_, err := svc.CreateTeam(teamCtx("team-alpha"), createInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("forbidden"))
			})

			It("should deny when no viewer is present", func() {
				_, err := svc.CreateTeam(noViewerCtx(), createInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unauthorized"))
			})
		})

		Describe("Success", func() {
			It("should create a team and return correct result", func() {
				result, err := svc.CreateTeam(adminCtx(), createInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
				Expect(result.Message).To(Equal("team created successfully"))
				Expect(*result.Namespace).To(Equal("dev"))
				Expect(*result.ResourceName).To(Equal("group-a--team-alpha"))

				// Verify the CRD was created in K8s
				team := &organizationv1.Team{}
				err = k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: "dev",
					Name:      "group-a--team-alpha",
				}, team)
				Expect(err).NotTo(HaveOccurred())
				Expect(team.Spec.Name).To(Equal("team-alpha"))
				Expect(team.Spec.Group).To(Equal("group-a"))
				Expect(team.Spec.Email).To(Equal("alpha@example.com"))
				Expect(team.Spec.Members).To(HaveLen(1))
				Expect(team.Spec.Members[0].Name).To(Equal("Alice"))
				Expect(team.Spec.Category).To(Equal(organizationv1.TeamCategoryCustomer))
			})
		})
	})

	Describe("UpdateTeam", func() {
		BeforeEach(func() {
			// Seed a team first
			_, err := svc.CreateTeam(adminCtx(), createInput)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("Authorization", func() {
			It("should allow admin to update any team", func() {
				result, err := svc.UpdateTeam(adminCtx(), updateInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should allow group viewer to update team in their group", func() {
				result, err := svc.UpdateTeam(groupCtx("group-a"), updateInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should deny group viewer updating team in a different group", func() {
				_, err := svc.UpdateTeam(groupCtx("group-b"), updateInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("forbidden"))
			})

			It("should allow team viewer to update their own team", func() {
				result, err := svc.UpdateTeam(teamCtx("team-alpha"), updateInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should deny team viewer updating a different team", func() {
				_, err := svc.UpdateTeam(teamCtx("team-beta"), updateInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("forbidden"))
			})

			It("should deny when no viewer is present", func() {
				_, err := svc.UpdateTeam(noViewerCtx(), updateInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unauthorized"))
			})
		})

		Describe("Partial update", func() {
			It("should only update provided fields", func() {
				newEmail := "updated@example.com"
				result, err := svc.UpdateTeam(adminCtx(), model.UpdateTeamInput{
					Environment: "dev",
					Group:       "group-a",
					Name:        "team-alpha",
					Email:       &newEmail,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())

				// Verify only email changed
				team := &organizationv1.Team{}
				err = k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: "dev",
					Name:      "group-a--team-alpha",
				}, team)
				Expect(err).NotTo(HaveOccurred())
				Expect(team.Spec.Email).To(Equal("updated@example.com"))
				// Members should be unchanged from create
				Expect(team.Spec.Members).To(HaveLen(1))
				Expect(team.Spec.Members[0].Name).To(Equal("Alice"))
			})
		})
	})

	Describe("RotateTeamToken", func() {
		rotateInput := model.RotateTeamTokenInput{
			Environment: "dev",
			Group:       "group-a",
			Name:        "team-alpha",
		}

		BeforeEach(func() {
			// Seed a team first
			_, err := svc.CreateTeam(adminCtx(), createInput)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("Authorization", func() {
			It("should allow admin to rotate any team token", func() {
				result, err := svc.RotateTeamToken(adminCtx(), rotateInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should allow group viewer to rotate token for team in their group", func() {
				result, err := svc.RotateTeamToken(groupCtx("group-a"), rotateInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should deny group viewer rotating token for team in a different group", func() {
				_, err := svc.RotateTeamToken(groupCtx("group-b"), rotateInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("forbidden"))
			})

			It("should allow team viewer to rotate their own team token", func() {
				result, err := svc.RotateTeamToken(teamCtx("team-alpha"), rotateInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should deny team viewer rotating a different team's token", func() {
				_, err := svc.RotateTeamToken(teamCtx("team-beta"), rotateInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("forbidden"))
			})

			It("should deny when no viewer is present", func() {
				_, err := svc.RotateTeamToken(noViewerCtx(), rotateInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unauthorized"))
			})
		})

		Describe("Success", func() {
			It("should set Spec.Secret to 'rotate' and return correct result", func() {
				result, err := svc.RotateTeamToken(adminCtx(), rotateInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
				Expect(result.Message).To(Equal("team token rotation initiated"))
				Expect(*result.Namespace).To(Equal("dev"))
				Expect(*result.ResourceName).To(Equal("group-a--team-alpha"))

				// Verify Spec.Secret was set on the CRD
				team := &organizationv1.Team{}
				err = k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: "dev",
					Name:      "group-a--team-alpha",
				}, team)
				Expect(err).NotTo(HaveOccurred())
				Expect(team.Spec.Secret).To(Equal("rotate"))
			})
		})
	})
})

func strPtr(s string) *string {
	return &s
}
