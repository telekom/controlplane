// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type applicationK8sService struct {
	client client.Client
}

// NewApplicationK8sService creates an ApplicationService backed by Kubernetes.
func NewApplicationK8sService(c client.Client) ApplicationService {
	return &applicationK8sService{client: c}
}

// applicationNamespace returns the K8s namespace for an application: <environment>--<team>.
func applicationNamespace(environment, team string) string {
	return environment + "--" + team
}

func (s *applicationK8sService) RotateApplicationSecret(ctx context.Context, input model.RotateApplicationSecretInput) (*model.RotateApplicationSecretResult, error) {
	if err := authorizeApplicationAction(ctx, input.Team); err != nil {
		return nil, err
	}

	namespace := applicationNamespace(input.Environment, input.Team)
	resourceName := input.Name

	scopedClient := cc.NewScopedClient(s.client, input.Environment)

	// Fetch existing Application to verify it exists
	app := &applicationv1.Application{}
	err := scopedClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: resourceName}, app)
	if err != nil {
		return nil, mapK8sError(err)
	}

	_, err = scopedClient.CreateOrUpdate(ctx, app, func() error {
		app.Spec.Secret = "rotate"
		return nil
	})
	if err != nil {
		return nil, mapK8sError(err)
	}

	return &model.RotateApplicationSecretResult{
		Success:      true,
		Message:      "application secret rotation initiated",
		Namespace:    &namespace,
		ResourceName: &resourceName,
	}, nil
}
