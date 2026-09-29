// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/config"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Team namespace admission", func() {
	It("rejects an oversized namespace through Kubernetes admission", func() {
		group, name := "engineering", strings.Repeat("t", 45)
		team := &organizationv1.Team{
			ObjectMeta: metav1.ObjectMeta{
				Name: organizationv1.TeamResourceName(group, name), Namespace: testNamespace,
				Labels: map[string]string{config.EnvironmentLabelKey: "prod"},
			},
			Spec: organizationv1.TeamSpec{
				Group: group, Name: name, Email: "team@example.com",
				Members: []organizationv1.Member{{Name: "Member", Email: "member@example.com"}},
			},
		}
		err := k8sClient.Create(ctx, team)
		Expect(apierrors.IsInvalid(err)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("spec.group and spec.name together may use at most 55 characters"))
	})

	DescribeTable("checks the complete namespace on create and update",
		func(environment, group, name string, valid bool) {
			team := &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:   organizationv1.TeamResourceName(group, name),
					Labels: map[string]string{config.EnvironmentLabelKey: environment},
				},
				Spec: organizationv1.TeamSpec{Group: group, Name: name, Email: "team@example.com"},
			}
			v := &TeamCustomValidator{}
			_, createErr := v.ValidateCreate(context.Background(), team)
			_, updateErr := v.ValidateUpdate(context.Background(), team.DeepCopy(), team)
			for _, err := range []error{createErr, updateErr} {
				if valid {
					Expect(err).NotTo(HaveOccurred())
					continue
				}
				Expect(apierrors.IsInvalid(err)).To(BeTrue())
				Expect(err.(apierrors.APIStatus).Status().Details.Causes[0].Field).To(Equal("spec.name"))
				Expect(err.Error()).To(ContainSubstring("generated namespace must not exceed 63 characters"))
			}
			if valid {
				return
			}
			By("rejecting before defaulting attempts to provision secrets or query the zone")
			Expect(apierrors.IsInvalid((&TeamCustomDefaulter{}).Default(context.Background(), team))).To(BeTrue())
			By("allowing deletion of an existing oversized team")
			now := metav1.Now()
			team.DeletionTimestamp = &now
			Expect((&TeamCustomDefaulter{}).Default(context.Background(), team)).To(Succeed())
			_, err := v.ValidateUpdate(context.Background(), team.DeepCopy(), team)
			Expect(err).NotTo(HaveOccurred())
			_, err = v.ValidateDelete(context.Background(), team)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("63 characters", "prod", "engineering", strings.Repeat("t", 44), true),
		Entry("64 characters", "prod", "engineering", strings.Repeat("t", 45), false),
		Entry("longer environment exhausts the budget", "prod-eu", "engineering", strings.Repeat("t", 44), false),
		Entry("long group exhausts the budget", "prod", strings.Repeat("g", 55), "t", false),
		Entry("shortened metadata does not bypass the spec length check", "prod", strings.Repeat("g", 130), strings.Repeat("t", 130), false),
	)
})
