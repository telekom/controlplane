// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package remoteorganization

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RemoteOrganizationHandler struct{}

func (h *RemoteOrganizationHandler) CreateOrUpdate(ctx context.Context, obj *adminv1.RemoteOrganization) (err error) {
	c := cclient.ClientFromContextOrDie(ctx)
	envName := contextutil.EnvFromContextOrDie(ctx)
	obj.SetCondition(condition.NewProcessingCondition("Provisioning", "RemoteOrganization is being provisioned"))

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: calculateName(envName, obj.Spec.Id),
		},
	}

	mutator := func() error {
		namespace.Labels = map[string]string{
			config.EnvironmentLabelKey: envName,
		}
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, namespace, mutator)
	if err != nil {
		return errors.Wrapf(err, "❌ failed to create or update namespace %s, environment %s", namespace.Name, envName)
	}

	obj.Status.Namespace = namespace.Name
	obj.SetCondition(condition.NewReadyCondition("Provisioned", "RemoteOrganization has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("RemoteOrganization has been provisioned"))
	return nil
}

func (h *RemoteOrganizationHandler) Delete(ctx context.Context, obj *adminv1.RemoteOrganization) error {
	c := cclient.ClientFromContextOrDie(ctx)
	envName := contextutil.EnvFromContextOrDie(ctx)

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: calculateName(envName, obj.Spec.Id),
		},
	}
	err := c.Delete(ctx, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "❌ failed to delete namespace %s", namespace.Name)
	}
	return nil
}

func calculateName(envName, name string) string {
	return strings.ToLower(fmt.Sprintf("%s--%s", envName, name))
}
