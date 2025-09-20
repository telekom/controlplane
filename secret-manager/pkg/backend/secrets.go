// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"fmt"
	"maps"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

var (
	// Secrets that are allowed for each environment
	NewEnvironmentSecrets = func() *Secrets {
		return &Secrets{
			secrets: map[string]SecretValue{
				"zones": InitialString("{}"),
			},
		}
	}
	// Secrets that are allowed for each team
	NewTeamSecrets = func() *Secrets {
		return &Secrets{
			secrets: map[string]SecretValue{
				"clientSecret": InitialString(uuid.NewString()),
				"teamToken":    InitialString(uuid.NewString()),
			},
		}
	}
	// Secrets that are allowed for each application
	NewApplicationSecrets = func() *Secrets {
		return &Secrets{
			secrets: map[string]SecretValue{
				"clientSecret":    InitialString(uuid.NewString()),
				"externalSecrets": InitialString("{}"),
			},
		}
	}
)

type Secrets struct {
	secrets    map[string]SecretValue
	subSecrets map[string]map[string]string
}

func (a *Secrets) GetSecrets() (map[string]SecretValue, error) {
	if a.secrets == nil {
		return nil, nil
	}
	secrets := make(map[string]SecretValue, len(a.secrets))
	maps.Copy(secrets, a.secrets)

	var err error
	if a.subSecrets != nil {
		for key, subSecrets := range a.subSecrets {
			if !a.secrets[key].IsEmpty() {
				return nil, fmt.Errorf("cannot set sub-secrets for non-empty secret %s", key)
			}
			secrets[key], err = JSON(subSecrets)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create JSON secret for %s", key)
			}
		}
	}
	return secrets, nil
}

func (a *Secrets) TrySetSecret(secretPath string, value SecretValue) bool {
	if a.secrets == nil {
		return false
	}
	path := GetPath(secretPath)
	if _, ok := a.secrets[path]; !ok {
		return false
	}
	subPath := GetSubPath(secretPath)
	if subPath == "" {
		a.secrets[path] = value
		return true
	}
	if a.subSecrets == nil {
		a.subSecrets = make(map[string]map[string]string)
	}
	if _, ok := a.subSecrets[path]; !ok {
		a.subSecrets[path] = make(map[string]string)
	}
	a.subSecrets[path][subPath] = value.Value()
	return true
}

// SecretIdConstructor is a function type that constructs a SecretId instance.
// It must be implemented by all backends to allow generic creation of SecretId instances.
type SecretIdConstructor[T SecretId] func(env, team, app, path string, checksum string) T

// TryAddSecrets adds the provided secrets to the allowed secrets using the provided new-function to create SecretId instances.
func TryAddSecrets[T SecretId](newFunc SecretIdConstructor[T], allowedSecrets *Secrets, env, teamId, appId string, secrets map[string]SecretValue) error {
	for key, value := range secrets {
		secretId := newFunc(env, teamId, appId, key, "")
		ok := allowedSecrets.TrySetSecret(key, value)
		if !ok {
			return Forbidden(secretId, errors.Errorf("secret %s is not allowed for onboarding", key))
		}
	}
	return nil
}

// MergeSecretRefs adds missing secrets from the provided secrets map to the secretRefs map using the provided new-function to create SecretId instances.
func MergeSecretRefs[T SecretId](newFunc SecretIdConstructor[T], secretRefs map[string]SecretRef, env, teamId, appId string, secrets map[string]SecretValue) {
	for secretPath, secretValue := range secrets {
		if _, ok := secretRefs[secretPath]; !ok {
			secretRefs[secretPath] = newFunc(env, teamId, appId, secretPath, MakeChecksum(secretValue.Value()))
		}
	}
}
