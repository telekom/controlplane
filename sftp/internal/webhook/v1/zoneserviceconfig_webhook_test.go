// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/config"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newZoneServiceConfig() *sftpv1.ZoneServiceConfig {
	return &sftpv1.ZoneServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dataplane1",
			Namespace: "default",
			Labels: map[string]string{
				config.EnvironmentLabelKey: "poc",
			},
		},
		Spec: sftpv1.ZoneServiceConfigSpec{
			API: sftpv1.APIEndpoint{
				Endpoint:     "https://sftp.example.com/api",
				Issuer:       "https://issuer.example.com/token",
				ClientID:     "sftp-client",
				ClientSecret: "plain-secret",
			},
		},
	}
}

var _ = Describe("ZoneServiceConfig Webhook", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("validation", func() {
		var validator *ZoneServiceConfigCustomValidator

		BeforeEach(func() {
			validator = &ZoneServiceConfigCustomValidator{}
		})

		It("allows a valid ZoneServiceConfig on create", func() {
			warnings, err := validator.ValidateCreate(ctx, newZoneServiceConfig())

			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("allows a valid ZoneServiceConfig on update", func() {
			oldZSC := newZoneServiceConfig()
			newZSC := newZoneServiceConfig()
			newZSC.Spec.API.Endpoint = "https://sftp-new.example.com/api"

			warnings, err := validator.ValidateUpdate(ctx, oldZSC, newZSC)

			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("rejects missing environment label", func() {
			zsc := newZoneServiceConfig()
			zsc.Labels = nil

			warnings, err := validator.ValidateCreate(ctx, zsc)

			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("must contain an environment label"))
			Expect(warnings).To(BeNil())
		})

		It("rejects missing required API fields", func() {
			zsc := newZoneServiceConfig()
			zsc.Spec.API.Endpoint = ""
			zsc.Spec.API.Issuer = ""
			zsc.Spec.API.ClientID = ""
			zsc.Spec.API.ClientSecret = ""

			warnings, err := validator.ValidateCreate(ctx, zsc)

			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("spec.api.endpoint"))
			Expect(err.Error()).To(ContainSubstring("spec.api.issuer"))
			Expect(err.Error()).To(ContainSubstring("spec.api.clientID"))
			Expect(err.Error()).To(ContainSubstring("spec.api.clientSecret"))
			Expect(warnings).To(BeNil())
		})

		It("rejects malformed endpoint and issuer URLs", func() {
			zsc := newZoneServiceConfig()
			zsc.Spec.API.Endpoint = "not-a-url"
			zsc.Spec.API.Issuer = "://bad-url"

			warnings, err := validator.ValidateCreate(ctx, zsc)

			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("must be a valid SFTP Tardis API base URL"))
			Expect(err.Error()).To(ContainSubstring("must be a valid OAuth2 token endpoint URL"))
			Expect(warnings).To(BeNil())
		})

		It("allows deletion without validating the spec", func() {
			zsc := newZoneServiceConfig()
			zsc.Labels = nil
			zsc.Spec.API = sftpv1.APIEndpoint{}

			warnings, err := validator.ValidateDelete(ctx, zsc)

			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})
	})
})
