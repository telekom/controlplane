// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"maps"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const FinalizerName = "secret-manager/finalizer"

// isRetryableConflict returns true for errors that indicate a concurrent
// write race on the same Kubernetes object. This covers both:
//   - Conflict (409 StatusReasonConflict): two Updates raced, one got a stale ResourceVersion
//   - AlreadyExists (409 StatusReasonAlreadyExists): two Creates raced after both saw NotFound
func isRetryableConflict(err error) bool {
	return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
}

var _ backend.Onboarder = &KubernetesOnboarder{}

type KubernetesOnboarder struct {
	client client.Client
}

func NewOnboarder(client client.Client) *KubernetesOnboarder {
	return &KubernetesOnboarder{
		client: client,
	}
}

func (k *KubernetesOnboarder) OnboardEnvironment(ctx context.Context, env string, opts ...backend.OnboardOption) (backend.OnboardResponse, error) {
	options := backend.OnboardOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	allowedSecrets := backend.NewTeamSecrets()
	if err := backend.TryAddSecrets(New, allowedSecrets, env, backend.NoTeam, backend.NoApp, options.SecretValues); err != nil {
		return nil, err
	}
	secrets, err := allowedSecrets.GetSecrets()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get allowed secrets")
	}

	obj := NewSecretObj(env, backend.NoTeam, backend.NoApp)
	err = retry.OnError(retry.DefaultRetry, isRetryableConflict, func() error {
		_, err := controllerutil.CreateOrUpdate(ctx, k.client, obj, func() error {
			controllerutil.AddFinalizer(obj, FinalizerName)
			obj.Data = applySecrets(options.Strategy, obj.Data, secrets)
			return nil
		})
		return err
	})
	if err != nil {
		return backend.NewDefaultOnboardResponse(nil), backend.NewBackendError(nil, err, "failed to create or update environment")
	}

	secretRefs := make(map[string]backend.SecretRef, len(secrets))
	for secretName := range secrets {
		secretRefs[secretName] = New(env, backend.NoTeam, backend.NoApp, secretName, obj.GetResourceVersion())
	}
	backend.MergeSecretRefs(New, secretRefs, env, backend.NoTeam, backend.NoApp, options.SecretValues)

	return backend.NewDefaultOnboardResponse(secretRefs), nil
}

func (k *KubernetesOnboarder) OnboardTeam(ctx context.Context, env string, teamId string, opts ...backend.OnboardOption) (backend.OnboardResponse, error) {
	options := backend.OnboardOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	allowedSecrets := backend.NewTeamSecrets()
	if err := backend.TryAddSecrets(New, allowedSecrets, env, teamId, backend.NoApp, options.SecretValues); err != nil {
		return nil, err
	}
	secrets, err := allowedSecrets.GetSecrets()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get allowed secrets")
	}

	obj := NewSecretObj(env, teamId, backend.NoApp)
	err = retry.OnError(retry.DefaultRetry, isRetryableConflict, func() error {
		_, err := controllerutil.CreateOrUpdate(ctx, k.client, obj, func() error {
			controllerutil.AddFinalizer(obj, FinalizerName)
			obj.Data = applySecrets(options.Strategy, obj.Data, secrets)
			return nil
		})
		return err
	})
	if err != nil {
		return backend.NewDefaultOnboardResponse(nil), backend.NewBackendError(nil, err, "failed to create or update team")
	}

	secretRefs := make(map[string]backend.SecretRef, len(secrets))
	for secretName := range secrets {
		secretRefs[secretName] = New(env, teamId, backend.NoApp, secretName, obj.GetResourceVersion())
	}
	backend.MergeSecretRefs(New, secretRefs, env, teamId, backend.NoApp, options.SecretValues)

	return backend.NewDefaultOnboardResponse(secretRefs), nil
}

func (k *KubernetesOnboarder) OnboardApplication(ctx context.Context, env string, teamId string, appId string, opts ...backend.OnboardOption) (backend.OnboardResponse, error) {
	options := backend.OnboardOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	allowedSecrets := backend.NewApplicationSecrets()
	if err := backend.TryAddSecrets(New, allowedSecrets, env, teamId, appId, options.SecretValues); err != nil {
		return nil, err
	}
	secrets, err := allowedSecrets.GetSecrets()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get allowed secrets")
	}

	obj := NewSecretObj(env, teamId, appId)
	err = retry.OnError(retry.DefaultRetry, isRetryableConflict, func() error {
		_, err := controllerutil.CreateOrUpdate(ctx, k.client, obj, func() error {
			controllerutil.AddFinalizer(obj, FinalizerName)
			obj.Data = applySecrets(options.Strategy, obj.Data, secrets)
			return nil
		})
		return err
	})
	if err != nil {
		return backend.NewDefaultOnboardResponse(nil), backend.NewBackendError(nil, err, "failed to create or update application")
	}

	secretRefs := make(map[string]backend.SecretRef, len(secrets)+len(options.SecretValues))
	for secretPath := range secrets {
		secretRefs[secretPath] = New(env, teamId, appId, secretPath, obj.GetResourceVersion())
	}
	backend.MergeSecretRefs(New, secretRefs, env, teamId, appId, options.SecretValues)

	return backend.NewDefaultOnboardResponse(secretRefs), nil
}

func (k *KubernetesOnboarder) DeleteEnvironment(ctx context.Context, env string) error {
	obj := NewSecretObj(env, backend.NoTeam, backend.NoApp)

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
	obj := NewSecretObj(env, id, backend.NoApp)

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
	id := New(env, teamId, appId, backend.NoValue, backend.NoChecksum)
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

// applySecrets applies newData to the existing secret data based on the given strategy.
// With "merge", existing keys not present in newData are preserved in the result.
// With "replace" (or any other value, including empty string), only keys from newData
// are included in the result.
// In both modes, keys where AllowChange() is false and already exist in the existing
// data will retain their existing value (immutable initial values).
func applySecrets(strategy backend.WriteStrategy, existing map[string][]byte, newData map[string]backend.SecretValue) map[string][]byte {
	result := make(map[string][]byte)

	// For merge strategy, start by copying all existing keys
	if strategy == backend.StrategyMerge {
		maps.Copy(result, existing)
	}

	// Apply new data on top
	for k, v := range newData {
		if _, keyExists := existing[k]; keyExists && !v.AllowChange() {
			// Preserve existing value for immutable (InitialString) secrets
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
