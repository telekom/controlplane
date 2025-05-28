// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	corev1 "k8s.io/api/core/v1"
)

type HandlingContext struct {
	Zone        *adminv1.Zone        `json:"zone,omitempty"`
	Environment *adminv1.Environment `json:"environment,omitempty"`
	Namespace   *corev1.Namespace    `json:"namespace,omitempty"`
}
