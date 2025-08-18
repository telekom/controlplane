// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

var (
	EnvironmentLabelKey     = BuildLabelKey("environment")
	OwnerUidLabelKey        = BuildLabelKey("owner.uid")
	OwnerNameLabelKey       = BuildLabelKey("owner.name")
	OwnerNamespaceLabelKey  = BuildLabelKey("owner.namespace")
	OwnerApiVersionLabelKey = BuildLabelKey("owner.apiVersion")
	OwnerKindLabelKey       = BuildLabelKey("owner.kind")
)

func BuildLabelKey(key string) string {
	return LabelKeyPrefix + "/" + key
}
