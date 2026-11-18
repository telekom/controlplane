// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
)

var _ = Describe("MapPublisherIdsToRovers", func() {
	It("maps listed IDs to publisher Rovers by whole-string ClientId in the same environment", func() {
		s := runtime.NewScheme()
		Expect(applicationv1.AddToScheme(s)).To(Succeed())
		mk := func(name, ns, env, clientId string) *applicationv1.Application {
			return &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{cconfig.EnvironmentLabelKey: env}},
				Status:     applicationv1.ApplicationStatus{ClientId: clientId},
			}
		}
		c := fake.NewClientBuilder().WithScheme(s).WithObjects(
			mk("pub", "ns-a", "dev", "hub--team--pub"),
			mk("pub", "ns-b", "prod", "hub--team--pub"),
			mk("other", "ns-a", "dev", "hub--team--other"),
		).Build()
		r := &RoverReconciler{Client: c}

		reqs := r.MapPublisherIdsToRovers(ctx, "dev", []string{"hub--team--pub", "hub--team--missing"})
		Expect(reqs).To(HaveLen(1))
		Expect(reqs[0]).To(Equal(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns-a", Name: "pub"}}))
		Expect(r.MapPublisherIdsToRovers(ctx, "dev", nil)).To(BeEmpty())
	})
})
