// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
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

	secrets, err := backend.NewEnvironmentSecrets().GetSecrets()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get allowed secrets")
	}

	mutate := func() error {
		controllerutil.AddFinalizer(obj, FinalizerName)
		obj.Data = mergeDataFormat(obj.Data, secrets)
		return nil
	}
	_, err = controllerutil.CreateOrUpdate(ctx, k.client, obj, mutate)
	if err != nil {
		return backend.NewDefaultOnboardResponse(nil), backend.NewBackendError(nil, err, "failed to create or update environment")
	}

	secretRefs := make(map[string]backend.SecretRef, len(secrets))
	for secretName := range secrets {
		secretRefs[secretName] = New(env, "", "", secretName, obj.GetResourceVersion())
	}

	return backend.NewDefaultOnboardResponse(secretRefs), nil
}

func (k *KubernetesOnboarder) OnboardTeam(ctx context.Context, env string, teamId string) (backend.OnboardResponse, error) {
	obj := NewSecretObj(env, teamId, "")

	secrets, err := backend.NewTeamSecrets().GetSecrets()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get allowed secrets")
	}

	mutate := func() error {
		controllerutil.AddFinalizer(obj, FinalizerName)
		obj.Data = mergeDataFormat(obj.Data, secrets)
		return nil
	}

	_, err = controllerutil.CreateOrUpdate(ctx, k.client, obj, mutate)
	if err != nil {
		return backend.NewDefaultOnboardResponse(nil), backend.NewBackendError(nil, err, "failed to create or update team")
	}

	secretRefs := make(map[string]backend.SecretRef, len(secrets))
	for secretName := range secrets {
		secretRefs[secretName] = New(env, teamId, "", secretName, obj.GetResourceVersion())
	}

	return backend.NewDefaultOnboardResponse(secretRefs), nil
}

func (k *KubernetesOnboarder) OnboardApplication(ctx context.Context, env string, teamId string, appId string, opts ...backend.OnboardOption) (backend.OnboardResponse, error) {
	log := logr.FromContextOrDiscard(ctx)
	options := backend.OnboardOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	obj := NewSecretObj(env, teamId, appId)

	allowedSecrets := backend.NewApplicationSecrets()
	for key, value := range options.SecretValues {
		ok := allowedSecrets.TrySetSecret(key, value)
		if !ok {
			secretId := New(env, teamId, appId, key, "")
			return nil, backend.Forbidden(secretId, errors.Errorf("secret %s is not allowed for application onboarding", key))
		}
	}
	secrets, err := allowedSecrets.GetSecrets()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get allowed secrets")
	}

	mutate := func() error {
		controllerutil.AddFinalizer(obj, FinalizerName)
		obj.Data = mergeDataFormat(obj.Data, secrets)
		return nil
	}

	_, err = controllerutil.CreateOrUpdate(ctx, k.client, obj, mutate)
	if err != nil {
		return backend.NewDefaultOnboardResponse(nil), backend.NewBackendError(nil, err, "failed to create or update application")
	}

	secretRefs := make(map[string]backend.SecretRef, len(secrets)+len(options.SecretValues))
	for secretPath := range secrets {
		secretRefs[secretPath] = New(env, teamId, appId, secretPath, obj.GetResourceVersion())
	}

	for secretPath := range options.SecretValues {
		if _, ok := secretRefs[secretPath]; !ok {
			secretRefs[secretPath] = New(env, teamId, appId, secretPath, obj.GetResourceVersion())
		} else {
			log.Info("Value for secret already exists", "secretPath", secretPath)
		}
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

func mergeDataFormat(existing map[string][]byte, newData map[string]backend.SecretValue) map[string][]byte {
	result := make(map[string][]byte)
	for k, v := range newData {
		_, keyExists := existing[k]
		if keyExists && !v.AllowChange() {
			result[k] = existing[k]
			continue
		}
		if v.IsEmpty() {
			continue
		}
		result[k] = []byte(v.Value())
	}
	return result
}
