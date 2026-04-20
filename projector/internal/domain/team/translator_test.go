// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package team_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	orgv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/domain/team"
)

var _ = Describe("Team Translator", func() {
	var t *team.Translator

	BeforeEach(func() {
		t = &team.Translator{}
	})

	Describe("ShouldSkip", func() {
		It("always returns false", func() {
			obj := &orgv1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: "grp--team-a"},
			}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		It("maps full Team CR to TeamData with members", func() {
			obj := &orgv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "platform--narvi",
					Namespace: "production",
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": "production",
					},
				},
				Spec: orgv1.TeamSpec{
					Name:     "narvi",
					Group:    "platform",
					Email:    "narvi@example.com",
					Category: orgv1.TeamCategoryCustomer,
					Members: []orgv1.Member{
						{Name: "Alice", Email: "alice@example.com"},
						{Name: "Bob", Email: "bob@example.com"},
					},
				},
				Status: orgv1.TeamStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionTrue,
							Message: "All good",
						},
					},
				},
			}

			result, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(&team.TeamData{
				Meta:          shared.NewMetadata("production", "platform--narvi", map[string]string{"cp.ei.telekom.de/environment": "production"}),
				StatusPhase:   "READY",
				StatusMessage: "All good",
				Name:          "platform--narvi",
				Email:         "narvi@example.com",
				Category:      "CUSTOMER",
				GroupName:     "platform",
				Members: []team.MemberData{
					{Name: "Alice", Email: "alice@example.com"},
					{Name: "Bob", Email: "bob@example.com"},
				},
			}))
		})

		It("upper-cases Infrastructure category", func() {
			obj := &orgv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "infra--ops",
					Namespace: "staging",
				},
				Spec: orgv1.TeamSpec{
					Name:     "ops",
					Group:    "infra",
					Email:    "ops@example.com",
					Category: orgv1.TeamCategoryInfrastructure,
					Members:  []orgv1.Member{{Name: "Charlie", Email: "charlie@example.com"}},
				},
			}

			result, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Category).To(Equal("INFRASTRUCTURE"))
		})

		It("handles empty members slice", func() {
			obj := &orgv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "grp--empty",
					Namespace: "dev",
				},
				Spec: orgv1.TeamSpec{
					Name:     "empty",
					Group:    "grp",
					Email:    "empty@example.com",
					Category: orgv1.TeamCategoryCustomer,
					Members:  []orgv1.Member{},
				},
			}

			result, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Members).To(BeEmpty())
		})

		It("extracts UNKNOWN status when no conditions", func() {
			obj := &orgv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "grp--no-status",
					Namespace: "dev",
				},
				Spec: orgv1.TeamSpec{
					Name:     "no-status",
					Group:    "grp",
					Email:    "ns@example.com",
					Category: orgv1.TeamCategoryCustomer,
					Members:  []orgv1.Member{{Name: "Dan", Email: "dan@example.com"}},
				},
				Status: orgv1.TeamStatus{},
			}

			result, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.StatusPhase).To(Equal("UNKNOWN"))
			Expect(result.StatusMessage).To(BeEmpty())
		})

		It("extracts ERROR status from failed condition", func() {
			obj := &orgv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "grp--err",
					Namespace: "dev",
				},
				Spec: orgv1.TeamSpec{
					Name:     "err",
					Group:    "grp",
					Email:    "err@example.com",
					Category: orgv1.TeamCategoryCustomer,
					Members:  []orgv1.Member{{Name: "Eve", Email: "eve@example.com"}},
				},
				Status: orgv1.TeamStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "Error",
							Message: "something broke",
						},
					},
				},
			}

			result, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.StatusPhase).To(Equal("ERROR"))
			Expect(result.StatusMessage).To(Equal("something broke"))
		})
	})

	Describe("KeyFromObject", func() {
		It("returns TeamKey from object name", func() {
			obj := &orgv1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: "grp--test-team"},
			}
			Expect(t.KeyFromObject(obj)).To(Equal(team.TeamKey("grp--test-team")))
		})
	})

	Describe("KeyFromDelete", func() {
		It("derives key from request name (Strong strategy)", func() {
			req := types.NamespacedName{Name: "grp--deleted-team", Namespace: "prod"}
			key, err := t.KeyFromDelete(req, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key).To(Equal(team.TeamKey("grp--deleted-team")))
		})

		It("ignores lastKnown even when provided", func() {
			req := types.NamespacedName{Name: "grp--deleted-team"}
			lastKnown := &orgv1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: "different-name"},
			}
			key, err := t.KeyFromDelete(req, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key).To(Equal(team.TeamKey("grp--deleted-team")))
		})
	})
})
