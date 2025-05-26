// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

func RegisterSchemesOrDie(scheme *runtime.Scheme) {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(adminv1.AddToScheme(scheme))
	utilruntime.Must(identityv1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.AddToScheme(scheme))
}
