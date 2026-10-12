// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package providerca holds the shared provenance contract of Admin-managed provider CA copies
// and the runtime metadata validation applied by every consumer of provider CA references.
package providerca

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
)

const (
	providerCAAnnotationPrefix = "admin.cp.ei.telekom.de/"

	// AnnotationZoneUID records the UID of the Zone that generated a child or durable CA copy.
	AnnotationZoneUID = providerCAAnnotationPrefix + "zone-uid"
	// AnnotationProviderCAOriginZoneNamespace is the namespace of the originating Zone (= source namespace).
	AnnotationProviderCAOriginZoneNamespace = providerCAAnnotationPrefix + "provider-ca-origin-zone-namespace"
	// AnnotationProviderCAOriginZoneName is the name of the originating Zone.
	AnnotationProviderCAOriginZoneName = providerCAAnnotationPrefix + "provider-ca-origin-zone-name"
	// AnnotationProviderCAOriginConfigMap is the name of the source ConfigMap of a single-source copy.
	// Zone bundles combine several sources and carry neither this nor the key annotation.
	AnnotationProviderCAOriginConfigMap = providerCAAnnotationPrefix + "provider-ca-origin-configmap"
	// AnnotationProviderCAOriginKey is the data key of the source ConfigMap of a single-source copy.
	AnnotationProviderCAOriginKey = providerCAAnnotationPrefix + "provider-ca-origin-key"
	// AnnotationProviderCAOriginHash is the full SHA-256 of the canonical origin tuple.
	AnnotationProviderCAOriginHash = providerCAAnnotationPrefix + "provider-ca-origin-hash"

	// ProviderCACopyKey is the fixed data key of durable CA copies.
	ProviderCACopyKey = "ca.crt"
	// ProviderCACopyPrefix prefixes durable CA copy names.
	ProviderCACopyPrefix = "provider-ca-"

	providerCACopyHashSize = 40
)

var (
	providerCACopyNameRe = regexp.MustCompile(`^` + ProviderCACopyPrefix + `[0-9a-f]{40}$`)

	providerCAOriginAnnotations = []string{
		AnnotationZoneUID,
		AnnotationProviderCAOriginZoneNamespace,
		AnnotationProviderCAOriginZoneName,
		AnnotationProviderCAOriginConfigMap,
		AnnotationProviderCAOriginKey,
		AnnotationProviderCAOriginHash,
	}

	zoneLabelKey = config.BuildLabelKey("zone")
)

// ProviderCAOrigin is the identity of an Admin-managed durable CA copy. A Zone bundle, the union
// of all CA sources of a Zone, has an empty ConfigMap and Key. Copies of a single source, written
// by earlier Admin versions, set both; they remain valid so persisted references keep working.
type ProviderCAOrigin struct {
	ZoneUID       string
	ZoneNamespace string
	ZoneName      string
	ConfigMap     string
	Key           string
}

// Canonical returns the length-prefixed canonical encoding of the origin tuple.
func (o *ProviderCAOrigin) Canonical() string {
	var b strings.Builder
	for _, f := range []string{o.ZoneUID, o.ZoneNamespace, o.ZoneName, o.ConfigMap, o.Key} {
		b.WriteString(strconv.Itoa(len(f)))
		b.WriteByte(':')
		b.WriteString(f)
	}
	return b.String()
}

// Hash returns the full hex SHA-256 of the canonical origin.
func (o *ProviderCAOrigin) Hash() string {
	sum := sha256.Sum256([]byte(o.Canonical()))
	return hex.EncodeToString(sum[:])
}

// CopyName returns the durable copy name in the generated namespace.
func (o *ProviderCAOrigin) CopyName() string {
	return ProviderCACopyPrefix + o.Hash()[:providerCACopyHashSize]
}

// Annotations returns the complete origin annotations of a copy.
func (o *ProviderCAOrigin) Annotations() map[string]string {
	a := map[string]string{
		AnnotationZoneUID:                       o.ZoneUID,
		AnnotationProviderCAOriginZoneNamespace: o.ZoneNamespace,
		AnnotationProviderCAOriginZoneName:      o.ZoneName,
		AnnotationProviderCAOriginHash:          o.Hash(),
	}
	if o.ConfigMap != "" {
		a[AnnotationProviderCAOriginConfigMap] = o.ConfigMap
		a[AnnotationProviderCAOriginKey] = o.Key
	}
	return a
}

