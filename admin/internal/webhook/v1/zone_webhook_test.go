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
			IdentityProvider: adminv1.IdentityProviderConfig{
				Admin: adminv1.IdentityProviderAdminConfig{
					Url:      &identityAdminUrl,
					ClientId: "admin-client",
					UserName: "admin",
					Password: "",
				},
				Url: "https://idp.example.com",
			},
			Gateway: adminv1.GatewayConfig{
				Admin: adminv1.GatewayAdminConfig{
					Url: "https://gateway.example.com/admin",
				},
				Url: "https://gateway.example.com",
			},
			Redis: adminv1.RedisConfig{
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
			It("should generate IDP admin password when empty", func() {
				obj := newValidZone()
				obj.Spec.IdentityProvider.Admin.Password = ""

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.IdentityProvider.Admin.Password).NotTo(BeEmpty())
				Expect(obj.Spec.IdentityProvider.Admin.Password).To(HavePrefix("trd_"))
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
				obj.Spec.Gateway.Admin.ClientSecret = ptr("")

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Gateway.Admin.ClientSecret).NotTo(BeNil())
				Expect(*obj.Spec.Gateway.Admin.ClientSecret).NotTo(BeEmpty())
				Expect(*obj.Spec.Gateway.Admin.ClientSecret).To(HavePrefix("trd_"))
			})

			It("should not generate gateway client secret when nil", func() {
				obj := newValidZone()
				obj.Spec.Gateway.Admin.ClientSecret = nil

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Gateway.Admin.ClientSecret).To(BeNil())
			})

			It("should rotate IDP admin password when set to 'rotate'", func() {
				obj := newValidZone()
				obj.Spec.IdentityProvider.Admin.Password = secretsapi.KeywordRotate

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.IdentityProvider.Admin.Password).NotTo(Equal(secretsapi.KeywordRotate))
				Expect(obj.Spec.IdentityProvider.Admin.Password).NotTo(BeEmpty())
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
				obj.Spec.IdentityProvider.Admin.Password = "my-idp-password"
				obj.Spec.Redis.Password = "my-redis-password"
				obj.Spec.Gateway.Admin.ClientSecret = ptr("my-gw-secret")

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.IdentityProvider.Admin.Password).To(Equal("my-idp-password"))
				Expect(obj.Spec.Redis.Password).To(Equal("my-redis-password"))
				Expect(*obj.Spec.Gateway.Admin.ClientSecret).To(Equal("my-gw-secret"))
			})
		})

		Context("on UPDATE", func() {
			It("should preserve existing secrets when new value is empty", func() {
				oldObj := newValidZone()
				oldObj.Spec.IdentityProvider.Admin.Password = "old-idp-password"
				oldObj.Spec.Redis.Password = "old-redis-password"
				oldObj.Spec.Gateway.Admin.ClientSecret = ptr("old-gw-secret")

				newObj := newValidZone()
				newObj.Spec.IdentityProvider.Admin.Password = ""
				newObj.Spec.Redis.Password = ""
				newObj.Spec.Gateway.Admin.ClientSecret = nil

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.IdentityProvider.Admin.Password).To(Equal("old-idp-password"))
				Expect(newObj.Spec.Redis.Password).To(Equal("old-redis-password"))
				Expect(newObj.Spec.Gateway.Admin.ClientSecret).NotTo(BeNil())
				Expect(*newObj.Spec.Gateway.Admin.ClientSecret).To(Equal("old-gw-secret"))
			})

			It("should rotate secrets when set to 'rotate' even on update", func() {
				oldObj := newValidZone()
				oldObj.Spec.IdentityProvider.Admin.Password = "old-idp-password"
				oldObj.Spec.Redis.Password = "old-redis-password"

				newObj := newValidZone()
				newObj.Spec.IdentityProvider.Admin.Password = secretsapi.KeywordRotate
				newObj.Spec.Redis.Password = secretsapi.KeywordRotate

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.IdentityProvider.Admin.Password).NotTo(Equal(secretsapi.KeywordRotate))
				Expect(newObj.Spec.IdentityProvider.Admin.Password).NotTo(Equal("old-idp-password"))
				Expect(newObj.Spec.Redis.Password).NotTo(Equal(secretsapi.KeywordRotate))
				Expect(newObj.Spec.Redis.Password).NotTo(Equal("old-redis-password"))
			})

			It("should preserve user-provided non-empty secret on update", func() {
				oldObj := newValidZone()
				oldObj.Spec.IdentityProvider.Admin.Password = "old-idp-password"
				oldObj.Spec.Redis.Password = "old-redis-password"

				newObj := newValidZone()
				newObj.Spec.IdentityProvider.Admin.Password = "new-idp-password"
				newObj.Spec.Redis.Password = "new-redis-password"

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.IdentityProvider.Admin.Password).To(Equal("new-idp-password"))
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
			It("should onboard secrets and set secret refs when empty", func() {
				obj := newValidZone()
				obj.Spec.IdentityProvider.Admin.Password = ""
				obj.Spec.Redis.Password = ""

				idpSecretPath := "zones/test-zone/admin/identityProvider/password"
				redisSecretPath := "zones/test-zone/admin/redis/password"
				gatewaySecretPath := "zones/test-zone/admin/gateway/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						idpSecretPath:     "idp-secret-uuid",
						redisSecretPath:   "redis-secret-uuid",
						gatewaySecretPath: "gw-secret-uuid",
					}, nil)

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.IdentityProvider.Admin.Password).To(Equal("$<idp-secret-uuid>"))
				Expect(obj.Spec.Redis.Password).To(Equal("$<redis-secret-uuid>"))
				Expect(obj.Spec.Gateway.Admin.ClientSecret).NotTo(BeNil())
				Expect(*obj.Spec.Gateway.Admin.ClientSecret).To(Equal("$<gw-secret-uuid>"))
			})

			It("should onboard gateway secret when provided", func() {
				obj := newValidZone()
				obj.Spec.IdentityProvider.Admin.Password = "$<existing-idp-ref>"
				obj.Spec.Redis.Password = "$<existing-redis-ref>"
				obj.Spec.Gateway.Admin.ClientSecret = ptr("my-gw-secret")

				gatewaySecretPath := "zones/test-zone/admin/gateway/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything).
					Return(map[string]string{
						gatewaySecretPath: "gw-secret-uuid",
					}, nil)

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Gateway.Admin.ClientSecret).NotTo(BeNil())
				Expect(*obj.Spec.Gateway.Admin.ClientSecret).To(Equal("$<gw-secret-uuid>"))
			})

			It("should skip gateway secret when nil", func() {
				obj := newValidZone()
				obj.Spec.IdentityProvider.Admin.Password = "$<existing-idp-ref>"
				obj.Spec.Redis.Password = "$<existing-redis-ref>"
				obj.Spec.Gateway.Admin.ClientSecret = nil

				gatewaySecretPath := "zones/test-zone/admin/gateway/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything).
					Return(map[string]string{
						gatewaySecretPath: "gw-secret-uuid",
					}, nil)

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Gateway.Admin.ClientSecret).NotTo(BeNil())
				Expect(*obj.Spec.Gateway.Admin.ClientSecret).To(Equal("$<gw-secret-uuid>"))
			})

			It("should upload user-provided plain secrets to secret manager", func() {
				obj := newValidZone()
				obj.Spec.IdentityProvider.Admin.Password = "my-custom-idp-password"
				obj.Spec.Redis.Password = "my-custom-redis-password"

				idpSecretPath := "zones/test-zone/admin/identityProvider/password"
				redisSecretPath := "zones/test-zone/admin/redis/password"
				gatewaySecretPath := "zones/test-zone/admin/gateway/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						idpSecretPath:     "idp-custom-uuid",
						redisSecretPath:   "redis-custom-uuid",
						gatewaySecretPath: "gw-custom-uuid",
					}, nil)

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.IdentityProvider.Admin.Password).To(Equal("$<idp-custom-uuid>"))
				Expect(obj.Spec.Redis.Password).To(Equal("$<redis-custom-uuid>"))
			})

			It("should generate new secrets when set to 'rotate'", func() {
				obj := newValidZone()
				obj.Spec.IdentityProvider.Admin.Password = secretsapi.KeywordRotate
				obj.Spec.Redis.Password = secretsapi.KeywordRotate

				idpSecretPath := "zones/test-zone/admin/identityProvider/password"
				redisSecretPath := "zones/test-zone/admin/redis/password"
				gatewaySecretPath := "zones/test-zone/admin/gateway/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						idpSecretPath:     "idp-rotated-uuid",
						redisSecretPath:   "redis-rotated-uuid",
						gatewaySecretPath: "gw-rotated-uuid",
					}, nil)

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.IdentityProvider.Admin.Password).To(Equal("$<idp-rotated-uuid>"))
				Expect(obj.Spec.Redis.Password).To(Equal("$<redis-rotated-uuid>"))
			})
		})

		Context("on UPDATE", func() {
			It("should preserve existing secret ref when new value is empty", func() {
				oldObj := newValidZone()
				oldObj.Spec.IdentityProvider.Admin.Password = "$<existing-idp-ref>"
				oldObj.Spec.Redis.Password = "$<existing-redis-ref>"

				newObj := newValidZone()
				newObj.Spec.IdentityProvider.Admin.Password = ""
				newObj.Spec.Redis.Password = ""

				gatewaySecretPath := "zones/test-zone/admin/gateway/clientSecret"

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
				Expect(newObj.Spec.IdentityProvider.Admin.Password).To(Equal("$<existing-idp-ref>"))
				Expect(newObj.Spec.Redis.Password).To(Equal("$<existing-redis-ref>"))
			})

			It("should rotate secrets when set to 'rotate' even on update", func() {
				oldObj := newValidZone()
				oldObj.Spec.IdentityProvider.Admin.Password = "$<existing-idp-ref>"
				oldObj.Spec.Redis.Password = "$<existing-redis-ref>"

				newObj := newValidZone()
				newObj.Spec.IdentityProvider.Admin.Password = secretsapi.KeywordRotate
				newObj.Spec.Redis.Password = secretsapi.KeywordRotate

				idpSecretPath := "zones/test-zone/admin/identityProvider/password"
				redisSecretPath := "zones/test-zone/admin/redis/password"
				gatewaySecretPath := "zones/test-zone/admin/gateway/clientSecret"

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
				Expect(newObj.Spec.IdentityProvider.Admin.Password).To(Equal("$<idp-rotated-uuid>"))
				Expect(newObj.Spec.Redis.Password).To(Equal("$<redis-rotated-uuid>"))
			})

			It("should upload user-provided plain secret on update", func() {
				oldObj := newValidZone()
				oldObj.Spec.IdentityProvider.Admin.Password = "$<existing-idp-ref>"
				oldObj.Spec.Redis.Password = "$<existing-redis-ref>"

				newObj := newValidZone()
				newObj.Spec.IdentityProvider.Admin.Password = "new-custom-password"
				newObj.Spec.Redis.Password = "new-custom-redis"

				idpSecretPath := "zones/test-zone/admin/identityProvider/password"
				redisSecretPath := "zones/test-zone/admin/redis/password"
				gatewaySecretPath := "zones/test-zone/admin/gateway/clientSecret"

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
				Expect(newObj.Spec.IdentityProvider.Admin.Password).To(Equal("$<idp-new-uuid>"))
				Expect(newObj.Spec.Redis.Password).To(Equal("$<redis-new-uuid>"))
			})
		})

		It("should skip onboarding when all secrets are already refs", func() {
			obj := newValidZone()
			obj.Spec.IdentityProvider.Admin.Password = "$<existing-idp-ref>"
			obj.Spec.Redis.Password = "$<existing-redis-ref>"
			obj.Spec.Gateway.Admin.ClientSecret = ptr("$<existing-gw-ref>")

			// No UpsertEnvironment call expected since all are already refs
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Spec.IdentityProvider.Admin.Password).To(Equal("$<existing-idp-ref>"))
			Expect(obj.Spec.Redis.Password).To(Equal("$<existing-redis-ref>"))
			Expect(*obj.Spec.Gateway.Admin.ClientSecret).To(Equal("$<existing-gw-ref>"))
		})

		It("should onboard only non-ref secrets", func() {
			obj := newValidZone()
			obj.Spec.IdentityProvider.Admin.Password = "$<existing-idp-ref>"
			obj.Spec.Redis.Password = "" // needs onboarding

			redisSecretPath := "zones/test-zone/admin/redis/password"
			gatewaySecretPath := "zones/test-zone/admin/gateway/clientSecret"

			secretManagerMock.EXPECT().
				UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything).
				Return(map[string]string{
					redisSecretPath:   "new-redis-secret-uuid",
					gatewaySecretPath: "new-gw-secret-uuid",
				}, nil)

			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Spec.IdentityProvider.Admin.Password).To(Equal("$<existing-idp-ref>"))
			Expect(obj.Spec.Redis.Password).To(Equal("$<new-redis-secret-uuid>"))
		})

		It("should return an error when environment label is missing", func() {
			obj := newValidZone()
			obj.Labels = nil
			obj.Spec.IdentityProvider.Admin.Password = ""

			err := defaulter.Default(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("environment label is required"))
		})

		It("should return an error when secretManager is nil", func() {
			nilDefaulter := ZoneCustomDefaulter{secretManager: nil}
			obj := newValidZone()
			obj.Spec.IdentityProvider.Admin.Password = ""

			err := nilDefaulter.Default(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Secret-Manager is not configured"))
		})
	})
})
