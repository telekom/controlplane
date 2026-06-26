// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	gateway "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

func RegisterSchemesOrDie(scheme *runtime.Scheme) {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(adminv1.AddToScheme(scheme))
	utilruntime.Must(approvalv1.AddToScheme(scheme))
	utilruntime.Must(filev1.AddToScheme(scheme))
	utilruntime.Must(identityv1.AddToScheme(scheme))
	utilruntime.Must(sftpv1.AddToScheme(scheme))
	utilruntime.Must(gateway.AddToScheme(scheme))
}
