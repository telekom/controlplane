// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package group_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	orgv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/group"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
)

var _ = Describe("Group Translator", func() {
	var t *group.Translator

	BeforeEach(func() {
		t = &group.Translator{}
	})

	Describe("ShouldSkip", func() {
		It("always returns false", func() {
			obj := &orgv1.Group{
				ObjectMeta: metav1.ObjectMeta{Name: "group-a"},
			}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		DescribeTable("maps Group CR to GroupData",
			func(obj *orgv1.Group, expected *group.GroupData) {
				result, err := t.Translate(context.Background(), obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expected))
			},
			Entry("full spec with display name and description",
				&orgv1.Group{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "team-platform",
						Namespace: "org",
						Labels: map[string]string{
							"cp.ei.telekom.de/environment": "production",
						},
					},
					Spec: orgv1.GroupSpec{
						DisplayName: "Platform Team",
						Description: "The platform engineering group",
					},
				},
				&group.GroupData{
					Meta:        shared.NewMetadata("org", "team-platform", map[string]string{"cp.ei.telekom.de/environment": "production"}),
					Name:        "team-platform",
					DisplayName: "Platform Team",
					Description: "The platform engineering group",
				},
			),
			Entry("empty description",
				&orgv1.Group{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "group-b",
						Namespace: "org",
					},
					Spec: orgv1.GroupSpec{
						DisplayName: "Group B",
						Description: "",
					},
				},
				&group.GroupData{
					Meta:        shared.NewMetadata("org", "group-b", nil),
					Name:        "group-b",
					DisplayName: "Group B",
					Description: "",
				},
			),
			Entry("no labels",
				&orgv1.Group{
					ObjectMeta: metav1.ObjectMeta{
						Name: "group-c",
					},
					Spec: orgv1.GroupSpec{
						DisplayName: "Group C",
						Description: "Some description",
					},
				},
				&group.GroupData{
					Meta:        shared.NewMetadata("", "group-c", nil),
					Name:        "group-c",
					DisplayName: "Group C",
					Description: "Some description",
				},
			),
		)
	})

	Describe("KeyFromObject", func() {
		It("returns GroupKey from object name", func() {
			obj := &orgv1.Group{
				ObjectMeta: metav1.ObjectMeta{Name: "test-group"},
			}
			Expect(t.KeyFromObject(obj)).To(Equal(group.GroupKey("test-group")))
		})
	})

	Describe("KeyFromDelete", func() {
		It("derives key from request name (Strong strategy)", func() {
			req := types.NamespacedName{Name: "deleted-group", Namespace: "org"}
			key, err := t.KeyFromDelete(req, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key).To(Equal(group.GroupKey("deleted-group")))
		})

		It("ignores lastKnown even when provided", func() {
			req := types.NamespacedName{Name: "deleted-group"}
			lastKnown := &orgv1.Group{
				ObjectMeta: metav1.ObjectMeta{Name: "different-name"},
			}
			key, err := t.KeyFromDelete(req, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key).To(Equal(group.GroupKey("deleted-group")))
		})
	})
})
