// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	secrets "github.com/telekom/controlplane/secret-manager/api"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ Replacer = &SecretManagerResolver{}

type SecretManagerResolver struct {
	M secrets.SecretsApi
}

func NewSecretManagerResolver(api secrets.SecretsApi) *SecretManagerResolver {
	return &SecretManagerResolver{
		M: api,
	}
}

func NewDefaultSecretManagerResolver() *SecretManagerResolver {
	return &SecretManagerResolver{
		M: secrets.NewSecrets(),
	}
}

func (s *SecretManagerResolver) ReplaceAll(ctx context.Context, obj any, jsonPaths []string) (any, error) {
	log := logr.FromContextOrDiscard(ctx)
	if obj == nil {
		return nil, nil
	}
	if len(jsonPaths) == 0 {
		return obj, nil
	}

	b, ok := obj.([]byte)
	if ok {
		return s.ReplaceAllFromBytes(ctx, b, jsonPaths)
	}
	str, ok := obj.(string)
	if ok {
		b, err := s.ReplaceAllFromBytes(ctx, []byte(str), jsonPaths)
		if b != nil {
			return string(b), err
		}
		return nil, err
	}
	m, ok := obj.(map[string]any)
	if ok {
		return s.ReplaceAllFromMap(ctx, m, jsonPaths)
	}

	u, ok := obj.(*unstructured.Unstructured)
	if ok {
		m, err := s.ReplaceAllFromMap(ctx, u.UnstructuredContent(), jsonPaths)
		if err != nil {
			return nil, errors.Wrap(err, "failed to replace all from unstructured")
		}
		u.SetUnstructuredContent(m)
		return u, nil
	}

	b, err := json.Marshal(obj)
	if err == nil {
		log.V(1).Info("Replacing secrets in object", "type", fmt.Sprintf("%T", obj))

		b, err = s.ReplaceAllFromBytes(ctx, b, jsonPaths)
		if err != nil {
			return nil, errors.Wrap(err, "failed to replace all from json")
		}
		err = json.Unmarshal(b, &obj)
		if err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal replaced json")
		}
		log.V(1).Info("Replaced secrets in object", "type", fmt.Sprintf("%T", obj))
		return obj, nil
	}

	return nil, fmt.Errorf("unsupported type %T", obj)
}

func (s *SecretManagerResolver) ReplaceAllFromBytes(ctx context.Context, b []byte, jsonPaths []string) ([]byte, error) {
	log := logr.FromContextOrDiscard(ctx)
	for _, jsonPath := range jsonPaths {
		result := gjson.GetBytes(b, jsonPath)
		if !result.Exists() {
			continue
		}
		if result.IsArray() {
			var err error
			paths := result.Paths(string(b))
			if len(paths) == 0 {
				continue
			}
			log.V(1).Info("Replacing secrets in array", "jsonPath", paths)
			b, err = s.ReplaceAllFromBytes(ctx, b, paths)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to replace all from bytes for json path %s", jsonPath)
			}
			continue
		}
		if result.IsObject() {
			return nil, errors.New("object not supported")
		}

		possibleSecret := result.String()
		secretRef, ok := secrets.FromRef(possibleSecret)
		if !ok {
			log.V(1).Info("Secret is not a placeholder, skipping ...")
			continue
		}
		log.V(1).Info("Replacing secret", "jsonPath", jsonPath, "secretRef", secretRef)
		secretValue, err := s.M.Get(ctx, secretRef)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get secret value")
		}

		b, err = sjson.SetBytes(b, jsonPath, secretValue)
		if err != nil {
			return nil, errors.Wrap(err, "failed to set secret value")
		}
		log.V(1).Info("Replaced secret", "jsonPath", jsonPath, "secretValue", secretValue)
	}

	return b, nil
}

func (s *SecretManagerResolver) ReplaceAllFromMap(ctx context.Context, m map[string]any, jsonPaths []string) (map[string]any, error) {
	log := logr.FromContextOrDiscard(ctx)
	for _, jsonPath := range jsonPaths {
		// TODO: refactor this to support arrays
		if strings.Contains(jsonPath, "#") {
			return nil, errors.New("arrays are not supported when using maps")
		}

		parts := strings.Split(jsonPath, ".")

		result, ok, err := unstructured.NestedString(m, parts...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get json path")
		}
		if !ok || result == "" {
			continue
		}
		possibleSecret := result
		secretRef, ok := secrets.FromRef(possibleSecret)
		if !ok {
			log.V(1).Info("Secret is not a placeholder, skipping ...")
			continue
		}
		secretValue, err := s.M.Get(ctx, secretRef)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get secret value")
		}

		err = unstructured.SetNestedField(m, secretValue, parts...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to set secret value")
		}
	}

	return m, nil
}
