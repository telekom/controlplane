// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package namespace

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	handler "github.com/telekom/controlplane/organization/internal/handler/team/handler"
)

type NamespaceHandler struct {
}

var _ handler.ObjectHandler = &NamespaceHandler{}

func (n NamespaceHandler) CreateOrUpdate(ctx context.Context, owner *organizationv1.Team) error {
	k8sClient := client.ClientFromContextOrDie(ctx)
	env := contextutil.EnvFromContextOrDie(ctx)
	nsObj := buildNamespaceObj(buildNamespaceName(env, owner))

	mutator := func() error {
		nsObj.SetLabels(owner.GetLabels())
		return nil
	}

	owner.Status.Namespace = nsObj.Name
	_, err := k8sClient.CreateOrUpdate(ctx, nsObj, mutator)
	return err
}

func (n NamespaceHandler) Delete(ctx context.Context, owner *organizationv1.Team) error {
	k8sClient := client.ClientFromContextOrDie(ctx)
	teamNamespaceObj := buildNamespaceObj(owner.Status.Namespace)
	return k8sClient.Delete(ctx, teamNamespaceObj)
}

func (n NamespaceHandler) Identifier() string {
	return "namespace"
}
