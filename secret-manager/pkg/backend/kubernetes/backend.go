// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ backend.Backend[Id, backend.DefaultSecret[Id]] = &KubernetesBackend{}

type KubernetesBackend struct {
	client               client.Client
	MatchResourceVersion bool
}

func NewBackend(client client.Client) backend.Backend[Id, backend.DefaultSecret[Id]] {
	return &KubernetesBackend{
		client:               client,
		MatchResourceVersion: false, // Cannot be set to true, as the resource version can change from different secret changes
	}
}

func (k *KubernetesBackend) ParseSecretId(rawId string) (Id, error) {
	return FromString(rawId)
}

func (k *KubernetesBackend) Get(ctx context.Context, secretId Id) (res backend.DefaultSecret[Id], err error) {
	log := logr.FromContextOrDiscard(ctx)
	obj := &corev1.Secret{}
	err = k.client.Get(ctx, secretId.ObjectKey(), obj)
	if err != nil {
		return res, handleError(err, secretId)
	}

	if k.MatchResourceVersion {
		if secretId.checksum != "" && secretId.checksum != obj.GetResourceVersion() {
			return res, backend.ErrBadChecksum(secretId)
		}
	}

	subPath := secretId.SubPath()
	path := secretId.Path()
	log.Info("get secret", "path", path, "subPath", subPath)
	if subPath != "" {
		data, ok := obj.Data[path]
		if !ok {
			return res, backend.ErrSecretNotFound(secretId)
		}
		result := gjson.GetBytes(data, subPath)
		if !result.Exists() {
			return res, backend.ErrSecretNotFound(secretId)
		}
		return backend.NewDefaultSecret(secretId, result.String()), nil
	}
	data, ok := obj.Data[path]
	if !ok {
		return res, backend.ErrSecretNotFound(secretId)
	}
	return backend.NewDefaultSecret(secretId, string(data)), nil
}

func (k *KubernetesBackend) Set(ctx context.Context, secretId Id, secretValue backend.SecretValue) (res backend.DefaultSecret[Id], err error) {
	log := logr.FromContextOrDiscard(ctx)
	secret, err := k.Get(ctx, secretId)
	if err != nil {
		// If the secret is not found, we can create it
		// For all other cases, we return an error immediately
		if !backend.IsNotFoundErr(err) {
			return res, err
		}
	}

	if secret.Value() != "" && !secretValue.AllowChange() {
		return secret, nil
	}

	if secretValue.EqualString(secret.Value()) {
		return secret, nil
	}

	ref := secretId.ObjectKey()
	obj := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Name,
			Namespace: ref.Namespace,
		},
	}

	subPath := secretId.SubPath()
	path := secretId.Path()
	log.Info("set secret", "path", path, "subPath", subPath)

	mutate := func() error {
		if k.MatchResourceVersion {
			if secretId.checksum != "" && secretId.checksum != obj.GetResourceVersion() {
				return backend.ErrBadChecksum(secretId)
			}
		}

		if subPath != "" {
			data, ok := obj.Data[path]
			if !ok {
				return backend.ErrSecretNotFound(secretId)
			}

			newData, err := sjson.SetBytes(data, subPath, secretValue.Value())
			if err != nil {
				return handleError(err, secretId)
			}

			obj.Data[path] = newData
			return nil
		}

		if obj.Data == nil {
			obj.Data = make(map[string][]byte)
		}
		obj.Data[path] = []byte(secretValue.Value())
		return nil
	}
	_, err = controllerutil.CreateOrUpdate(ctx, k.client, obj, mutate)
	if err != nil {
		return res, handleError(err, secretId)
	}

	newId := secretId.CopyWithChecksum(obj.GetResourceVersion())
	return backend.NewDefaultSecret(newId, ""), nil
}

func (k *KubernetesBackend) Delete(ctx context.Context, secretId Id) error {
	log := logr.FromContextOrDiscard(ctx)
	obj := NewSecretObj(secretId.env, secretId.team, secretId.app)

	mutate := func() error {
		if k.MatchResourceVersion {
			if secretId.checksum != "" && secretId.checksum != obj.GetResourceVersion() {
				return backend.ErrBadChecksum(secretId)
			}
		}

		if obj.Data == nil {
			return backend.ErrSecretNotFound(secretId)
		}

		subPath := secretId.SubPath()
		path := secretId.Path()
		log.Info("delete secret", "path", path, "subPath", subPath)
		if subPath != "" {
			data, ok := obj.Data[path]
			if !ok {
				return backend.ErrSecretNotFound(secretId)
			}
			newData, err := sjson.DeleteBytes(data, subPath)
			if err != nil {
				return handleError(err, secretId)
			}
			obj.Data[path] = newData
			return nil
		}

		if obj.Data[path] != nil {
			delete(obj.Data, path)
		}
		return nil
	}

	_, err := controllerutil.CreateOrUpdate(ctx, k.client, obj, mutate)
	if err != nil {
		return handleError(err, secretId)
	}

	return err
}

func handleError(err error, id Id) error {
	if backend.IsBackendError(err) {
		return err
	}
	if apierrors.IsNotFound(err) {
		return backend.ErrIncorrectState(id, err)
	}
	return backend.NewBackendError(id, err, "InternalError")
}
