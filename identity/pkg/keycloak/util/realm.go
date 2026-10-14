// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"maps"
	"slices"

	"k8s.io/utils/ptr"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
)

// MapToRealmRepresentation maps the realm spec to the representation used to
// create a new realm. Attributes are only set when the spec defines any.
func MapToRealmRepresentation(realm *identityv1.Realm) api.RealmRepresentation {
	rep := api.RealmRepresentation{
		Enabled: ptr.To(true),
		Realm:   &realm.Name,
	}
	if len(realm.Spec.Attributes) > 0 {
		attrs := make(map[string]interface{}, len(realm.Spec.Attributes))
		for k, v := range realm.Spec.Attributes {
			attrs[k] = v
		}
		rep.Attributes = &attrs
	}
	return rep
}

// ReconcileRealmAttributes computes the attributes of an existing realm after
// applying the desired attributes. Existing attributes are kept, keys in
// previouslyManaged that are not desired anymore are removed and desired
// attributes are overlaid. The existing map is not modified.
//
// Keycloak treats a non-nil attributes map in a realm PUT as the complete set:
// keys missing from it are removed. The result therefore always contains all
// attributes that should remain. The returned bool is false (and the map nil)
// when no attributes are managed, i.e. the existing attributes must be left
// untouched.
func ReconcileRealmAttributes(existing map[string]interface{}, desired map[string]string, previouslyManaged []string) (map[string]interface{}, bool) {
	if len(desired) == 0 && len(previouslyManaged) == 0 {
		return nil, false
	}

	result := make(map[string]interface{}, len(existing)+len(desired))
	maps.Copy(result, existing)
	for _, key := range previouslyManaged {
		if _, ok := desired[key]; !ok {
			delete(result, key)
		}
	}
	for k, v := range desired {
		result[k] = v
	}
	return result, true
}

// ManagedAttributeKeys returns the sorted keys of the given attributes, or nil
// if there are none.
func ManagedAttributeKeys(attributes map[string]string) []string {
	if len(attributes) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(attributes))
}

// RealmAttribute returns the string value of the given attribute key.
func RealmAttribute(attributes map[string]interface{}, key string) string {
	if s, ok := attributes[key].(string); ok {
		return s
	}
	return ""
}

// Attributes dereferences an optional attributes map.
//
//nolint:gocritic // The generated API represents optional attributes as a pointer to a map.
func Attributes(attributes *map[string]interface{}) map[string]interface{} {
	if attributes == nil {
		return nil
	}
	return *attributes
}

func CompareRealmRepresentation(existingRealm, newRealm *api.RealmRepresentation) bool {
	if existingRealm == nil || newRealm == nil {
		return existingRealm == newRealm
	}
	if existingRealm.Realm == nil || newRealm.Realm == nil {
		if existingRealm.Realm != newRealm.Realm {
			return false
		}
	} else if *existingRealm.Realm != *newRealm.Realm {
		return false
	}
	if !compareAttributes(Attributes(existingRealm.Attributes), newRealm.Attributes) {
		return false
	}
	if existingRealm.Enabled == nil || newRealm.Enabled == nil {
		return existingRealm.Enabled == newRealm.Enabled
	}
	return *existingRealm.Enabled == *newRealm.Enabled
}

// compareAttributes reports whether applying desired would leave existing
// unchanged. A nil desired map means attributes are not managed.
//
//nolint:gocritic // The generated API represents optional attributes as a pointer to a map.
func compareAttributes(existing map[string]interface{}, desired *map[string]interface{}) bool {
	if desired == nil {
		return true
	}
	if len(existing) != len(*desired) {
		return false
	}
	for k, v := range *desired {
		ev, ok := existing[k]
		if !ok || ev != v {
			return false
		}
	}
	return true
}

func MergeRealmRepresentation(existingRealm, newRealm *api.RealmRepresentation) *api.RealmRepresentation {
	existingRealm.Enabled = newRealm.Enabled
	existingRealm.Realm = newRealm.Realm
	if newRealm.Attributes != nil {
		existingRealm.Attributes = newRealm.Attributes
	}
	return existingRealm
}
