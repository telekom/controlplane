// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"

	"github.com/google/uuid"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const FinalizerName = "secret-manager/finalizer"

var _ backend.Onboarder = &KubernetesOnboarder{}

type KubernetesOnboarder struct {
	client client.Client
}

func NewOnboarder(client client.Client) *KubernetesOnboarder {
	return &KubernetesOnboarder{
		client: client,
	}
}

func (k *KubernetesOnboarder) OnboardEnvironment(ctx context.Context, env string) (backend.OnboardResponse, error) {
	obj := NewSecretObj(env, "", "")

	mutate := func() error {
		controllerutil.AddFinalizer(obj, FinalizerName)

		obj.Labels = map[string]string{
			"cp.ei.telekom.de/environment": env,
			"app.kubernetes.io/managed-by": "secret-manager",
		}
		obj.Type = corev1.SecretTypeOpaque
		if obj.Data == nil {
			obj.Data = map[string][]byte{
				"zones": []byte(""),
			}
		}

		return nil
	}
	_, err := controllerutil.CreateOrUpdate(ctx, k.client, obj, mutate)
	if err != nil {
		return backend.NewDefaultOnboardResponse(nil), backend.NewBackendError(nil, err, "failed to create or update environment")
	}

	secretRefs := make(map[string]backend.SecretRef, len(backend.EnvironmentSecrets))
	for _, secret := range backend.EnvironmentSecrets {
		secretRefs[secret] = New(env, "", "", secret, obj.GetResourceVersion())
	}

	return backend.NewDefaultOnboardResponse(secretRefs), nil
}

func (k *KubernetesOnboarder) OnboardTeam(ctx context.Context, env string, teamId string) (backend.OnboardResponse, error) {
	obj := NewSecretObj(env, teamId, "")

	mutate := func() error {
		controllerutil.AddFinalizer(obj, FinalizerName)

		obj.Labels = map[string]string{
			"cp.ei.telekom.de/environment": env,
			"cp.ei.telekom.de/team":        teamId,
			"app.kubernetes.io/managed-by": "secret-manager",
		}
		obj.Type = corev1.SecretTypeOpaque
		if obj.Data == nil { // Only do the initial onboarding. After that, the data can only be changed using the secrets-API
			obj.Data = map[string][]byte{
				"clientSecret": []byte(uuid.NewString()),
				"teamToken":    []byte(uuid.NewString()),
			}
		}
		return nil
	}

	_, err := controllerutil.CreateOrUpdate(ctx, k.client, obj, mutate)
	if err != nil {
		return backend.NewDefaultOnboardResponse(nil), backend.NewBackendError(nil, err, "failed to create or update team")
	}

	secretRefs := make(map[string]backend.SecretRef, len(backend.TeamSecrets))
	for _, secret := range backend.TeamSecrets {
		secretRefs[secret] = New(env, teamId, "", secret, obj.GetResourceVersion())
	}

	return backend.NewDefaultOnboardResponse(secretRefs), nil
}

func (k *KubernetesOnboarder) OnboardApplication(ctx context.Context, env string, teamId string, appId string) (backend.OnboardResponse, error) {
	obj := NewSecretObj(env, teamId, appId)

	mutate := func() error {
		controllerutil.AddFinalizer(obj, FinalizerName)

		if obj.Data == nil { // Only do the initial onboarding. After that, the data can only be changed using the secrets-API
			appData := map[string]string{
				"clientSecret":    uuid.NewString(),
				"externalSecrets": "{}",
			}
			obj.Data = convertToDataFormat(appData)
		}
		return nil
	}

	_, err := controllerutil.CreateOrUpdate(ctx, k.client, obj, mutate)
	if err != nil {
		return backend.NewDefaultOnboardResponse(nil), backend.NewBackendError(nil, err, "failed to create or update application")
	}

	secretRefs := make(map[string]backend.SecretRef, len(backend.ApplicationSecrets))
	for _, secret := range backend.ApplicationSecrets {
		secretRefs[secret] = New(env, teamId, appId, secret, obj.GetResourceVersion())
	}

	return backend.NewDefaultOnboardResponse(secretRefs), nil
}

func (k *KubernetesOnboarder) DeleteEnvironment(ctx context.Context, env string) error {
	obj := NewSecretObj(env, "", "")

	err := RemoveFinalizer(ctx, k.client, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return backend.ErrNotFound()
		}
		return backend.NewBackendError(nil, err, "failed to remove finalizer")
	}
	err = k.client.Delete(ctx, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return backend.ErrNotFound()
		}
		return backend.NewBackendError(nil, err, "failed to delete environment")
	}
	return nil
}

func (k *KubernetesOnboarder) DeleteTeam(ctx context.Context, env string, id string) error {
	obj := NewSecretObj(env, id, "")

	err := RemoveFinalizer(ctx, k.client, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return backend.ErrNotFound()
		}
		return backend.NewBackendError(nil, err, "failed to remove finalizer")
	}
	err = k.client.Delete(ctx, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return backend.ErrNotFound()
		}
		return backend.NewBackendError(nil, err, "failed to delete team")
	}
	return nil
}

func (k *KubernetesOnboarder) DeleteApplication(ctx context.Context, env string, teamId string, appId string) error {
	obj := NewSecretObj(env, teamId, appId)

	err := RemoveFinalizer(ctx, k.client, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return backend.ErrNotFound()
		}
		return backend.NewBackendError(nil, err, "failed to remove finalizer")
	}
	err = k.client.Delete(ctx, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return backend.ErrNotFound()
		}
		return backend.NewBackendError(nil, err, "failed to delete application")
	}
	return nil
}

func NewSecretObj(env, teamId, appId string) *corev1.Secret {
	id := New(env, teamId, appId, "", "")
	ref := id.ObjectKey()
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Name,
			Namespace: ref.Namespace,
			Labels: map[string]string{
				"cp.ei.telekom.de/environment": env,
				"cp.ei.telekom.de/team":        teamId,
				"cp.ei.telekom.de/application": appId,
				"app.kubernetes.io/managed-by": "secret-manager",
			},
		},
		Type: corev1.SecretTypeOpaque,
	}
}

func RemoveFinalizer(ctx context.Context, c client.Client, obj client.Object) error {
	err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	if err != nil {
		return err
	}
	if controllerutil.RemoveFinalizer(obj, FinalizerName) {
		if err := c.Update(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

func convertToDataFormat(in map[string]string) map[string][]byte {
	out := map[string][]byte{}
	for k, v := range in {
		out[k] = []byte(v)
	}
	return out
}
