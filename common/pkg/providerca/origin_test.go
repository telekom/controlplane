// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package providerca

import (
	"crypto/sha256"
	"encoding/hex"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func validCopy(o *ProviderCAOrigin) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "env--zone", Name: o.CopyName(), Annotations: o.Annotations(),
			Labels: map[string]string{
				types.ProviderCALabelKey:     types.ProviderCALabelValue,
				config.EnvironmentLabelKey:   "env",
				config.BuildLabelKey("zone"): o.ZoneName,
			},
		},
		Data: map[string]string{ProviderCACopyKey: "pem"},
	}
}

var _ = Describe("ProviderCAOrigin", func() {
	origin := ProviderCAOrigin{ZoneUID: "uid-1", ZoneNamespace: "prod", ZoneName: "zone", ConfigMap: "src", Key: "ca.crt"}

	It("uses the length-prefixed canonical tuple and truncated hash name", func() {
		Expect(origin.Canonical()).To(Equal("5:uid-14:prod4:zone3:src6:ca.crt"))
		sum := sha256.Sum256([]byte(origin.Canonical()))
		Expect(origin.Hash()).To(Equal(hex.EncodeToString(sum[:])))
		Expect(origin.CopyName()).To(Equal("provider-ca-" + origin.Hash()[:40]))
		other := ProviderCAOrigin{ZoneUID: "uid-1", ZoneNamespace: "prod", ZoneName: "zon", ConfigMap: "esrc", Key: "ca.crt"}
		Expect(other.Hash()).ToNot(Equal(origin.Hash()))
	})

	It("round-trips complete annotations and rejects tampered ones", func() {
		cm := validCopy(&origin)
		got, ok := ProviderCAOriginFromObject(cm)
		Expect(ok).To(BeTrue())
		Expect(got).To(Equal(origin))

		cm.Annotations[AnnotationProviderCAOriginHash] = "x"
		_, ok = ProviderCAOriginFromObject(cm)
		Expect(ok).To(BeFalse())
	})

	Describe("Zone bundle origin", func() {
		zoneOrigin := ProviderCAOrigin{ZoneUID: "uid-1", ZoneNamespace: "prod", ZoneName: "zone"}

		It("claims no single source and has a name distinct from single-source copies", func() {
			cm := validCopy(&zoneOrigin)
			Expect(cm.Annotations).NotTo(HaveKey(AnnotationProviderCAOriginConfigMap))
			Expect(cm.Annotations).NotTo(HaveKey(AnnotationProviderCAOriginKey))
			got, err := ValidateProviderCACopyMetadata(cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(zoneOrigin))
			Expect(zoneOrigin.CopyName()).NotTo(Equal(origin.CopyName()))
			other := ProviderCAOrigin{ZoneUID: "uid-2", ZoneNamespace: "prod", ZoneName: "zone"}
			Expect(other.CopyName()).NotTo(Equal(zoneOrigin.CopyName()))
		})

		DescribeTable("rejects a half-present source tuple",
			func(key string) {
				cm := validCopy(&zoneOrigin)
				cm.Annotations[key] = "x"
				_, ok := ProviderCAOriginFromObject(cm)
				Expect(ok).To(BeFalse())
			},
			Entry("configmap only", AnnotationProviderCAOriginConfigMap),
			Entry("key only", AnnotationProviderCAOriginKey),
		)

		It("rejects empty source annotations on a single-source copy", func() {
			cm := validCopy(&origin)
			cm.Annotations[AnnotationProviderCAOriginKey] = ""
			_, ok := ProviderCAOriginFromObject(cm)
			Expect(ok).To(BeFalse())
		})
	})

	Describe("IsProviderCACopy", func() {
		It("detects the copy naming convention and any origin annotation", func() {
			Expect(IsProviderCACopy(validCopy(&origin))).To(BeTrue())
			Expect(IsProviderCACopy(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: origin.CopyName()}})).To(BeTrue())
			Expect(IsProviderCACopy(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name: "my-ca", Annotations: map[string]string{AnnotationZoneUID: "x"},
			}})).To(BeTrue())
			Expect(IsProviderCACopy(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "provider-ca-private"}})).To(BeFalse())
			Expect(IsProviderCACopy(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-ca"}})).To(BeFalse())
		})
	})

	Describe("ValidateProviderCAConfigMap", func() {
		ref := &types.CABundleConfigMapRef{Name: origin.CopyName(), Key: ProviderCACopyKey}

		It("accepts ordinary labeled local sources without provenance", func() {
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name: "private-ca", Labels: map[string]string{types.ProviderCALabelKey: types.ProviderCALabelValue},
			}}
			Expect(ValidateProviderCAConfigMap(cm, &types.CABundleConfigMapRef{Name: "private-ca", Key: "bundle.pem"})).To(Succeed())
		})

		It("requires the exact label on every reference", func() {
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "private-ca", Labels: map[string]string{types.ProviderCALabelKey: "True"}}}
			Expect(ValidateProviderCAConfigMap(cm, ref)).To(MatchError(ContainSubstring("lacks label")))
		})

		It("accepts a copy with complete provenance", func() {
			Expect(ValidateProviderCAConfigMap(validCopy(&origin), ref)).To(Succeed())
		})

		DescribeTable("rejects partial or spoofed copy metadata",
			func(mutate func(cm *corev1.ConfigMap), msg string) {
				cm := validCopy(&origin)
				mutate(cm)
				Expect(ValidateProviderCAConfigMap(cm, ref)).To(MatchError(ContainSubstring(msg)))
			},
			Entry("missing origin annotation", func(cm *corev1.ConfigMap) {
				delete(cm.Annotations, AnnotationProviderCAOriginKey)
			}, "origin metadata"),
			Entry("copy name without any provenance", func(cm *corev1.ConfigMap) {
				cm.Annotations = nil
			}, "origin metadata"),
			Entry("forged zone uid", func(cm *corev1.ConfigMap) {
				cm.Annotations[AnnotationZoneUID] = "other"
			}, "origin metadata"),
			Entry("hash not matching the name", func(cm *corev1.ConfigMap) {
				cm.Name = "provider-ca-" + "0000000000000000000000000000000000000000"
			}, "origin metadata"),
			Entry("ordinary name with partial provenance", func(cm *corev1.ConfigMap) {
				cm.Name = "my-ca"
			}, "origin metadata"),
			Entry("owner references", func(cm *corev1.ConfigMap) {
				cm.OwnerReferences = []metav1.OwnerReference{{Name: "x"}}
			}, "ownerReferences"),
			Entry("zone label mismatch", func(cm *corev1.ConfigMap) {
				cm.Labels[config.BuildLabelKey("zone")] = "other"
			}, "environment/zone labels"),
			Entry("environment label missing", func(cm *corev1.ConfigMap) {
				delete(cm.Labels, config.EnvironmentLabelKey)
			}, "environment/zone labels"),
		)

		It("requires the fixed key on copies", func() {
			Expect(ValidateProviderCAConfigMap(validCopy(&origin), &types.CABundleConfigMapRef{Name: origin.CopyName(), Key: "other"})).
				To(MatchError(ContainSubstring("key must be")))
		})

		It("requires a reference matching the copy name", func() {
			Expect(ValidateProviderCAConfigMap(validCopy(&origin), nil)).To(MatchError(ContainSubstring("reference is required")))
			Expect(ValidateProviderCAConfigMap(validCopy(&origin), &types.CABundleConfigMapRef{Name: "x", Key: ProviderCACopyKey})).
				To(MatchError(ContainSubstring("does not match")))
		})

		It("validates copy metadata including the label", func() {
			cm := validCopy(&origin)
			delete(cm.Labels, types.ProviderCALabelKey)
			_, err := ValidateProviderCACopyMetadata(cm)
			Expect(err).To(MatchError(ContainSubstring("lacks label")))
		})
	})
})
