// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// disableSecretManager disables the FeatureSecretManager feature flag
// via environment variable and returns a cleanup function to restore it.
func disableSecretManager() func() {
	os.Setenv("FEATURE_SECRET_MANAGER_ENABLED", "false")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	config.Parse()

	return func() {
		os.Unsetenv("FEATURE_SECRET_MANAGER_ENABLED")
		viper.Reset()
		config.Parse()
	}
}

// enableSecretManager enables the FeatureSecretManager feature flag
// via environment variable and returns a cleanup function to restore it.
func enableSecretManager() func() {
	os.Setenv("FEATURE_SECRET_MANAGER_ENABLED", "true")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	config.Parse()

	return func() {
		os.Unsetenv("FEATURE_SECRET_MANAGER_ENABLED")
		viper.Reset()
		config.Parse()
	}
}

func ptr(s string) *string {
	return &s
}

// newValidZone creates a Zone with all required fields populated.
func newValidZone() *adminv1.Zone {
	identityAdminUrl := "https://idp.example.com/admin"
	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-zone",
			Namespace: "default",
			Labels: map[string]string{
				config.EnvironmentLabelKey: "test-env",
			},
		},
		Spec: adminv1.ZoneSpec{
			IdentityProviders: []adminv1.IdentityProviderConfig{{
				Name: "primary",
				Admin: adminv1.IdentityProviderAdminConfig{
					Url:      &identityAdminUrl,
					ClientId: "admin-client",
					UserName: "admin",
					Password: "",
				},
				TokenUrl: "https://idp.example.com/token",
			}},
			Gateways: []adminv1.GatewayConfig{{
				Name: "standard",
				Admin: adminv1.GatewayAdminConfig{
					IdentityProviderRef: "primary",
					Url:                 "https://gateway.example.com/admin",
				},
			}},
			Presets: []adminv1.Preset{{
				Name:                "default",
				Type:                adminv1.GatewayTypeAPI,
				Default:             true,
				GatewayRef:          "standard",
				IdentityProviderRef: "primary",
				Urls:                []adminv1.UrlConfig{{Hostname: "gateway.example.com", Scheme: "https", BasePath: "/"}},
			}},
			Redis: &adminv1.RedisConfig{
				Host:     "redis://redis-master:6379",
				Port:     6379,
				Password: "",
			},
			Visibility: adminv1.ZoneVisibilityEnterprise,
		},
	}
}

// updateContextWithOldObject returns a context that simulates an UPDATE admission request
// with the given old Zone as the previous version of the resource.
func updateContextWithOldObject(parent context.Context, oldObj *adminv1.Zone) context.Context {
	raw, err := json.Marshal(oldObj)
	Expect(err).NotTo(HaveOccurred())

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			OldObject: runtime.RawExtension{Raw: raw},
		},
	}
	return admission.NewContextWithRequest(parent, req)
}

