// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/hash"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
)

func makeName(notification *notificationv1.Notification) string {
	specHash := hash.ComputeHash(&notification.Spec, nil)
	return labelutil.NormalizeValue(notification.Spec.Purpose) + "--" + specHash
}

func ensureLabels(notification *notificationv1.Notification) {
	if notification.Labels == nil {
		notification.Labels = make(map[string]string)
	}
	notification.Labels[cconfig.BuildLabelKey("purpose")] = notification.Spec.Purpose
	notification.Labels[cconfig.BuildLabelKey("sender-type")] = string(notification.Spec.Sender.Type)
}
