// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiSpecification admission (envtest)", Ordered, func() {
	const (
		env         = "catenv"
		otherEnv    = "catother"
		group       = "grp"
		teamName    = "team"
		specNs      = env + "--" + group + "--" + teamName
		categoryNs  = "catenv-categories"
		otherCatNs  = "aaa-catother-categories"
		timeout     = 10 * time.Second
		interval    = 250 * time.Millisecond
		categoryKey = "spec.category"
	)

	envLabels := func(e string) map[string]string { return map[string]string{config.EnvironmentLabelKey: e} }

	newCategory := func(ns, name, label, e string, active bool) *apiv1.ApiCategory {
		return &apiv1.ApiCategory{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: envLabels(e)},
			Spec:       apiv1.ApiCategorySpec{LabelValue: label, Active: active, MustHaveGroupPrefix: true},
		}
	}

	newSpec := func(name, category string) *roverv1.ApiSpecification {
		return &roverv1.ApiSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: specNs, Labels: envLabels(env)},
			Spec: roverv1.ApiSpecificationSpec{
				Specification: "file-id",
				Category:      category,
				BasePath:      "/" + group + "/" + name + "/v1",
				Hash:          "hash",
				Version:       "1.0.0",
			},
		}
	}

	BeforeAll(func() {
		for _, ns := range []string{env, specNs, categoryNs, otherCatNs} {
			CreateNamespace(ns)
		}
		Expect(k8sClient.Create(ctx, &organizationv1.Team{
			ObjectMeta: metav1.ObjectMeta{Name: group + "--" + teamName, Namespace: env, Labels: envLabels(env)},
			Spec: organizationv1.TeamSpec{
				Name: teamName, Group: group, Email: "team@example.com",
				Category: organizationv1.TeamCategoryCustomer,
				Members:  []organizationv1.Member{{Name: "m", Email: "m@example.com"}},
			},
		})).To(Succeed())

		// Same label (case-insensitively) in another environment, in a namespace listed first.
		Expect(k8sClient.Create(ctx, newCategory(otherCatNs, "public", "PUBLIC", otherEnv, true))).To(Succeed())
		Expect(k8sClient.Create(ctx, newCategory(categoryNs, "public", "Public", env, true))).To(Succeed())
		Expect(k8sClient.Create(ctx, newCategory(categoryNs, "retired", "Retired", env, false))).To(Succeed())
	})

	AfterAll(func() {
		for _, ns := range []string{env, specNs, categoryNs, otherCatNs} {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}))).To(Succeed())
		}
	})

	getCategory := func(name string) func(g Gomega) string {
		return func(g Gomega) string {
			fresh := &roverv1.ApiSpecification{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: specNs, Name: name}, fresh)).To(Succeed())
			return fresh.Spec.Category
		}
	}

	It("persists the canonical labelValue of the environment's category on create", func() {
		spec := newSpec("create-api", "PUBLIC")
		Expect(k8sClient.Create(ctx, spec)).To(Succeed())
		Expect(spec.Spec.Category).To(Equal("Public"), "object returned by the API server")
		Eventually(getCategory("create-api")).WithTimeout(timeout).WithPolling(interval).Should(Equal("Public"))
	})

	It("persists the canonical labelValue on update and stays idempotent", func() {
		fresh := &roverv1.ApiSpecification{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: specNs, Name: "create-api"}, fresh)).To(Succeed())
		fresh.Spec.Category = "public"
		Expect(k8sClient.Update(ctx, fresh)).To(Succeed())
		Expect(fresh.Spec.Category).To(Equal("Public"))
		Eventually(getCategory("create-api")).WithTimeout(timeout).WithPolling(interval).Should(Equal("Public"))

		rv := fresh.ResourceVersion
		Expect(k8sClient.Update(ctx, fresh)).To(Succeed())
		Expect(fresh.Spec.Category).To(Equal("Public"))
		Expect(fresh.ResourceVersion).To(Equal(rv), "no-op update must not change the object")
	})

	It("still rejects inactive categories", func() {
		err := k8sClient.Create(ctx, newSpec("inactive-api", "RETIRED"))
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "got %v", err)
		Expect(err.Error()).To(ContainSubstring(categoryKey))
		Expect(err.Error()).To(ContainSubstring("not active"))
	})

	It("still rejects unknown categories", func() {
		err := k8sClient.Create(ctx, newSpec("unknown-api", "internal"))
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "got %v", err)
		Expect(err.Error()).To(ContainSubstring(`ApiCategory "internal" not found`))
	})

	It("still enforces the group prefix policy", func() {
		spec := newSpec("prefix-api", "public")
		spec.Spec.BasePath = "/other/prefix-api/v1"
		err := k8sClient.Create(ctx, spec)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "got %v", err)
		Expect(err.Error()).To(ContainSubstring("team group prefix"))
	})
})
