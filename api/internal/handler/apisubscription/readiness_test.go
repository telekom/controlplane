// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"
	"path/filepath"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subscription route readiness", Ordered, func() {
	var testEnv *envtest.Environment
	var scopedClient cclient.JanitorClient
	ctx := context.Background()

	BeforeAll(func() {
		testEnv = &envtest.Environment{
			CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "..", "admin", "config", "crd", "bases")},
			ErrorIfCRDPathMissing: true,
		}
		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(testEnv.Stop()).To(Succeed()) })
		scheme := runtime.NewScheme()
		Expect(adminapi.AddToScheme(scheme)).To(Succeed())
		k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())
		scopedClient = cclient.NewJanitorClient(cclient.NewScopedClient(k8sClient, "test"))
		zone := &adminapi.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name: "subscriber", Namespace: "default",
				Labels: map[string]string{config.EnvironmentLabelKey: "test"},
			},
			Spec: adminapi.ZoneSpec{
				Visibility: adminapi.ZoneVisibilityWorld,
				Gateway: adminapi.GatewayConfig{
					Admin: adminapi.GatewayAdminConfig{Url: "http://gateway.example.com"},
					Presets: []adminapi.GatewayConfigPreset{{
						Name: "default", Default: true,
						Urls: []adminapi.UrlConfig{{Hostname: "gateway.example.com", Scheme: "https", Port: 443, BasePath: "/"}},
					}},
				},
				IdentityProvider: adminapi.IdentityProviderConfig{Url: "http://idp.example.com"},
			},
		}
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		zone.Status.Namespace = "subscriber-zone"
		zone.Status.Links = adminapi.Links{Url: "https://gateway.example.com", Issuer: "https://idp.example.com", LmsIssuer: "https://lms.example.com"}
		zone.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Zone ready"))
		Expect(k8sClient.Status().Update(ctx, zone)).To(Succeed())
	})

	DescribeTable("clears previous readiness when the required route reference is missing",
		func(sameZone, failover bool, message string) {
			sub := &apiapi.ApiSubscription{Spec: apiapi.ApiSubscriptionSpec{
				Zone: types.ObjectRef{Name: "subscriber", Namespace: "default"},
			}}
			sub.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Previously ready"))
			ref, err := resolveRouteRef(ctx, scopedClient, sub, &apiapi.ApiExposure{}, sameZone, failover)
			Expect(err).NotTo(HaveOccurred())
			Expect(ref).To(BeNil())
			ready := meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(condition.ReasonPreconditionNotMet))
			Expect(ready.Message).To(Equal(message))
		},
		Entry("primary route", true, false, "Waiting for ApiExposure to create the route"),
		Entry("provider failover route", false, true, "Waiting for ApiExposure to create the failover route for this zone"),
		Entry("proxy route", false, false, "Waiting for ApiExposure to create the proxy route for this zone"),
	)
})