// ProviderCAOriginFromObject reads a complete origin and verifies its hash and the copy name.
// The source ConfigMap and key must be both present (single-source copy) or both absent (Zone bundle).
func ProviderCAOriginFromObject(obj metav1.Object) (ProviderCAOrigin, bool) {
	a := obj.GetAnnotations()
	o := ProviderCAOrigin{
		ZoneUID:       a[AnnotationZoneUID],
		ZoneNamespace: a[AnnotationProviderCAOriginZoneNamespace],
		ZoneName:      a[AnnotationProviderCAOriginZoneName],
		ConfigMap:     a[AnnotationProviderCAOriginConfigMap],
		Key:           a[AnnotationProviderCAOriginKey],
	}
	_, hasConfigMap := a[AnnotationProviderCAOriginConfigMap]
	_, hasKey := a[AnnotationProviderCAOriginKey]
	if o.ZoneUID == "" || o.ZoneNamespace == "" || o.ZoneName == "" ||
		hasConfigMap != hasKey || (hasConfigMap && (o.ConfigMap == "" || o.Key == "")) {
		return o, false
	}
	if a[AnnotationProviderCAOriginHash] != o.Hash() || obj.GetName() != o.CopyName() {
		return o, false
	}
	return o, true
}

// IsProviderCACopy reports whether a ConfigMap claims to be an Admin-managed durable copy:
// its name follows the copy naming convention or it carries any origin annotation.
// Such objects must carry complete, valid provenance; ordinary local sources need not.
func IsProviderCACopy(obj metav1.Object) bool {
	if providerCACopyNameRe.MatchString(obj.GetName()) {
		return true
	}
	a := obj.GetAnnotations()
	for _, k := range providerCAOriginAnnotations {
		if _, ok := a[k]; ok {
			return true
		}
	}
	return false
}

// ValidateProviderCACopyMetadata validates label and provenance of an Admin-managed copy:
// the provider-ca label, complete origin annotations whose hash matches the copy name,
// a Zone label matching the origin, an Environment label and no ownerReferences.
func ValidateProviderCACopyMetadata(cm *corev1.ConfigMap) (ProviderCAOrigin, error) {
	if !types.HasProviderCALabel(cm.Labels) {
		return ProviderCAOrigin{}, fmt.Errorf("lacks label %s=%s", types.ProviderCALabelKey, types.ProviderCALabelValue)
	}
	origin, ok := ProviderCAOriginFromObject(cm)
	if !ok {
		return origin, errors.New("has incomplete or invalid provider CA origin metadata")
	}
	if len(cm.OwnerReferences) > 0 {
		return origin, errors.New("has unexpected ownerReferences")
	}
	if cm.Labels[config.EnvironmentLabelKey] == "" || cm.Labels[zoneLabelKey] != origin.ZoneName {
		return origin, errors.New("has environment/zone labels not matching its origin")
	}
	return origin, nil
}

// ValidateProviderCAConfigMap validates the metadata of a referenced provider CA ConfigMap.
// Every reference requires the exact provider-ca label. ConfigMaps claiming to be Admin-managed
// copies (see IsProviderCACopy) additionally require complete, valid provenance; partial or
// spoofed metadata is rejected, and the selected key must be the fixed copy key.
func ValidateProviderCAConfigMap(cm *corev1.ConfigMap, ref *types.CABundleConfigMapRef) error {
	if !types.HasProviderCALabel(cm.Labels) {
		return fmt.Errorf("lacks label %s=%s", types.ProviderCALabelKey, types.ProviderCALabelValue)
	}
	if !IsProviderCACopy(cm) {
		return nil
	}
	if _, err := ValidateProviderCACopyMetadata(cm); err != nil {
		return err
	}
	if ref == nil {
		return errors.New("is a provider CA copy; a reference is required")
	}
	if ref.Name != cm.Name {
		return fmt.Errorf("is a provider CA copy; reference name %q does not match", ref.Name)
	}
	if ref.Key != ProviderCACopyKey {
		return fmt.Errorf("is a provider CA copy; key must be %q", ProviderCACopyKey)
	}
	return nil
}
