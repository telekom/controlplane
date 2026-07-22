// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
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
				{Label: "provider-key", Key: newED25519Key()},
				{Label: "consumer-key", Key: newED25519Key()},
			}
			validateFilePublicKeys(valErr, keys, filePath)
			Expect(valErr.BuildError()).NotTo(HaveOccurred())
		})

		It("should reject duplicate public key labels per fileType", func() {
			valErr := newValErr()
			keys := []roverv1.PublicKey{
				{Label: "dup", Key: newED25519Key()},
				{Label: "dup", Key: newED25519Key()},
			}
			validateFilePublicKeys(valErr, keys, filePath)
			err := valErr.BuildError()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("labels must be unique per fileType"))
		})

		It("should reject duplicate public key values per fileType", func() {
			valErr := newValErr()
			sameKey := newED25519Key()
			keys := []roverv1.PublicKey{
				{Label: "key-a", Key: sameKey},
				{Label: "key-b", Key: sameKey},
			}
			validateFilePublicKeys(valErr, keys, filePath)
			err := valErr.BuildError()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("key values must be unique per fileType"))
		})

		It("should accept all supported SSH key types", func() {
			valErr := newValErr()
			keys := []roverv1.PublicKey{
				{Label: "rsa-key", Key: newRSAKey()},
				{Label: "ed25519-key", Key: newED25519Key()},
				{Label: "ecdsa-key", Key: newECDSAKey(elliptic.P521())},
			}
			validateFilePublicKeys(valErr, keys, filePath)
			Expect(valErr.BuildError()).NotTo(HaveOccurred())
		})

		It("should reject a malformed key that cannot be parsed", func() {
			valErr := newValErr()
			keys := []roverv1.PublicKey{
				{Label: "bad-key", Key: "ssh-ed25519 not-valid-base64!!"},
			}
			validateFilePublicKeys(valErr, keys, filePath)
			err := valErr.BuildError()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid SSH public key for label 'bad-key'"))
		})

		It("should reject a well-formed key of an unsupported type", func() {
			valErr := newValErr()
			keys := []roverv1.PublicKey{
				// A valid ECDSA P-256 key parses fine but is not in the allowlist
				// (only ecdsa-sha2-nistp521 is supported).
				{Label: "bad-type", Key: newECDSAKey(elliptic.P256())},
			}
			validateFilePublicKeys(valErr, keys, filePath)
			err := valErr.BuildError()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported key type 'ecdsa-sha2-nistp256' for key labelled 'bad-type'"))
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
				PublicKeys: []roverv1.PublicKey{{Label: "provider-key", Key: newED25519Key()}},
			}}
		}

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

// mustAuthorizedKey marshals a crypto public key into an SSH authorized-keys line.
func mustAuthorizedKey(pub crypto.PublicKey) string {
	sshPub, err := ssh.NewPublicKey(pub)
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
}

// newED25519Key generates a fresh, valid ssh-ed25519 authorized-keys entry.
func newED25519Key() string {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	return mustAuthorizedKey(pub)
}

// newRSAKey generates a fresh, valid ssh-rsa authorized-keys entry.
func newRSAKey() string {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())
	return mustAuthorizedKey(priv.Public())
}

// newECDSAKey generates a fresh, valid ecdsa-sha2-* authorized-keys entry for the
// given curve (e.g. elliptic.P521() -> ecdsa-sha2-nistp521).
func newECDSAKey(curve elliptic.Curve) string {
	priv, err := ecdsa.GenerateKey(curve, rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	return mustAuthorizedKey(priv.Public())
}
