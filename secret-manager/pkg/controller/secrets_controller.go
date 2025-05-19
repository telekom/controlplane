// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/google/uuid"
	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
)

type SecretResponse struct {
	Id    string `json:"id"`
	Value string `json:"value"`
}

type SecretsController interface {
	GetSecret(ctx context.Context, rawId string) (SecretResponse, error)
	SetSecret(ctx context.Context, rawId, value string) (SecretResponse, error)
	DeleteSecret(ctx context.Context, rawId string) error
}

type secretsController[T backend.SecretId, S backend.Secret[T]] struct {
	Backend backend.Backend[T, S]
}

func NewSecretsController[T backend.SecretId, S backend.Secret[T]](b backend.Backend[T, S]) SecretsController {
	return &secretsController[T, S]{Backend: b}
}

func (c *secretsController[T, S]) GetSecret(ctx context.Context, rawId string) (res SecretResponse, err error) {
	id, err := c.Backend.ParseSecretId(rawId)
	if err != nil {
		return res, err
	}

	secret, err := c.Backend.Get(ctx, id)
	if err != nil {
		return res, err
	}

	return SecretResponse{Id: secret.Id().String(), Value: secret.Value()}, nil
}

func (c *secretsController[T, S]) SetSecret(ctx context.Context, rawId, value string) (res SecretResponse, err error) {
	id, err := c.Backend.ParseSecretId(rawId)
	if err != nil {
		return res, err
	}

	secretValue := backend.String(value)
	if value == api.KeywordRotate {
		secretValue = backend.String(uuid.NewString())
	}

	secret, err := c.Backend.Set(ctx, id, secretValue)
	if err != nil {
		return res, err
	}
	return SecretResponse{Id: secret.Id().String(), Value: secret.Value()}, nil
}

func (c *secretsController[T, S]) DeleteSecret(ctx context.Context, rawId string) error {
	id, err := c.Backend.ParseSecretId(rawId)
	if err != nil {
		return err
	}

	return c.Backend.Delete(ctx, id)
}
