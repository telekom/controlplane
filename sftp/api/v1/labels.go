// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/config"
)

// Label keys for filtering ApprovalRequest and Approval resources.
var (
	InstanceNameKey               = config.BuildLabelKey("instance.name")
	InstanceNamespaceKey          = config.BuildLabelKey("instance.namespace")
	ZoneServiceConfigNameKey      = config.BuildLabelKey("zoneserviceconfig.name")
	ZoneServiceConfigNamespaceKey = config.BuildLabelKey("zoneserviceconfig.namespace")
)
