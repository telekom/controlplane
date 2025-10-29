// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"strings"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/hash"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
)

func makeName(name string, notification *notificationv1.Notification) string {
	specHash := hash.ComputeHash(&notification.Spec, nil)

	var resourceName string
	if name != "" {
		resourceName = labelutil.NormalizeValue(name)
	} else {
		resourceName = labelutil.NormalizeValue(notification.Spec.Purpose)
	}

	return resourceName + "--" + specHash
}

func ensureLabels(notification *notificationv1.Notification) {
	if notification.Labels == nil {
		notification.Labels = make(map[string]string)
	}
	notification.Labels[cconfig.BuildLabelKey("purpose")] = notification.Spec.Purpose
	notification.Labels[cconfig.BuildLabelKey("sender-type")] = string(notification.Spec.Sender.Type)
}

// ExtractApplicationInformation extract values from the target structure based on conventions
func ExtractApplicationInformation(target types.TypedObjectRef) (kind, application, basepath, group, team string) {
	kind = target.Kind                            // k8s kind
	splitName := strings.Split(target.Name, "--") //target.Name: application--basepath
	if len(splitName) >= 2 {
		application = splitName[0]
		basepath = splitName[1]
	} else {
		application = target.Name
	}

	splitNamespace := strings.Split(target.Namespace, "--") //target.Namespace: env--group--team
	if len(splitNamespace) >= 3 {
		group = splitNamespace[1]
		team = splitNamespace[2]
	} else {
		group = target.Namespace
		team = target.Namespace
	}
	return
}
