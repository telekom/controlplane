// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	cerrors "github.com/telekom/controlplane/common/pkg/errors"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ = Describe("File Type (SFTP) Validation", func() {

	newValErr := func() *cerrors.ValidationError {
		return cerrors.NewValidationError(roverv1.GroupVersion.WithKind("Rover").GroupKind(), NewRover(testZone))
	}

	Context("validateFilePublicKeys", func() {
		filePath := field.NewPath("spec").Child("exposures").Index(0).Child("file")

		It("should require at least one public key", func() {
			valErr := newValErr()
			validateFilePublicKeys(valErr, nil, filePath)
			err := valErr.BuildError()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one public key must be specified"))
		})

		It("should accept unique labels and key values", func() {
			valErr := newValErr()
			keys := []roverv1.PublicKey{
				{Label: "provider-key", Key: "ssh-ed25519 AAAA1"},
				{Label: "consumer-key", Key: "ssh-ed25519 AAAA2"},
			}
			validateFilePublicKeys(valErr, keys, filePath)
			Expect(valErr.BuildError()).NotTo(HaveOccurred())
		})

		It("should reject duplicate public key labels per fileType", func() {
			valErr := newValErr()
			keys := []roverv1.PublicKey{
				{Label: "dup", Key: "ssh-ed25519 AAAA1"},
				{Label: "dup", Key: "ssh-ed25519 AAAA2"},
			}
			validateFilePublicKeys(valErr, keys, filePath)
			err := valErr.BuildError()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("labels must be unique per fileType"))
		})

		It("should reject duplicate public key values per fileType", func() {
			valErr := newValErr()
			keys := []roverv1.PublicKey{
				{Label: "key-a", Key: "ssh-ed25519 SAME"},
				{Label: "key-b", Key: "ssh-ed25519 SAME"},
			}
			validateFilePublicKeys(valErr, keys, filePath)
			err := valErr.BuildError()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("key values must be unique per fileType"))
		})
	})

	Context("MustNotHaveDuplicates for file types", func() {
		It("should reject two subscriptions to the same fileType", func() {
			valErr := newValErr()
			subs := []roverv1.Subscription{
				{File: &roverv1.FileSubscription{FileType: "demo-sftp-spec-v1"}},
				{File: &roverv1.FileSubscription{FileType: "demo-sftp-spec-v1"}},
			}
			Expect(MustNotHaveDuplicates(valErr, subs, nil)).To(Succeed())
			err := valErr.BuildError()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate subscription for file-type demo-sftp-spec-v1"))
		})

		It("should reject two exposures of the same fileType", func() {
			valErr := newValErr()
			exps := []roverv1.Exposure{
				{File: &roverv1.FileExposure{FileType: "demo-sftp-spec-v1"}},
				{File: &roverv1.FileExposure{FileType: "demo-sftp-spec-v1"}},
			}
			Expect(MustNotHaveDuplicates(valErr, nil, exps)).To(Succeed())
			err := valErr.BuildError()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate exposure for file-type demo-sftp-spec-v1"))
		})
	})

	Context("file type zone restriction (Rover webhook)", func() {
		var validator RoverValidator

		BeforeEach(func() {
			validator = RoverValidator{client: k8sClient}
		})

		fileExposure := func() roverv1.Exposure {
			return roverv1.Exposure{File: &roverv1.FileExposure{
				FileType:   "demo-sftp-spec-v1",
				PublicKeys: []roverv1.PublicKey{{Label: "provider-key", Key: "ssh-ed25519 AAAA"}},
			}}
		}

		It("should reject a file exposure on an unsupported zone", func() {
			// testZone is named "test" and is not in {cetus, canis}.
			rover := NewRover(testZone)
			rover.Spec.Exposures = []roverv1.Exposure{fileExposure()}
			warnings, err := validator.ValidateCreate(ctx, rover)
			assertValidationFailedWith(warnings, err, "does not support file types")
		})

		It("should reject a file subscription on an unsupported zone", func() {
			rover := NewRover(testZone)
			rover.Spec.Subscriptions = []roverv1.Subscription{{File: &roverv1.FileSubscription{
				FileType:   "demo-sftp-spec-v1",
				PublicKeys: []roverv1.PublicKey{{Label: "consumer-key", Key: "ssh-ed25519 BBBB"}},
			}}}
			warnings, err := validator.ValidateCreate(ctx, rover)
			assertValidationFailedWith(warnings, err, "does not support file types")
		})

		It("should accept a file exposure on a supported zone (cetus)", func() {
			cetus := NewZone("cetus", testZone.Namespace)
			CreateZone(ctx, cetus)
			rover := NewRover(cetus)
			rover.Spec.Exposures = []roverv1.Exposure{fileExposure()}
			warnings, err := validator.ValidateCreate(ctx, rover)
			Expect(warnings).To(BeNil())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("FileSpecificationCustomValidator", func() {
		var validator *FileSpecificationCustomValidator

		BeforeEach(func() {
			validator = &FileSpecificationCustomValidator{client: k8sClient}
		})

		newFileSpec := func(name string, storageType roverv1.FileStorageType) *roverv1.FileSpecification {
			return &roverv1.FileSpecification{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
				Spec:       roverv1.FileSpecificationSpec{Description: "demo", StorageType: storageType},
			}
		}

		It("should accept a FileSpecification with the sftp storageType", func() {
			warnings, err := validator.ValidateCreate(ctx, newFileSpec("demo-sftp-spec-v1", roverv1.FileStorageTypeSFTP))
			Expect(warnings).To(BeNil())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept a FileSpecification with an empty storageType (defaulted by CRD)", func() {
			warnings, err := validator.ValidateCreate(ctx, newFileSpec("demo-sftp-spec-v1", ""))
			Expect(warnings).To(BeNil())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject a FileSpecification with an unsupported storageType", func() {
			_, err := validator.ValidateCreate(ctx, newFileSpec("demo-sftp-spec-v1", "s3"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.storageType must be"))
		})
	})
})
