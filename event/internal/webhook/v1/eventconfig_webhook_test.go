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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
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

// newValidEventConfig creates an EventConfig with all required fields populated.
func newValidEventConfig() *eventv1.EventConfig {
	return &eventv1.EventConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-eventconfig",
			Namespace: "default",
			Labels: map[string]string{
				config.EnvironmentLabelKey: "test-env",
			},
		},
		Spec: eventv1.EventConfigSpec{
			Zone: ctypes.ObjectRef{
				Name:      "test-zone",
				Namespace: "default",
			},
			Admin: eventv1.AdminConfig{
				Url: "https://admin.example.com",
				Client: eventv1.ClientConfig{
					Realm: ctypes.ObjectRef{
						Name:      "admin-realm",
						Namespace: "default",
					},
				},
			},
			Mesh: eventv1.MeshConfig{
				FullMesh: true,
				Client: eventv1.ClientConfig{
					Realm: ctypes.ObjectRef{
						Name:      "mesh-realm",
						Namespace: "default",
					},
				},
			},
			ServerSendEventUrl: "https://sse.example.com",
			PublishEventUrl:    "https://publish.example.com",
		},
	}
}

// updateContextWithOldObject returns a context that simulates an UPDATE admission request
// with the given old EventConfig as the previous version of the resource.
func updateContextWithOldObject(parent context.Context, oldObj *eventv1.EventConfig) context.Context {
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

var _ = Describe("EventConfig Webhook", func() {
	// ─────────────────────────────────────────────────────────────────────────
	// Validation Tests
	// ─────────────────────────────────────────────────────────────────────────

	Context("When validating EventConfig", func() {
		var validator EventConfigCustomValidator

		BeforeEach(func() {
			validator = EventConfigCustomValidator{}
		})

		Context("ValidateCreate", func() {
			It("should accept a valid EventConfig with both realms set", func() {
				obj := newValidEventConfig()
				warnings, err := validator.ValidateCreate(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(warnings).To(BeNil())
			})

			It("should reject when admin realm is empty", func() {
				obj := newValidEventConfig()
				obj.Spec.Admin.Client.Realm = ctypes.ObjectRef{}

				warnings, err := validator.ValidateCreate(ctx, obj)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsInvalid(err)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("realm must be specified for admin client"))
				Expect(warnings).To(BeNil())
			})

			It("should reject when mesh realm is empty", func() {
				obj := newValidEventConfig()
				obj.Spec.Mesh.Client.Realm = ctypes.ObjectRef{}

				warnings, err := validator.ValidateCreate(ctx, obj)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsInvalid(err)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("realm must be specified for mesh client"))
				Expect(warnings).To(BeNil())
			})

			It("should reject when both realms are empty and report both errors", func() {
				obj := newValidEventConfig()
				obj.Spec.Admin.Client.Realm = ctypes.ObjectRef{}
				obj.Spec.Mesh.Client.Realm = ctypes.ObjectRef{}

				warnings, err := validator.ValidateCreate(ctx, obj)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsInvalid(err)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("realm must be specified for admin client"))
				Expect(err.Error()).To(ContainSubstring("realm must be specified for mesh client"))
				Expect(warnings).To(BeNil())
			})

			It("should accept when realm has only Name set", func() {
				obj := newValidEventConfig()
				obj.Spec.Admin.Client.Realm = ctypes.ObjectRef{Name: "admin-realm"}
				obj.Spec.Mesh.Client.Realm = ctypes.ObjectRef{Name: "mesh-realm"}

				warnings, err := validator.ValidateCreate(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(warnings).To(BeNil())
			})

			It("should accept when realm has only Namespace set", func() {
				obj := newValidEventConfig()
				obj.Spec.Admin.Client.Realm = ctypes.ObjectRef{Namespace: "default"}
				obj.Spec.Mesh.Client.Realm = ctypes.ObjectRef{Namespace: "default"}

				warnings, err := validator.ValidateCreate(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(warnings).To(BeNil())
			})
		})

		Context("ValidateUpdate", func() {
			It("should apply the same validation as create", func() {
				oldObj := newValidEventConfig()
				newObj := newValidEventConfig()
				newObj.Spec.Admin.Client.Realm = ctypes.ObjectRef{}

				warnings, err := validator.ValidateUpdate(ctx, oldObj, newObj)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsInvalid(err)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("realm must be specified for admin client"))
				Expect(warnings).To(BeNil())
			})

			It("should accept a valid update", func() {
				oldObj := newValidEventConfig()
				newObj := newValidEventConfig()

				warnings, err := validator.ValidateUpdate(ctx, oldObj, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(warnings).To(BeNil())
			})
		})

		Context("ValidateDelete", func() {
			It("should always allow deletion", func() {
				obj := newValidEventConfig()
				warnings, err := validator.ValidateDelete(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(warnings).To(BeNil())
			})

			It("should allow deletion even with invalid config", func() {
				obj := &eventv1.EventConfig{}
				warnings, err := validator.ValidateDelete(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(warnings).To(BeNil())
			})
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Defaulting Tests — Secret Manager Disabled
	// ─────────────────────────────────────────────────────────────────────────

	Context("When defaulting EventConfig with Secret Manager disabled", func() {
		var (
			defaulter EventConfigCustomDefaulter
			cleanup   func()
		)

		BeforeEach(func() {
			cleanup = disableSecretManager()
			defaulter = EventConfigCustomDefaulter{secretManager: nil}
		})

		AfterEach(func() {
			cleanup()
		})

		It("should set default admin ClientId when empty", func() {
			obj := newValidEventConfig()
			obj.Spec.Admin.Client.ClientId = ""

			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Spec.Admin.Client.ClientId).To(Equal(util.AdminClientName))
		})

		It("should set default mesh ClientId when empty", func() {
			obj := newValidEventConfig()
			obj.Spec.Mesh.Client.ClientId = ""

			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Spec.Mesh.Client.ClientId).To(Equal(util.MeshClientName))
		})

		It("should preserve existing admin ClientId", func() {
			obj := newValidEventConfig()
			obj.Spec.Admin.Client.ClientId = "custom-admin-client"

			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Spec.Admin.Client.ClientId).To(Equal("custom-admin-client"))
		})

		It("should preserve existing mesh ClientId", func() {
			obj := newValidEventConfig()
			obj.Spec.Mesh.Client.ClientId = "custom-mesh-client"

			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Spec.Mesh.Client.ClientId).To(Equal("custom-mesh-client"))
		})

		Context("on CREATE", func() {
			It("should generate admin ClientSecret when empty", func() {
				obj := newValidEventConfig()
				obj.Spec.Admin.Client.ClientSecret = ""
				obj.Spec.Mesh.Client.ClientSecret = "existing-secret"

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Admin.Client.ClientSecret).NotTo(BeEmpty())
			})

			It("should generate mesh ClientSecret when empty", func() {
				obj := newValidEventConfig()
				obj.Spec.Admin.Client.ClientSecret = "existing-secret"
				obj.Spec.Mesh.Client.ClientSecret = ""

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Mesh.Client.ClientSecret).NotTo(BeEmpty())
			})

			It("should rotate admin ClientSecret when set to 'rotate'", func() {
				obj := newValidEventConfig()
				obj.Spec.Admin.Client.ClientSecret = secretsapi.KeywordRotate
				obj.Spec.Mesh.Client.ClientSecret = "existing-secret"

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Admin.Client.ClientSecret).NotTo(Equal(secretsapi.KeywordRotate))
				Expect(obj.Spec.Admin.Client.ClientSecret).NotTo(BeEmpty())
			})

			It("should rotate mesh ClientSecret when set to 'rotate'", func() {
				obj := newValidEventConfig()
				obj.Spec.Admin.Client.ClientSecret = "existing-secret"
				obj.Spec.Mesh.Client.ClientSecret = secretsapi.KeywordRotate

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Mesh.Client.ClientSecret).NotTo(Equal(secretsapi.KeywordRotate))
				Expect(obj.Spec.Mesh.Client.ClientSecret).NotTo(BeEmpty())
			})

			It("should preserve existing non-empty ClientSecrets", func() {
				obj := newValidEventConfig()
				obj.Spec.Admin.Client.ClientSecret = "admin-secret-123"
				obj.Spec.Mesh.Client.ClientSecret = "mesh-secret-456"

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Admin.Client.ClientSecret).To(Equal("admin-secret-123"))
				Expect(obj.Spec.Mesh.Client.ClientSecret).To(Equal("mesh-secret-456"))
			})
		})

		Context("on UPDATE", func() {
			It("should preserve existing secret when new value is empty", func() {
				oldObj := newValidEventConfig()
				oldObj.Spec.Admin.Client.ClientSecret = "old-admin-secret"
				oldObj.Spec.Mesh.Client.ClientSecret = "old-mesh-secret"

				newObj := newValidEventConfig()
				newObj.Spec.Admin.Client.ClientSecret = ""
				newObj.Spec.Mesh.Client.ClientSecret = ""

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.Admin.Client.ClientSecret).To(Equal("old-admin-secret"))
				Expect(newObj.Spec.Mesh.Client.ClientSecret).To(Equal("old-mesh-secret"))
			})

			It("should rotate secret when set to 'rotate' even on update", func() {
				oldObj := newValidEventConfig()
				oldObj.Spec.Admin.Client.ClientSecret = "old-admin-secret"
				oldObj.Spec.Mesh.Client.ClientSecret = "old-mesh-secret"

				newObj := newValidEventConfig()
				newObj.Spec.Admin.Client.ClientSecret = secretsapi.KeywordRotate
				newObj.Spec.Mesh.Client.ClientSecret = secretsapi.KeywordRotate

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.Admin.Client.ClientSecret).NotTo(Equal(secretsapi.KeywordRotate))
				Expect(newObj.Spec.Admin.Client.ClientSecret).NotTo(Equal("old-admin-secret"))
				Expect(newObj.Spec.Admin.Client.ClientSecret).NotTo(BeEmpty())
				Expect(newObj.Spec.Mesh.Client.ClientSecret).NotTo(Equal(secretsapi.KeywordRotate))
				Expect(newObj.Spec.Mesh.Client.ClientSecret).NotTo(Equal("old-mesh-secret"))
				Expect(newObj.Spec.Mesh.Client.ClientSecret).NotTo(BeEmpty())
			})

			It("should preserve user-provided non-empty secret on update", func() {
				oldObj := newValidEventConfig()
				oldObj.Spec.Admin.Client.ClientSecret = "old-admin-secret"
				oldObj.Spec.Mesh.Client.ClientSecret = "old-mesh-secret"

				newObj := newValidEventConfig()
				newObj.Spec.Admin.Client.ClientSecret = "new-admin-secret"
				newObj.Spec.Mesh.Client.ClientSecret = "new-mesh-secret"

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.Admin.Client.ClientSecret).To(Equal("new-admin-secret"))
				Expect(newObj.Spec.Mesh.Client.ClientSecret).To(Equal("new-mesh-secret"))
			})
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Defaulting Tests — Secret Manager Enabled
	// ─────────────────────────────────────────────────────────────────────────

	Context("When defaulting EventConfig with Secret Manager enabled", func() {
		var (
			defaulter         EventConfigCustomDefaulter
			secretManagerMock *fake.MockSecretManager
			cleanup           func()
		)

		BeforeEach(func() {
			cleanup = enableSecretManager()
			secretManagerMock = fake.NewMockSecretManager(GinkgoT())
			defaulter = EventConfigCustomDefaulter{secretManager: secretManagerMock}
		})

		AfterEach(func() {
			cleanup()
		})

		Context("on CREATE", func() {
			It("should onboard secrets and set secret refs for both clients when empty", func() {
				obj := newValidEventConfig()
				obj.Spec.Admin.Client.ClientSecret = ""
				obj.Spec.Mesh.Client.ClientSecret = ""

				adminSecretId := "zones/test-zone/event/admin/clientSecret"
				meshSecretId := "zones/test-zone/event/mesh/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						adminSecretId: "admin-secret-uuid",
						meshSecretId:  "mesh-secret-uuid",
					}, nil)

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Admin.Client.ClientSecret).To(Equal("$<admin-secret-uuid>"))
				Expect(obj.Spec.Mesh.Client.ClientSecret).To(Equal("$<mesh-secret-uuid>"))
			})

			It("should upload user-provided plain secret to secret manager", func() {
				obj := newValidEventConfig()
				obj.Spec.Admin.Client.ClientSecret = "my-custom-admin-secret"
				obj.Spec.Mesh.Client.ClientSecret = "my-custom-mesh-secret"

				adminSecretId := "zones/test-zone/event/admin/clientSecret"
				meshSecretId := "zones/test-zone/event/mesh/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env",
						mock.Anything, // WithMergeStrategy
						mock.MatchedBy(func(opt secretsapi.OnboardingOption) bool { return true }), // admin secret
						mock.MatchedBy(func(opt secretsapi.OnboardingOption) bool { return true }), // mesh secret
					).
					Return(map[string]string{
						adminSecretId: "admin-custom-uuid",
						meshSecretId:  "mesh-custom-uuid",
					}, nil)

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Admin.Client.ClientSecret).To(Equal("$<admin-custom-uuid>"))
				Expect(obj.Spec.Mesh.Client.ClientSecret).To(Equal("$<mesh-custom-uuid>"))
			})

			It("should generate new secret when set to 'rotate'", func() {
				obj := newValidEventConfig()
				obj.Spec.Admin.Client.ClientSecret = secretsapi.KeywordRotate
				obj.Spec.Mesh.Client.ClientSecret = secretsapi.KeywordRotate

				adminSecretId := "zones/test-zone/event/admin/clientSecret"
				meshSecretId := "zones/test-zone/event/mesh/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						adminSecretId: "admin-rotated-uuid",
						meshSecretId:  "mesh-rotated-uuid",
					}, nil)

				err := defaulter.Default(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Spec.Admin.Client.ClientSecret).To(Equal("$<admin-rotated-uuid>"))
				Expect(obj.Spec.Mesh.Client.ClientSecret).To(Equal("$<mesh-rotated-uuid>"))
			})
		})

		Context("on UPDATE", func() {
			It("should preserve existing secret ref when new value is empty", func() {
				oldObj := newValidEventConfig()
				oldObj.Spec.Admin.Client.ClientSecret = "$<existing-admin-ref>"
				oldObj.Spec.Mesh.Client.ClientSecret = "$<existing-mesh-ref>"

				newObj := newValidEventConfig()
				newObj.Spec.Admin.Client.ClientSecret = ""
				newObj.Spec.Mesh.Client.ClientSecret = ""

				// After resolving, both secrets become the old refs.
				// Since both are already refs, no UpsertEnvironment call is expected.
				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.Admin.Client.ClientSecret).To(Equal("$<existing-admin-ref>"))
				Expect(newObj.Spec.Mesh.Client.ClientSecret).To(Equal("$<existing-mesh-ref>"))
			})

			It("should rotate secret when set to 'rotate' even on update", func() {
				oldObj := newValidEventConfig()
				oldObj.Spec.Admin.Client.ClientSecret = "$<existing-admin-ref>"
				oldObj.Spec.Mesh.Client.ClientSecret = "$<existing-mesh-ref>"

				newObj := newValidEventConfig()
				newObj.Spec.Admin.Client.ClientSecret = secretsapi.KeywordRotate
				newObj.Spec.Mesh.Client.ClientSecret = secretsapi.KeywordRotate

				adminSecretId := "zones/test-zone/event/admin/clientSecret"
				meshSecretId := "zones/test-zone/event/mesh/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						adminSecretId: "admin-rotated-uuid",
						meshSecretId:  "mesh-rotated-uuid",
					}, nil)

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.Admin.Client.ClientSecret).To(Equal("$<admin-rotated-uuid>"))
				Expect(newObj.Spec.Mesh.Client.ClientSecret).To(Equal("$<mesh-rotated-uuid>"))
			})

			It("should upload user-provided plain secret on update", func() {
				oldObj := newValidEventConfig()
				oldObj.Spec.Admin.Client.ClientSecret = "$<existing-admin-ref>"
				oldObj.Spec.Mesh.Client.ClientSecret = "$<existing-mesh-ref>"

				newObj := newValidEventConfig()
				newObj.Spec.Admin.Client.ClientSecret = "new-custom-secret"
				newObj.Spec.Mesh.Client.ClientSecret = "new-custom-mesh-secret"

				adminSecretId := "zones/test-zone/event/admin/clientSecret"
				meshSecretId := "zones/test-zone/event/mesh/clientSecret"

				secretManagerMock.EXPECT().
					UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything, mock.Anything).
					Return(map[string]string{
						adminSecretId: "admin-new-uuid",
						meshSecretId:  "mesh-new-uuid",
					}, nil)

				updateCtx := updateContextWithOldObject(ctx, oldObj)
				err := defaulter.Default(updateCtx, newObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(newObj.Spec.Admin.Client.ClientSecret).To(Equal("$<admin-new-uuid>"))
				Expect(newObj.Spec.Mesh.Client.ClientSecret).To(Equal("$<mesh-new-uuid>"))
			})
		})

		It("should skip onboarding when secrets are already refs", func() {
			obj := newValidEventConfig()
			obj.Spec.Admin.Client.ClientSecret = "$<existing-admin-ref>"
			obj.Spec.Mesh.Client.ClientSecret = "$<existing-mesh-ref>"

			// No UpsertEnvironment call expected since both are already refs
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Spec.Admin.Client.ClientSecret).To(Equal("$<existing-admin-ref>"))
			Expect(obj.Spec.Mesh.Client.ClientSecret).To(Equal("$<existing-mesh-ref>"))
		})

		It("should onboard only non-ref secrets", func() {
			obj := newValidEventConfig()
			obj.Spec.Admin.Client.ClientSecret = "$<existing-admin-ref>"
			obj.Spec.Mesh.Client.ClientSecret = "" // needs onboarding

			meshSecretId := "zones/test-zone/event/mesh/clientSecret"

			secretManagerMock.EXPECT().
				UpsertEnvironment(mock.Anything, "test-env", mock.Anything, mock.Anything).
				Return(map[string]string{
					meshSecretId: "new-mesh-secret-uuid",
				}, nil)

			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Spec.Admin.Client.ClientSecret).To(Equal("$<existing-admin-ref>"))
			Expect(obj.Spec.Mesh.Client.ClientSecret).To(Equal("$<new-mesh-secret-uuid>"))
		})

		It("should return an error when environment label is missing", func() {
			obj := newValidEventConfig()
			obj.Labels = nil
			obj.Spec.Admin.Client.ClientSecret = ""

			err := defaulter.Default(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("environment label is required"))
		})

		It("should return an error when secretManager is nil", func() {
			nilDefaulter := EventConfigCustomDefaulter{secretManager: nil}
			obj := newValidEventConfig()
			obj.Spec.Admin.Client.ClientSecret = ""

			err := nilDefaulter.Default(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Secret-Manager is not configured"))
		})

		It("should set default ClientIds before onboarding secrets", func() {
			obj := newValidEventConfig()
			obj.Spec.Admin.Client.ClientId = ""
			obj.Spec.Mesh.Client.ClientId = ""
			obj.Spec.Admin.Client.ClientSecret = "$<existing-ref>"
			obj.Spec.Mesh.Client.ClientSecret = "$<existing-ref>"

			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Spec.Admin.Client.ClientId).To(Equal(util.AdminClientName))
			Expect(obj.Spec.Mesh.Client.ClientId).To(Equal(util.MeshClientName))
		})
	})
})
