// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

const (
	ExposureVariantDefault        = "default"
	ExposureVariantMCP            = "mcp"
	ExposureVariantTelecontextMCP = "telecontextmcp"
	ExposureVariantAgent          = "agent"
)

var (
	EnvironmentLabelKey     = BuildLabelKey("environment")
	OwnerUidLabelKey        = BuildLabelKey("owner.uid")
	DomainLabelKey          = BuildLabelKey("domain")
	ExposureVariantLabelKey = BuildLabelKey("exposure.variant")
)

func BuildLabelKey(key string) string {
	return LabelKeyPrefix + "/" + key
}
