// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newApiChangelog(name, basePath string) *apiv1.ApiChangelog {
	return &apiv1.ApiChangelog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
				apiv1.BasePathLabelKey:     labelutil.NormalizeLabelValue(basePath),
			},
		},
		Spec: apiv1.ApiChangelogSpec{BasePath: basePath, Contents: "file-id", Hash: "hash"},
	}
}

func newApiRoadmap(name, basePath string) *apiv1.ApiRoadmap {
	return &apiv1.ApiRoadmap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
				apiv1.BasePathLabelKey:     labelutil.NormalizeLabelValue(basePath),
			},
		},
		Spec: apiv1.ApiRoadmapSpec{BasePath: basePath, Contents: "file-id", Hash: "hash"},
	}
}

func hasOwnerUID(obj client.Object, api *apiv1.Api) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == api.UID && ref.Kind == "Api" {
			return true
		}
	}
	return false
}

var _ = Describe("ApiChangelog and ApiRoadmap Controllers", Ordered, func() {
	basePath := "/changelogroadmap/test/v1"

	It("should stay blocked without an Api and become ready and owned once the Api appears", func() {
		changelog := newApiChangelog("cl-late-api", basePath)
		roadmap := newApiRoadmap("rm-late-api", basePath)
		Expect(k8sClient.Create(ctx, changelog)).To(Succeed())
		Expect(k8sClient.Create(ctx, roadmap)).To(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(changelog), changelog)).To(Succeed())
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(roadmap), roadmap)).To(Succeed())
			g.Expect(meta.IsStatusConditionFalse(changelog.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			g.Expect(meta.IsStatusConditionFalse(roadmap.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
		Expect(changelog.GetOwnerReferences()).To(BeEmpty())
		Expect(roadmap.GetOwnerReferences()).To(BeEmpty())

		api := NewApi(basePath)
		Expect(k8sClient.Create(ctx, api)).To(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(api), api)).To(Succeed())
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(changelog), changelog)).To(Succeed())
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(roadmap), roadmap)).To(Succeed())
			g.Expect(hasOwnerUID(changelog, api)).To(BeTrue())
			g.Expect(hasOwnerUID(roadmap, api)).To(BeTrue())
			g.Expect(meta.IsStatusConditionTrue(changelog.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			g.Expect(meta.IsStatusConditionTrue(roadmap.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	It("should not delete the Api when the changelog or roadmap is deleted", func() {
		api := NewApi(basePath)
		changelog := newApiChangelog("cl-late-api", basePath)
		roadmap := newApiRoadmap("rm-late-api", basePath)

		Expect(k8sClient.Delete(ctx, changelog)).To(Succeed())
		Expect(k8sClient.Delete(ctx, roadmap)).To(Succeed())

		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(changelog), changelog)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(roadmap), roadmap)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}, timeout, interval).Should(Succeed())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(api), api)).To(Succeed())
		Expect(api.DeletionTimestamp.IsZero()).To(BeTrue())
	})
})
