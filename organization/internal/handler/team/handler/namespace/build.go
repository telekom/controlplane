// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package namespace

import (
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildNamespaceObj(namespace string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
}

func buildNamespaceName(env string, t *organizationv1.Team) string {
	return env + "--" + t.Spec.Group + "--" + t.Spec.Name
}
