// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur

import (
	"context"
	"encoding/json"
	"maps"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur/bouncer"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var _ backend.Backend[ConjurSecretId, backend.DefaultSecret[ConjurSecretId]] = &ConjurBackend{}

type ConjurBackend struct {
	writeAPI ConjurAPI
	readAPI  ConjurAPI

	// MustMatchChecksum is used to check if the checksum of the requested secret
	// is actually the same as the one in the backend. If it is not, an error is returned.
	MustMatchChecksum bool

	// bouncer provides per-variable locking for Set operations to prevent
	// concurrent read-modify-write races on the same secret.
	bouncer bouncer.Bouncer
}

func NewBackend(writeAPI, readAPI ConjurAPI) *ConjurBackend {
	return &ConjurBackend{
		writeAPI:          writeAPI,
		readAPI:           readAPI,
		MustMatchChecksum: false,
	}
}

func (c *ConjurBackend) WithBouncer(b bouncer.Bouncer) *ConjurBackend {
	if b == nil {
		return c
	}
	c.bouncer = b
	return c
}

func (c *ConjurBackend) ParseSecretId(rawId string) (ConjurSecretId, error) {
	return FromString(rawId)
}

func (c *ConjurBackend) Get(ctx context.Context, id ConjurSecretId) (res backend.DefaultSecret[ConjurSecretId], err error) {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Getting secret", "variableID", id.VariableId())
	secret, err := c.readAPI.RetrieveSecret(id.VariableId())
	if err != nil {
		return res, handleError(err, id)
	}

	subPath := id.SubPath()
	if subPath != backend.NoSubPath {
		log.Info("Subpath found. Using subpath to get secret", "subPath", subPath)
		result := gjson.GetBytes(secret, subPath)
		if !result.Exists() {
			return res, backend.ErrSecretNotFound(id)
		}
		newId := id.CopyWithChecksum(backend.MakeChecksum(result.String()))
		res = backend.NewDefaultSecret(newId, result.String())
	} else {
		newId := id.CopyWithChecksum(backend.MakeChecksum(string(secret)))
		res = backend.NewDefaultSecret(newId, string(secret))
	}

	if id.checksum != backend.NoChecksum && id.checksum != res.Id().checksum {
		if c.MustMatchChecksum {
			log.Info("Checksum mismatch. Returning error", "id", id.String())
			return res, backend.ErrBadChecksum(id)

		}
		log.Info("Checksum mismatch but its ignored. Returning secret", "id", id.String())
		return res, nil
	}

	return res, nil
}

func (c *ConjurBackend) Set(ctx context.Context, id ConjurSecretId, secretValue backend.SecretValue, opts ...backend.WriteOption) (res backend.DefaultSecret[ConjurSecretId], err error) {
	if c.bouncer != nil {
		lockKey := id.VariableId()
		bouncerErr := c.bouncer.RunB(ctx, lockKey, func(ctx context.Context) error {
			res, err = c.doSet(ctx, id, secretValue, opts...)
			return err
		})
		if bouncerErr != nil && errors.Is(bouncerErr, bouncer.ErrLockNotAcquired) {
			return res, backend.NewBackendError(nil, bouncerErr, backend.TypeErrTooManyRequests)
		}
		return res, err
	}
	return c.doSet(ctx, id, secretValue, opts...)
}

func (c *ConjurBackend) doSet(ctx context.Context, id ConjurSecretId, secretValue backend.SecretValue, opts ...backend.WriteOption) (res backend.DefaultSecret[ConjurSecretId], err error) {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Setting secret", "id", id.VariableId())

	options := backend.WriteOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	data, err := c.readAPI.RetrieveSecret(id.VariableId())
	if err != nil {
		cErr, ok := AsError(err)
		if ok && cErr.Code == 404 {
			return c.initialCreation(ctx, id, secretValue)
		}
		return res, handleError(err, id)
	}

	subPath := id.SubPath()
	currentValue := string(data)
	if subPath != backend.NoSubPath {
		result := gjson.GetBytes(data, subPath)
		if !result.Exists() {
			currentValue = backend.NoValue
		} else {
			currentValue = result.String()
		}
	}

	if currentValue != backend.NoValue && !secretValue.AllowChange() {
		log.Info("Secret already exists but is not empty. Not updating...", "id", id.String())
		return backend.NewDefaultSecret(id.CopyWithChecksum(backend.MakeChecksum(currentValue)), currentValue), nil
	}

	// For merge strategy, shallow-merge JSON objects with the existing value
	effectiveValue := secretValue.Value()
	if options.Strategy == backend.StrategyMerge && currentValue != backend.NoValue {
		merged, wasMerged := shallowMergeJSON(currentValue, effectiveValue)
		if wasMerged {
			log.Info("Merge strategy: shallow-merged JSON values", "id", id.String())
			effectiveValue = merged
		}
	}

	if effectiveValue == currentValue {
		log.Info("Secret already exists and is up to date", "id", id.String())
		return backend.NewDefaultSecret(id, currentValue), nil
	}

	nextValue := effectiveValue
	if subPath != backend.NoSubPath {
		log.Info("Subpath found. Using subpath to set secret", "subPath", subPath)
		newData, err := sjson.SetBytes(data, subPath, nextValue)
		if err != nil {
			return res, handleError(err, id)
		}
		nextValue = string(newData)
	}

	log.Info("Secret already exists but is not up to date. Updating...", "id", id.String())
	err = c.writeAPI.AddSecret(id.VariableId(), nextValue)
	if err != nil {
		return res, handleError(err, id)
	}
	newId := id.CopyWithChecksum(backend.MakeChecksum(effectiveValue))
	return backend.NewDefaultSecret(newId, backend.NoValue), nil
}

func (c *ConjurBackend) Delete(ctx context.Context, id ConjurSecretId) error {
	err := c.writeAPI.AddSecret(id.VariableId(), "")
	if err != nil {
		return handleError(err, id)
	}

	return nil
}

func handleError(err error, id ConjurSecretId) error {
	if backend.IsBackendError(err) {
		return err
	}
	cErr, ok := AsError(err)
	if ok {
		if cErr.Code == 404 {
			return backend.ErrSecretNotFound(id)
		}
	}
	return backend.NewBackendError(id, err, "InternalError")
}

func (c *ConjurBackend) initialCreation(ctx context.Context, id ConjurSecretId, value backend.SecretValue) (res backend.DefaultSecret[ConjurSecretId], err error) {
	log := logr.FromContextOrDiscard(ctx)

	log.Info("Secret does not exist yet. Initial creation...", "id", id.VariableId())
	subPath := id.SubPath()
	if subPath == backend.NoSubPath {
		err = c.writeAPI.AddSecret(id.VariableId(), value.Value())
		if err != nil {
			return res, handleError(err, id)
		}
		newId := id.CopyWithChecksum(backend.MakeChecksum(value.Value()))
		log.Info("Successfully created new secret", "id", newId.String())
		res = backend.NewDefaultSecret(newId, backend.NoValue)
	} else {
		log.Info("Subpath found. Using subpath to create secret", "subPath", subPath)
		data, err := c.readAPI.RetrieveSecret(id.VariableId())
		if err != nil {
			return res, handleError(err, id)
		}
		newData, err := sjson.SetBytes(data, subPath, value.Value())
		if err != nil {
			return res, handleError(err, id)
		}
		err = c.writeAPI.AddSecret(id.VariableId(), string(newData))
		if err != nil {
			return res, handleError(err, id)
		}
		newId := id.CopyWithChecksum(backend.MakeChecksum(value.Value()))
		log.Info("Successfully created new secret", "id", newId.String())
		res = backend.NewDefaultSecret(newId, backend.NoValue)
	}

	return res, err
}

// shallowMergeJSON attempts to shallow-merge two JSON object strings.
// If both current and incoming are valid JSON objects, it merges incoming keys
// into current (incoming keys overwrite, existing keys not in incoming are preserved).
// Returns the merged JSON string and true if merging was performed,
// or the incoming value and false if either value is not a JSON object.
func shallowMergeJSON(current, incoming string) (string, bool) {
	var currentMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(current), &currentMap); err != nil {
		// If current is not a JSON object, we cannot merge, so return incoming as is
		return incoming, false
	}
	var incomingMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(incoming), &incomingMap); err != nil {
		// If incoming is not a JSON object, we cannot merge, so return incoming as is
		return incoming, false
	}

	// Shallow merge: incoming keys overwrite, existing keys are preserved
	maps.Copy(currentMap, incomingMap)

	merged, err := json.Marshal(currentMap)
	if err != nil {
		// At this point, both current and incoming were valid JSON objects,
		// so we should not return an error.
		return incoming, false
	}
	return string(merged), true
}
