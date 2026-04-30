// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	pcpv1 "github.com/telekom/controlplane/permission/api/pcp/v1"
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
)

func RegisterSchemesOrDie(scheme *runtime.Scheme) {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(permissionv1.AddToScheme(scheme))
	utilruntime.Must(pcpv1.AddToScheme(scheme))
	utilruntime.Must(adminv1.AddToScheme(scheme))
}
