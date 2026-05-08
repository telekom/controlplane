// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cc "github.com/telekom/controlplane/common/pkg/client"
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

	BeforeEach(func() {
		k8sClient = newFakeClient()
		svc = service.NewTeamK8sService(cc.NewScopedClient(k8sClient, "poc"))
	})

	Describe("CreateTeam", func() {
		Describe("Authorization", func() {
			It("should allow admin to create any team", func() {
				result, err := svc.CreateTeam(adminCtx(), createInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Accepted).To(BeTrue())
				Expect(result.Errors).To(BeEmpty())
			})

			It("should allow group viewer to create team in their group", func() {
				result, err := svc.CreateTeam(groupCtx("group-a"), createInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Accepted).To(BeTrue())
				Expect(result.Errors).To(BeEmpty())
			})

			It("should deny group viewer creating team in a different group", func() {
				result, err := svc.CreateTeam(groupCtx("group-b"), createInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Accepted).To(BeFalse())
				Expect(result.Errors).To(HaveLen(1))
				Expect(result.Errors[0].Code).To(Equal(model.ErrorCodeForbidden))
			})

			It("should deny team viewer from creating a team", func() {
				result, err := svc.CreateTeam(teamCtx("team-alpha"), createInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Accepted).To(BeFalse())
				Expect(result.Errors).To(HaveLen(1))
				Expect(result.Errors[0].Code).To(Equal(model.ErrorCodeForbidden))
			})

			It("should deny when no viewer is present", func() {
				result, err := svc.CreateTeam(noViewerCtx(), createInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Accepted).To(BeFalse())
				Expect(result.Errors).NotTo(BeEmpty())
			})
		})

		Describe("Success", func() {
			It("should create a team and return accepted", func() {
				result, err := svc.CreateTeam(adminCtx(), createInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Accepted).To(BeTrue())
				Expect(result.Errors).To(BeEmpty())

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

	// NOTE: UpdateTeam, AddTeamMember, RemoveTeamMember, and RotateTeamToken
	// now take Node IDs and are not yet implemented in the K8s service layer.
	// Tests for these will be added once ID resolution is wired through the resolver.
})

func strPtr(s string) *string {
	return &s
}