var _ = Describe("Zone Webhook", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Defaulting Tests — Secret Manager Disabled
	// ─────────────────────────────────────────────────────────────────────────

	Context("When defaulting Zone with Secret Manager disabled", func() {
		var (
			defaulter ZoneCustomDefaulter
			cleanup   func()
		)

		BeforeEach(func() {
			cleanup = disableSecretManager()
			defaulter = ZoneCustomDefaulter{secretManager: nil}
		})

		AfterEach(func() {
			cleanup()
		})

		Context("on CREATE", func() {
			It("supports zones without Redis", func() {
				obj := newValidZone()
				obj.Spec.Redis = nil

				Expect(defaulter.Default(ctx, obj)).To(Succeed())
				Expect(obj.Spec.Redis).To(BeNil())
			})

			It("should generate IDP admin password when empty", func() {
				obj := newValidZone()
				obj.Spec.IdentityProviders[0].Admin.Password = ""

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.IdentityProviders[0].Admin.Password).NotTo(BeEmpty())
				Expect(obj.Spec.IdentityProviders[0].Admin.Password).To(HavePrefix("trd_"))
			})

			It("should generate Redis password when empty", func() {
				obj := newValidZone()
				obj.Spec.Redis.Password = ""

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Redis.Password).NotTo(BeEmpty())
				Expect(obj.Spec.Redis.Password).To(HavePrefix("trd_"))
			})

			It("should generate gateway client secret when non-nil and empty", func() {
				obj := newValidZone()
				obj.Spec.Gateways[0].Admin.ClientSecret = ptr("")

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Gateways[0].Admin.ClientSecret).NotTo(BeNil())
				Expect(*obj.Spec.Gateways[0].Admin.ClientSecret).NotTo(BeEmpty())
				Expect(*obj.Spec.Gateways[0].Admin.ClientSecret).To(HavePrefix("trd_"))
			})

			It("should not generate gateway client secret when nil", func() {
				obj := newValidZone()
				obj.Spec.Gateways[0].Admin.ClientSecret = nil

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Gateways[0].Admin.ClientSecret).To(BeNil())
			})

			It("should rotate IDP admin password when set to 'rotate'", func() {
				obj := newValidZone()
				obj.Spec.IdentityProviders[0].Admin.Password = secretsapi.KeywordRotate

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.IdentityProviders[0].Admin.Password).NotTo(Equal(secretsapi.KeywordRotate))
				Expect(obj.Spec.IdentityProviders[0].Admin.Password).NotTo(BeEmpty())
			})

			It("should rotate Redis password when set to 'rotate'", func() {
				obj := newValidZone()
				obj.Spec.Redis.Password = secretsapi.KeywordRotate

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Redis.Password).NotTo(Equal(secretsapi.KeywordRotate))
				Expect(obj.Spec.Redis.Password).NotTo(BeEmpty())
			})

			It("should preserve existing non-empty secrets", func() {
				obj := newValidZone()
				obj.Spec.IdentityProviders[0].Admin.Password = "my-idp-password"
				obj.Spec.Redis.Password = "my-redis-password"
				obj.Spec.Gateways[0].Admin.ClientSecret = ptr("my-gw-secret")

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.IdentityProviders[0].Admin.Password).To(Equal("my-idp-password"))
				Expect(obj.Spec.Redis.Password).To(Equal("my-redis-password"))
				Expect(*obj.Spec.Gateways[0].Admin.ClientSecret).To(Equal("my-gw-secret"))
			})
		})

		Context("on UPDATE", func() {
			It("preserves gateway secrets by name when gateway order changes", func() {
				oldObj := newValidZone()
				oldObj.Spec.Gateways[0].Admin.ClientSecret = ptr("standard-secret")
				ai := adminv1.GatewayConfig{Name: "ai", Admin: adminv1.GatewayAdminConfig{ClientSecret: ptr("ai-secret")}}
				oldObj.Spec.Gateways = append(oldObj.Spec.Gateways, ai)

				newObj := oldObj.DeepCopy()
				newObj.Spec.Gateways = []adminv1.GatewayConfig{ai, oldObj.Spec.Gateways[0]}
				newObj.Spec.Gateways[0].Admin.ClientSecret = nil
				newObj.Spec.Gateways[1].Admin.ClientSecret = ptr("")

				Expect(defaulter.Default(updateContextWithOldObject(ctx, oldObj), newObj)).To(Succeed())
				Expect(*newObj.Spec.Gateways[0].Admin.ClientSecret).To(Equal("ai-secret"))
				Expect(*newObj.Spec.Gateways[1].Admin.ClientSecret).To(Equal("standard-secret"))
			})

			It("rotates only the named gateway carrying rotate", func() {
				oldObj := newValidZone()
				oldObj.Spec.Gateways[0].Admin.ClientSecret = ptr("standard-secret")
				ai := adminv1.GatewayConfig{Name: "ai", Admin: adminv1.GatewayAdminConfig{ClientSecret: ptr("ai-secret")}}
				oldObj.Spec.Gateways = append(oldObj.Spec.Gateways, ai)

				newObj := oldObj.DeepCopy()
				newObj.Spec.Gateways = []adminv1.GatewayConfig{ai, oldObj.Spec.Gateways[0]}
				newObj.Spec.Gateways[0].Admin.ClientSecret = ptr(secretsapi.KeywordRotate)
				newObj.Spec.Gateways[1].Admin.ClientSecret = nil

				Expect(defaulter.Default(updateContextWithOldObject(ctx, oldObj), newObj)).To(Succeed())
				Expect(*newObj.Spec.Gateways[0].Admin.ClientSecret).NotTo(Equal("ai-secret"))
				Expect(*newObj.Spec.Gateways[0].Admin.ClientSecret).NotTo(Equal(secretsapi.KeywordRotate))
				Expect(*newObj.Spec.Gateways[1].Admin.ClientSecret).To(Equal("standard-secret"))
			})

			It("should preserve existing secrets when new value is empty", func() {
				oldObj := newValidZone()
				oldObj.Spec.IdentityProviders[0].Admin.Password = "old-idp-password"
				oldObj.Spec.Redis.Password = "old-redis-password"
				oldObj.Spec.Gateways[0].Admin.ClientSecret = ptr("old-gw-secret")

				newObj := newValidZone()
				newObj.Spec.IdentityProviders[0].Admin.Password = ""
				newObj.Spec.Redis.Password = ""
				newObj.Spec.Gateways[0].Admin.ClientSecret = nil

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.IdentityProviders[0].Admin.Password).To(Equal("old-idp-password"))
				Expect(newObj.Spec.Redis.Password).To(Equal("old-redis-password"))
				Expect(newObj.Spec.Gateways[0].Admin.ClientSecret).NotTo(BeNil())
				Expect(*newObj.Spec.Gateways[0].Admin.ClientSecret).To(Equal("old-gw-secret"))
			})

			It("should rotate secrets when set to 'rotate' even on update", func() {
				oldObj := newValidZone()
				oldObj.Spec.IdentityProviders[0].Admin.Password = "old-idp-password"
				oldObj.Spec.Redis.Password = "old-redis-password"

				newObj := newValidZone()
				newObj.Spec.IdentityProviders[0].Admin.Password = secretsapi.KeywordRotate
				newObj.Spec.Redis.Password = secretsapi.KeywordRotate

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.IdentityProviders[0].Admin.Password).NotTo(Equal(secretsapi.KeywordRotate))
				Expect(newObj.Spec.IdentityProviders[0].Admin.Password).NotTo(Equal("old-idp-password"))
				Expect(newObj.Spec.Redis.Password).NotTo(Equal(secretsapi.KeywordRotate))
				Expect(newObj.Spec.Redis.Password).NotTo(Equal("old-redis-password"))
			})

			It("should preserve user-provided non-empty secret on update", func() {
				oldObj := newValidZone()
				oldObj.Spec.IdentityProviders[0].Admin.Password = "old-idp-password"
				oldObj.Spec.Redis.Password = "old-redis-password"

				newObj := newValidZone()
				newObj.Spec.IdentityProviders[0].Admin.Password = "new-idp-password"
				newObj.Spec.Redis.Password = "new-redis-password"

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.IdentityProviders[0].Admin.Password).To(Equal("new-idp-password"))
				Expect(newObj.Spec.Redis.Password).To(Equal("new-redis-password"))
			})
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Defaulting Tests — Secret Manager Enabled
	// ─────────────────────────────────────────────────────────────────────────

	Context("When defaulting Zone with Secret Manager enabled", func() {
		var (
			defaulter         ZoneCustomDefaulter
			secretManagerMock *fake.MockSecretManager
			cleanup           func()
		)

		BeforeEach(func() {
			cleanup = enableSecretManager()
			secretManagerMock = fake.NewMockSecretManager(GinkgoT())
			defaulter = ZoneCustomDefaulter{secretManager: secretManagerMock}
		})

		AfterEach(func() {
			cleanup()
		})

		Context("on CREATE", func() {
			It("onboards every named gateway and the sole identity provider", func() {
				obj := newValidZone()
				obj.Spec.Gateways[0].Admin.ClientSecret = ptr("standard-secret")
				obj.Spec.Gateways = append(obj.Spec.Gateways, adminv1.GatewayConfig{
					Name: "ai",
					Admin: adminv1.GatewayAdminConfig{
						IdentityProviderRef: "primary",
						Url:                 "https://ai-gateway.example.com/admin",
						ClientSecret:        ptr("ai-secret"),
					},
				})
				obj.Spec.IdentityProviders[0].Admin.Password = "idp-secret"
				obj.Spec.Redis.Password = "$<existing-redis-ref>"

				standardPath := "zones/test-zone/admin/gateways/standard/clientSecret"
				aiPath := "zones/test-zone/admin/gateways/ai/clientSecret"
				idpPath := "zones/test-zone/admin/identityProviders/primary/password"
				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Run(func(_ context.Context, _ string, opts ...secretsapi.OnboardingOption) {
						options := &secretsapi.OnboardingOptions{}
						for _, option := range opts {
							option(options)
						}
						Expect(options.SecretValues).To(Equal(map[string]any{
							standardPath: "standard-secret",
							aiPath:       "ai-secret",
							idpPath:      "idp-secret",
						}))
					}).
					Return(map[string]string{
						standardPath: "standard-secret-ref",
						aiPath:       "ai-secret-ref",
						idpPath:      "idp-secret-ref",
					}, nil)

				Expect(defaulter.OnboardSecrets(ctx, obj)).To(Succeed())
				Expect(*obj.Spec.Gateways[0].Admin.ClientSecret).To(Equal("$<standard-secret-ref>"))
				Expect(*obj.Spec.Gateways[1].Admin.ClientSecret).To(Equal("$<ai-secret-ref>"))
				Expect(obj.Spec.IdentityProviders[0].Admin.Password).To(Equal("$<idp-secret-ref>"))
			})

			It("should onboard secrets and set secret refs when empty", func() {
				obj := newValidZone()
				obj.Spec.IdentityProviders[0].Admin.Password = ""
				obj.Spec.Redis.Password = ""

				idpSecretPath := "zones/test-zone/admin/identityProviders/primary/password"
				redisSecretPath := "zones/test-zone/admin/redis/password"
				gatewaySecretPath := "zones/test-zone/admin/gateways/standard/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						idpSecretPath:     "idp-secret-uuid",
						redisSecretPath:   "redis-secret-uuid",
						gatewaySecretPath: "gw-secret-uuid",
					}, nil)

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.IdentityProviders[0].Admin.Password).To(Equal("$<idp-secret-uuid>"))
				Expect(obj.Spec.Redis.Password).To(Equal("$<redis-secret-uuid>"))
				Expect(obj.Spec.Gateways[0].Admin.ClientSecret).NotTo(BeNil())
				Expect(*obj.Spec.Gateways[0].Admin.ClientSecret).To(Equal("$<gw-secret-uuid>"))
			})

			It("should onboard gateway secret when provided", func() {
				obj := newValidZone()
				obj.Spec.IdentityProviders[0].Admin.Password = "$<existing-idp-ref>"
				obj.Spec.Redis.Password = "$<existing-redis-ref>"
				obj.Spec.Gateways[0].Admin.ClientSecret = ptr("my-gw-secret")

				gatewaySecretPath := "zones/test-zone/admin/gateways/standard/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything).
					Return(map[string]string{
						gatewaySecretPath: "gw-secret-uuid",
					}, nil)

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Gateways[0].Admin.ClientSecret).NotTo(BeNil())
				Expect(*obj.Spec.Gateways[0].Admin.ClientSecret).To(Equal("$<gw-secret-uuid>"))
			})

			It("should onboard a generated gateway secret when nil", func() {
				obj := newValidZone()
				obj.Spec.IdentityProviders[0].Admin.Password = "$<existing-idp-ref>"
				obj.Spec.Redis.Password = "$<existing-redis-ref>"
				obj.Spec.Gateways[0].Admin.ClientSecret = nil

				gatewaySecretPath := "zones/test-zone/admin/gateways/standard/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything).
					Return(map[string]string{
						gatewaySecretPath: "gw-secret-uuid",
					}, nil)

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Gateways[0].Admin.ClientSecret).NotTo(BeNil())
				Expect(*obj.Spec.Gateways[0].Admin.ClientSecret).To(Equal("$<gw-secret-uuid>"))
			})

			It("should upload user-provided plain secrets to secret manager", func() {
				obj := newValidZone()
				obj.Spec.IdentityProviders[0].Admin.Password = "my-custom-idp-password"
				obj.Spec.Redis.Password = "my-custom-redis-password"

				idpSecretPath := "zones/test-zone/admin/identityProviders/primary/password"
				redisSecretPath := "zones/test-zone/admin/redis/password"
				gatewaySecretPath := "zones/test-zone/admin/gateways/standard/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						idpSecretPath:     "idp-custom-uuid",
						redisSecretPath:   "redis-custom-uuid",
						gatewaySecretPath: "gw-custom-uuid",
					}, nil)

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.IdentityProviders[0].Admin.Password).To(Equal("$<idp-custom-uuid>"))
				Expect(obj.Spec.Redis.Password).To(Equal("$<redis-custom-uuid>"))
			})

			It("should generate new secrets when set to 'rotate'", func() {
				obj := newValidZone()
				obj.Spec.IdentityProviders[0].Admin.Password = secretsapi.KeywordRotate
				obj.Spec.Redis.Password = secretsapi.KeywordRotate

				idpSecretPath := "zones/test-zone/admin/identityProviders/primary/password"
				redisSecretPath := "zones/test-zone/admin/redis/password"
				gatewaySecretPath := "zones/test-zone/admin/gateways/standard/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						idpSecretPath:     "idp-rotated-uuid",
						redisSecretPath:   "redis-rotated-uuid",
						gatewaySecretPath: "gw-rotated-uuid",
					}, nil)

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.IdentityProviders[0].Admin.Password).To(Equal("$<idp-rotated-uuid>"))
				Expect(obj.Spec.Redis.Password).To(Equal("$<redis-rotated-uuid>"))
			})
		})

		Context("on UPDATE", func() {
			It("should preserve existing secret ref when new value is empty", func() {
				oldObj := newValidZone()
				oldObj.Spec.IdentityProviders[0].Admin.Password = "$<existing-idp-ref>"
				oldObj.Spec.Redis.Password = "$<existing-redis-ref>"

				newObj := newValidZone()
				newObj.Spec.IdentityProviders[0].Admin.Password = ""
				newObj.Spec.Redis.Password = ""

				gatewaySecretPath := "zones/test-zone/admin/gateways/standard/clientSecret"

				// After resolving, IDP and Redis secrets become the old refs (already refs).
				// Gateway is nil so it still needs onboarding.
				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything).
					Return(map[string]string{
						gatewaySecretPath: "gw-secret-uuid",
					}, nil)

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.IdentityProviders[0].Admin.Password).To(Equal("$<existing-idp-ref>"))
				Expect(newObj.Spec.Redis.Password).To(Equal("$<existing-redis-ref>"))
			})

			It("should rotate secrets when set to 'rotate' even on update", func() {
				oldObj := newValidZone()
				oldObj.Spec.IdentityProviders[0].Admin.Password = "$<existing-idp-ref>"
				oldObj.Spec.Redis.Password = "$<existing-redis-ref>"

				newObj := newValidZone()
				newObj.Spec.IdentityProviders[0].Admin.Password = secretsapi.KeywordRotate
				newObj.Spec.Redis.Password = secretsapi.KeywordRotate

				idpSecretPath := "zones/test-zone/admin/identityProviders/primary/password"
				redisSecretPath := "zones/test-zone/admin/redis/password"
				gatewaySecretPath := "zones/test-zone/admin/gateways/standard/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						idpSecretPath:     "idp-rotated-uuid",
						redisSecretPath:   "redis-rotated-uuid",
						gatewaySecretPath: "gw-rotated-uuid",
					}, nil)

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.IdentityProviders[0].Admin.Password).To(Equal("$<idp-rotated-uuid>"))
				Expect(newObj.Spec.Redis.Password).To(Equal("$<redis-rotated-uuid>"))
			})

			It("should upload user-provided plain secret on update", func() {
				oldObj := newValidZone()
				oldObj.Spec.IdentityProviders[0].Admin.Password = "$<existing-idp-ref>"
				oldObj.Spec.Redis.Password = "$<existing-redis-ref>"

				newObj := newValidZone()
				newObj.Spec.IdentityProviders[0].Admin.Password = "new-custom-password"
				newObj.Spec.Redis.Password = "new-custom-redis"

				idpSecretPath := "zones/test-zone/admin/identityProviders/primary/password"
				redisSecretPath := "zones/test-zone/admin/redis/password"
				gatewaySecretPath := "zones/test-zone/admin/gateways/standard/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						idpSecretPath:     "idp-new-uuid",
						redisSecretPath:   "redis-new-uuid",
						gatewaySecretPath: "gw-new-uuid",
					}, nil)

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.IdentityProviders[0].Admin.Password).To(Equal("$<idp-new-uuid>"))
				Expect(newObj.Spec.Redis.Password).To(Equal("$<redis-new-uuid>"))
			})
		})

		It("should skip onboarding when all secrets are already refs", func() {
			obj := newValidZone()
			obj.Spec.IdentityProviders[0].Admin.Password = "$<existing-idp-ref>"
			obj.Spec.Redis.Password = "$<existing-redis-ref>"
			obj.Spec.Gateways[0].Admin.ClientSecret = ptr("$<existing-gw-ref>")

			// No UpsertEnvironment call expected since all are already refs
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Spec.IdentityProviders[0].Admin.Password).To(Equal("$<existing-idp-ref>"))
			Expect(obj.Spec.Redis.Password).To(Equal("$<existing-redis-ref>"))
			Expect(*obj.Spec.Gateways[0].Admin.ClientSecret).To(Equal("$<existing-gw-ref>"))
		})

		It("should onboard only non-ref secrets", func() {
			obj := newValidZone()
			obj.Spec.IdentityProviders[0].Admin.Password = "$<existing-idp-ref>"
			obj.Spec.Redis.Password = "" // needs onboarding

			redisSecretPath := "zones/test-zone/admin/redis/password"
			gatewaySecretPath := "zones/test-zone/admin/gateways/standard/clientSecret"

			secretManagerMock.EXPECT().
				UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything).
				Return(map[string]string{
					redisSecretPath:   "new-redis-secret-uuid",
					gatewaySecretPath: "new-gw-secret-uuid",
				}, nil)

			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Spec.IdentityProviders[0].Admin.Password).To(Equal("$<existing-idp-ref>"))
			Expect(obj.Spec.Redis.Password).To(Equal("$<new-redis-secret-uuid>"))
		})

		It("should return an error when environment label is missing", func() {
			obj := newValidZone()
			obj.Labels = nil
			obj.Spec.IdentityProviders[0].Admin.Password = ""

			err := defaulter.Default(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("environment label is required"))
		})

		It("should return an error when secretManager is nil", func() {
			nilDefaulter := ZoneCustomDefaulter{secretManager: nil}
			obj := newValidZone()
			obj.Spec.IdentityProviders[0].Admin.Password = ""

			err := nilDefaulter.Default(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Secret-Manager is not configured"))
		})
	})
})
