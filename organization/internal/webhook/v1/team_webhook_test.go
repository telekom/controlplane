// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"
	"strings"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/emailutil"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/secret"
	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var testMember = []organizationv1.Member{{Email: "test@example.com", Name: "member"}}

var _ = Describe("Team Webhook", func() {
	var (
		secretManagerMock *fake.MockSecretManager
		teamObj           *organizationv1.Team
		validator         TeamCustomValidator
	)
	zone := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testEnvironment,
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: adminv1.ZoneSpec{
			ManagedRoutes: &adminv1.ManagedRoutesConfig{Routes: []adminv1.ManagedRouteConfig{{
				Name: "team-api-1",
				Path: "/teamAPI",
				Url:  "https://example.org",
				Type: adminv1.ManagedRouteTypeTeamAPI,
			}}},
			Visibility: adminv1.ZoneVisibilityWorld,
			Gateway: adminv1.GatewayConfig{
				Admin: adminv1.GatewayAdminConfig{
					Url: "http://gateway-admin.test.local:8001",
				},
				Presets: []adminv1.GatewayConfigPreset{{
					Name:    "default",
					Default: true,
					Urls: []adminv1.UrlConfig{{
						Hostname: "gateway.test.local",
						BasePath: "/",
					}},
				}},
			},
			IdentityProvider: adminv1.IdentityProviderConfig{
				Url: "http://idp.test.local:8080",
				Admin: adminv1.IdentityProviderAdminConfig{
					Url: ptr.To("http://idp-admin.test.local:8080"),
				},
			},
		},
	}

	zoneStatus := adminv1.ZoneStatus{
		TeamApiIdentityRealm: &types.ObjectRef{
			Name:      "team-api-identity-realm",
			Namespace: testNamespace,
		},
		Links: adminv1.Links{
			Url:       "https://example.org",
			Issuer:    "https://example.org/issuer",
			LmsIssuer: "https://example.org/lms-issuer",
		},
	}

	BeforeEach(func() {
		By("Creating the Zone")
		freshZone := zone.DeepCopy()
		freshZone.ResourceVersion = ""
		err := k8sClient.Create(ctx, freshZone)
		Expect(err).NotTo(HaveOccurred())

		freshZone.Status = zoneStatus
		err = k8sClient.Status().Update(ctx, freshZone)
		Expect(err).NotTo(HaveOccurred())

		zone = freshZone

		secretManagerMock = fake.NewMockSecretManager(GinkgoT())
		secret.GetSecretManager = func() api.SecretManager {
			return secretManagerMock
		}

		teamObj = &organizationv1.Team{}
		validator = TeamCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(teamObj).NotTo(BeNil(), "Expected teamObj to be initialized")
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, zone)).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			freshZone := &adminv1.Zone{}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), freshZone)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	Context("When CreateOrUpdate a valid team", Ordered, func() {
		It("should skip defaulting when the team is being deleted", func() {
			teamBeingDeleted := teamObj.DeepCopy()
			now := metav1.Now()
			teamBeingDeleted.DeletionTimestamp = &now
			teamBeingDeleted.Spec.Members = []organizationv1.Member{
				{Name: "Bob", Email: "BOB@Example.COM"},
				{Name: "Alice", Email: "alice@Example.COM"},
			}
			original := teamBeingDeleted.DeepCopy()

			defaulter := TeamCustomDefaulter{client: k8sClient}
			Expect(defaulter.Default(ctx, teamBeingDeleted)).To(Succeed())
			Expect(teamBeingDeleted).To(Equal(original))
		})

		It("should return no error on valid settings", func() {
			By("Creating a team with name: spec.group--spec.name")
			teamObj = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "group-test--team-test",
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: organizationv1.TeamSpec{
					Group:   "group-test",
					Name:    "team-test",
					Email:   "test@example.com",
					Members: testMember,
				},
			}
			warning, err := validator.ValidateCreate(ctx, teamObj)
			Expect(warning).To(BeNil())
			Expect(err).NotTo(HaveOccurred())

			warning, err = validator.ValidateDelete(ctx, teamObj)
			Expect(warning).To(BeNil())
			Expect(err).NotTo(HaveOccurred())
		})
		It("should return same result as create", func() {
			By("Updating a team with name: spec.group--spec.name")
			teamObj = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "group-test--team-test",
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: organizationv1.TeamSpec{
					Group:   "group-test",
					Name:    "team-test",
					Email:   "test@example.com",
					Members: testMember,
				},
			}
			warning, err := validator.ValidateUpdate(ctx, teamObj, teamObj)
			Expect(warning).To(BeNil())
			Expect(err).NotTo(HaveOccurred())
			warning, err = validator.ValidateDelete(ctx, teamObj)
			Expect(warning).To(BeNil())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When CreateOrUpdate an invalid team", func() {
		It("reports precise contact and member syntax fields", func() {
			teamObj.ObjectMeta = metav1.ObjectMeta{Name: "group-test--team-test", Labels: map[string]string{config.EnvironmentLabelKey: testEnvironment}}
			teamObj.Spec = organizationv1.TeamSpec{Group: "group-test", Name: "team-test", Email: "Contact <contact@example.com>", Members: []organizationv1.Member{{Name: "Invalid", Email: " alice@example.com"}}}
			_, err := validator.ValidateCreate(ctx, teamObj)
			Expect(errors.IsInvalid(err)).To(BeTrue())
			status := err.(errors.APIStatus).Status()
			Expect(status.Details.Causes).To(HaveLen(2))
			Expect(status.Details.Causes[0].Field).To(Equal("spec.email"))
			Expect(status.Details.Causes[1].Field).To(Equal("spec.members[0].email"))
			Expect(status.Details.Causes[1].Type).To(Equal(metav1.CauseTypeFieldValueInvalid))
			now := metav1.Now()
			teamObj.DeletionTimestamp = &now
			_, err = validator.ValidateUpdate(ctx, teamObj, teamObj)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should return an error", func() {
			By("Creating a Team with name completely different from spec.group--spec.name")
			teamObj = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "here-is-a--complete-mismatch",
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: organizationv1.TeamSpec{
					Group:   "group-test",
					Name:    "team-test",
					Email:   "test@example.com",
					Members: testMember,
				},
			}
			warning, err := validator.ValidateCreate(ctx, teamObj)
			Expect(warning).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("must be equal to 'spec.group--spec.name'"))
		})
		It("should return an error since env is missing", func() {
			By("Creating a Team with name completely different from spec.group--spec.name")
			teamObj = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "group-test--team-test",
					Namespace: testNamespace,
				},
				Spec: organizationv1.TeamSpec{
					Group:   "group-test",
					Name:    "team-test",
					Email:   "test@example.com",
					Members: testMember,
				},
			}
			warning, err := validator.ValidateCreate(ctx, teamObj)
			Expect(warning).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("must contain an environment label"))
		})
	})

	Context("When inserting an valid team against the k8s", Ordered, func() {
		var localTeam *organizationv1.Team
		BeforeAll(func() {
			localTeam = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "group-test--team-test",
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: organizationv1.TeamSpec{
					Group: "group-test",
					Name:  "team-test",
					Email: "Contact@Example.COM",
					Members: []organizationv1.Member{
						{Name: "Alice", Email: "alice@Example.COM"},
						{Name: "Bob", Email: "BOB@Example.COM"},
					},
					Category: organizationv1.TeamCategoryCustomer,
				},
			}
			secretManagerMock.EXPECT().
				UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(map[string]string{
					"clientSecret": string(uuid.NewUUID()),
					"teamToken":    string(uuid.NewUUID()),
				}, nil)
			err := k8sClient.Create(ctx, localTeam)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(
			func() {
				By("Deleting the team")
				err := k8sClient.Delete(ctx, localTeam)
				Expect(err).NotTo(HaveOccurred())
			})

		It("should normalize member emails before sorting on create and update and remain idempotent", func() {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(localTeam), localTeam)).To(Succeed())
			Expect(localTeam.Spec.Members).To(Equal([]organizationv1.Member{
				{Name: "Alice", Email: "alice@example.com"},
				{Name: "Bob", Email: "bob@example.com"},
			}))
			Expect(localTeam.Spec.Email).To(Equal("Contact@Example.COM"))
			normalized := localTeam.DeepCopy()

			localTeam.Spec.Members = []organizationv1.Member{
				{Name: "Bob", Email: "BOB@EXAMPLE.COM"},
				{Name: "Alice", Email: "alice@EXAMPLE.COM"},
			}
			Expect(k8sClient.Update(ctx, localTeam)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(localTeam), localTeam)).To(Succeed())
			Expect(localTeam.Spec).To(Equal(normalized.Spec))

			Expect(k8sClient.Update(ctx, localTeam)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(localTeam), localTeam)).To(Succeed())
			Expect(localTeam.Spec).To(Equal(normalized.Spec))
		})

		It("should set secret", func() {
			Eventually(func(g Gomega) {
				By("Checking the team secret to be set")
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(localTeam), localTeam)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(localTeam.Spec.Secret).NotTo(BeEmpty())
				g.Expect(strings.HasPrefix(localTeam.Spec.Secret, "$<")).To(BeTrueBecause("client secret does not end with $<"))
				g.Expect(strings.HasSuffix(localTeam.Spec.Secret, ">")).To(BeTrueBecause("client secret does not end with >"))
			}, timeout, interval).Should(Succeed())
		})
		It("should update the secret if empty", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(localTeam), localTeam)
			Expect(err).NotTo(HaveOccurred())
			By("Setting the secret to empty")
			localTeam.Spec.Secret = ""

			secretManagerMock.EXPECT().
				UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(map[string]string{
					"clientSecret": string(uuid.NewUUID()),
					"teamToken":    string(uuid.NewUUID()),
				}, nil)
			err = k8sClient.Update(ctx, localTeam)
			Eventually(func(g Gomega) {
				By("Checking the team secret to be set")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(localTeam), localTeam)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(localTeam.Spec.Secret).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})
		It("should rotate the secret if rotate", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(localTeam), localTeam)
			Expect(err).NotTo(HaveOccurred())
			By("Setting the secret to rotate")
			localTeam.Spec.Secret = "rotate"
			secretManagerMock.EXPECT().
				UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(map[string]string{
					"clientSecret": string(uuid.NewUUID()),
					"teamToken":    string(uuid.NewUUID()),
				}, nil)
			err = k8sClient.Update(ctx, localTeam)
			Eventually(func(g Gomega) {
				By("Checking the team secret to be updated")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(localTeam), localTeam)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(localTeam.Spec.Secret).NotTo(BeEmpty())
				g.Expect(localTeam.Spec.Secret).NotTo(BeEquivalentTo("rotate"))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Email admission policy", func() {
		BeforeEach(func() {
			teamObj = &organizationv1.Team{
				TypeMeta:   metav1.TypeMeta{APIVersion: organizationv1.GroupVersion.String(), Kind: "Team"},
				ObjectMeta: metav1.ObjectMeta{Name: "group-test--email-policy", Namespace: testNamespace, Labels: map[string]string{config.EnvironmentLabelKey: testEnvironment}},
				Spec:       organizationv1.TeamSpec{Group: "group-test", Name: "email-policy", Email: "Contact@Example.COM", Category: organizationv1.TeamCategoryCustomer, Secret: "$<existing>", Members: []organizationv1.Member{{Name: "Alice", Email: "Alice@Example.COM"}}},
			}
		})

		It("admits Unicode unchanged on create and update and keeps Unicode case distinct", func() {
			teamObj.Spec.Members = []organizationv1.Member{{Name: "Lower", Email: "üSER@Example.COM"}, {Name: "Upper", Email: "ÜSER@Example.COM"}}
			Expect(k8sClient.Create(ctx, teamObj)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, teamObj)).To(Succeed()) })
			Expect(teamObj.Spec.Members).To(ConsistOf(organizationv1.Member{Name: "Lower", Email: "üser@example.com"}, organizationv1.Member{Name: "Upper", Email: "Üser@example.com"}))
			teamObj.Spec.Members[0].Name = "Updated"
			Expect(k8sClient.Update(ctx, teamObj)).To(Succeed())
			Expect(teamObj.Spec.Email).To(Equal("Contact@Example.COM"))
		})

		It("rejects ASCII duplicates and malformed addresses on create and update", func() {
			bad := teamObj.DeepCopy()
			bad.Spec.Members = append(bad.Spec.Members, organizationv1.Member{Name: "Duplicate", Email: "ALICE@example.com"})
			Expect(k8sClient.Create(ctx, bad)).NotTo(Succeed())
			bad = teamObj.DeepCopy()
			bad.Spec.Email = "Contact <contact@example.com>"
			Expect(k8sClient.Create(ctx, bad)).NotTo(Succeed())
			bad = teamObj.DeepCopy()
			bad.Spec.Members[0].Email = " alice@example.com"
			Expect(k8sClient.Create(ctx, bad)).NotTo(Succeed())
			Expect(k8sClient.Create(ctx, teamObj)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, teamObj)).To(Succeed()) })
			bad = teamObj.DeepCopy()
			bad.Spec.Email = " contact@example.com"
			Expect(k8sClient.Update(ctx, bad)).NotTo(Succeed())
			bad = teamObj.DeepCopy()
			bad.Spec.Members[0].Email = "Alice <alice@example.com>"
			Expect(k8sClient.Update(ctx, bad)).NotTo(Succeed())
			bad = teamObj.DeepCopy()
			bad.Spec.Members = append(bad.Spec.Members, organizationv1.Member{Name: "Duplicate", Email: "ALICE@example.com"})
			Expect(k8sClient.Update(ctx, bad)).NotTo(Succeed())
		})

		DescribeTable("admits bare punctuation through the existing schema", func(email, expected string) {
			teamObj.Spec.Members[0].Email = email
			Expect(k8sClient.Create(ctx, teamObj)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, teamObj)).To(Succeed()) })
			Expect(teamObj.Spec.Members[0].Email).To(Equal(expected))
		}, Entry("tags and percent", "A%_ICE+Tag@Example.COM", "a%_ice+tag@example.com"), Entry("apostrophe", "O'NEIL@Example.COM", "o'neil@example.com"), Entry("quoted mailbox", `"Alice Smith"@Example.COM`, `"alice smith"@example.com`))

		It("rejects repeated mixed-case SSA keys and supports canonical-key edits and removals", func() {
			teamObj.Spec.Members = append(teamObj.Spec.Members, organizationv1.Member{Name: "Bob", Email: "BOB@Example.COM"})
			apply := func(obj *organizationv1.Team) error {
				payload, err := json.Marshal(obj)
				Expect(err).NotTo(HaveOccurred())
				target := &organizationv1.Team{ObjectMeta: metav1.ObjectMeta{Name: obj.Name, Namespace: obj.Namespace}}
				return k8sClient.Patch(ctx, target, client.RawPatch(k8stypes.ApplyPatchType, payload), client.FieldOwner("email-policy-test"))
			}
			Expect(apply(teamObj)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, teamObj)).To(Succeed()) })
			for range 2 {
				err := apply(teamObj)
				Expect(errors.IsInvalid(err)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("Duplicate value"))
			}
			for i := range teamObj.Spec.Members {
				teamObj.Spec.Members[i].Email = emailutil.Canonicalize(teamObj.Spec.Members[i].Email)
			}
			Expect(apply(teamObj)).To(Succeed())
			Expect(apply(teamObj)).To(Succeed())
			teamObj.Spec.Members[0].Name = "Alice Updated"
			Expect(apply(teamObj)).To(Succeed())
			teamObj.Spec.Members = teamObj.Spec.Members[:1]
			Expect(apply(teamObj)).To(Succeed())
			stored := &organizationv1.Team{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(teamObj), stored)).To(Succeed())
			Expect(stored.Spec.Members).To(Equal([]organizationv1.Member{{Name: "Alice Updated", Email: "alice@example.com"}}))
		})
	})

	Context("When inserting an invalid team against  k8s", func() {
		It("should return an error from the webhook", func() {
			teamObj = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "here-is-a--complete-mismatch",
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: organizationv1.TeamSpec{
					Group:    "group-test",
					Name:     "team-test",
					Email:    "test@example.com",
					Members:  testMember,
					Category: organizationv1.TeamCategoryCustomer,
				},
			}
			secretManagerMock.EXPECT().
				UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(map[string]string{
					"clientSecret": string(uuid.NewUUID()),
					"teamToken":    string(uuid.NewUUID()),
				}, nil)
			err := k8sClient.Create(ctx, teamObj)
			Expect(errors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("Invalid value: \"here-is-a--complete-mismatch\": must be equal to 'spec.group--spec.name'"))
		})
	})
})
