// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"log"
	"sync"

	"github.com/go-logr/logr"
)

var once sync.Once
var api SecretManager

// Get retrieves the secret value from the secret manager.
// The difference to the Get function in the api package is that this function
// will check if this is a secret placeholder and if so, it will call the secret manager API.
// If the secret is not a placeholder, it will return the secretRef as is.
var Get = func(ctx context.Context, secretRef string) (value string, err error) {
	log := logr.FromContextOrDiscard(ctx)
	secretId, ok := FromRef(secretRef)
	if !ok {
		log.V(1).Info("Secret is not a placeholder, skipping ...")
		return secretRef, nil
	}

	value, err = API().Get(ctx, secretId)
	if err != nil {
		return "", err
	}
	log.V(1).Info("Secret resolved successfully", "id", secretId)
	return value, nil
}

// Set sets the secret value in the secret manager.
// The difference to the Set function in the api package is that this function
// will check if this is a secret placeholder and if so, it will call the secret manager API.
// If the secret is not a placeholder, it will return the secretRef as is.
var Set = func(ctx context.Context, secretRef string, value string) (newRef string, err error) {
	log := logr.FromContextOrDiscard(ctx)
	secretId, ok := FromRef(secretRef)
	if !ok {
		log.V(1).Info("Secret is not a placeholder, skipping ...")
		return secretId, nil
	}

	newID, err := API().Set(ctx, secretId, value)
	if err != nil {
		return secretRef, err
	}
	log.V(1).Info("Secret set successfully", "newID", newID)

	return ToRef(newID), nil
}

var API = func() SecretManager {
	if api == nil {
		once.Do(func() {
			api = New()
		})

		if api == nil {
			log.Fatal("SecretManager API is not initialized.")
		}
	}
	return api
}
