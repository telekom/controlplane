// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
)

// Replacer is an interface for replacing secret values in different types of objects.
type Replacer interface {
	ReplaceAll(ctx context.Context, obj any, jsonPaths []string) (any, error)
}

type SecretStore[T store.Object] struct {
	store.ObjectStore[T]

	secretJsonPaths []string
	// resolver is used to replace secret placeholders with their values.
	resolver Replacer
	// obfuscator is used to replace secret placeholders with a placeholder like "******".
	obfuscator Replacer
}

func WrapStore[T store.Object](s store.ObjectStore[T], secretJsonPaths []string, secretsResolver Replacer) *SecretStore[T] {
	return &SecretStore[T]{
		secretJsonPaths: secretJsonPaths,
		resolver:        secretsResolver,
		obfuscator:      NewObfuscator(),
		ObjectStore:     s,
	}
}

func (s *SecretStore[T]) Get(ctx context.Context, namespace, name string) (res T, err error) {
	res, err = s.ObjectStore.Get(ctx, namespace, name)
	if err != nil {
		return res, errors.Wrap(err, "failed to get secret")
	}

	if len(s.secretJsonPaths) == 0 {
		return res, nil
	}

	replacer := s.resolver
	if security.IsObfuscated(ctx) {
		replacer = s.obfuscator
	}

	return s.forItem(ctx, replacer, res)
}

func (s *SecretStore[T]) List(ctx context.Context, opts store.ListOpts) (*store.ListResponse[T], error) {
	res, err := s.ObjectStore.List(ctx, opts)
	if err != nil {
		return res, errors.Wrap(err, "failed to list secrets")
	}

	if len(s.secretJsonPaths) == 0 {
		return res, nil
	}

	replacer := s.resolver
	if security.IsObfuscated(ctx) {
		replacer = s.obfuscator
	}

	for i := range res.Items {
		res.Items[i], err = s.forItem(ctx, replacer, res.Items[i])
		if err != nil {
			return nil, errors.Wrap(err, "failed to replace secret values")
		}
	}

	return res, nil
}

func (s *SecretStore[T]) forItem(ctx context.Context, replacer Replacer, item T) (T, error) {
	o, err := replacer.ReplaceAll(ctx, item, s.secretJsonPaths)
	if err != nil {
		return item, errors.Wrap(err, "failed to replace secret values")
	}
	item, ok := o.(T)
	if !ok {
		return item, fmt.Errorf("failed to cast object to type %T", item)
	}
	return item, nil
}
