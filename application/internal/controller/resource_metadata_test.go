// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	applicationhandler "github.com/telekom/controlplane/application/internal/handler/application"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Generated application metadata", func() {
	It("persists long names and labels without changing client IDs or resource references", func() {
		owner := &applicationv1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: strings.Repeat("a", 253), Namespace: "default", UID: "metadata-owner"},
			Spec:       applicationv1.ApplicationSpec{Team: strings.Repeat("t", 80), Secret: "test-secret"},
		}
		zone := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{Name: "metadata-zone"},
			Status: adminv1.ZoneStatus{
				Namespace:     "default",
				IdentityRealm: &ctypes.ObjectRef{Name: strings.Repeat("r", 100), Namespace: "default"},
				Gateway:       &ctypes.ObjectRef{Name: strings.Repeat("g", 100), Namespace: "default"},
			},
		}
		scoped := cclient.NewScopedClient(k8sClient, testEnvironment)
		handlerCtx := cclient.WithClient(ctx, cclient.NewJanitorClient(scoped))
		clientID := applicationhandler.MakeClientName(owner)
		key := client.ObjectKey{Namespace: "default", Name: labelutil.NormalizeNameValue(clientID + "--" + zone.Name)}

		idp, err := applicationhandler.CreateIdentityClient(handlerCtx, zone, owner)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, idp)).To(Succeed()) })
		Expect(applicationhandler.CreateGatewayConsumer(handlerCtx, zone, owner)).To(Succeed())
		consumer := &gatewayv1.Consumer{}
		Expect(k8sClient.Get(ctx, key, consumer)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, consumer)).To(Succeed()) })

		persisted := &identityv1.Client{}
		Expect(k8sClient.Get(ctx, key, persisted)).To(Succeed())
		Expect(persisted.Spec.ClientId).To(Equal(clientID))
		Expect(persisted.Spec.Realm.Name).To(Equal(zone.Status.IdentityRealm.Name))
		Expect(consumer.Spec.Name).To(Equal(clientID))
		Expect(consumer.Spec.Gateway).To(Equal(*zone.Status.Gateway))
		for _, obj := range []client.Object{persisted, consumer} {
			Expect(obj.GetLabels()).To(HaveKeyWithValue(config.BuildLabelKey("application"), labelutil.NormalizeLabelValue(owner.Name)))
			Expect(obj.GetLabels()).To(HaveKeyWithValue(config.BuildLabelKey("team"), labelutil.NormalizeLabelValue(owner.Spec.Team)))
			for _, value := range obj.GetLabels() {
				Expect(len(value)).To(BeNumerically("<=", labelutil.MaxLabelLength))
			}
		}

		By("finding the children through the normalized application selector")
		clients := &identityv1.ClientList{}
		Expect(scoped.List(ctx, clients, client.InNamespace("default"), client.MatchingLabels{
			config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(owner.Name),
		})).To(Succeed())
		Expect(clients.Items).To(HaveLen(1))

		By("reconciling without creating new children or updating unchanged resources")
		scoped.Reset()
		_, err = applicationhandler.CreateIdentityClient(handlerCtx, zone, owner)
		Expect(err).NotTo(HaveOccurred())
		Expect(applicationhandler.CreateGatewayConsumer(handlerCtx, zone, owner)).To(Succeed())
		Expect(scoped.AnyChanged()).To(BeFalse())
	})
})
