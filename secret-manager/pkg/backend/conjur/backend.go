// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
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
}

func NewBackend(writeAPI, readAPI ConjurAPI) backend.Backend[ConjurSecretId, backend.DefaultSecret[ConjurSecretId]] {
	return &ConjurBackend{
		writeAPI:          writeAPI,
		readAPI:           readAPI,
		MustMatchChecksum: false,
	}
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
	if subPath != "" {
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

	if id.checksum != "" && id.checksum != res.Id().checksum {
		if c.MustMatchChecksum {
			log.Info("Checksum mismatch. Returning error", "id", id.String())
			return res, backend.ErrBadChecksum(id)

		}
		log.Info("Checksum mismatch but its ignored. Returning secret", "id", id.String())
		return res, nil
	}

	return res, nil
}

func (c *ConjurBackend) Set(ctx context.Context, id ConjurSecretId, secretValue backend.SecretValue) (res backend.DefaultSecret[ConjurSecretId], err error) {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Setting secret", "id", id.VariableId())
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
	if subPath != "" {
		result := gjson.GetBytes(data, subPath)
		if !result.Exists() {
			currentValue = ""
		} else {
			currentValue = result.String()
		}
	}

	if currentValue != "" && !secretValue.AllowChange() {
		log.Info("Secret already exists but is not empty. Not updating...", "id", id.String())
		return backend.NewDefaultSecret(id.CopyWithChecksum(backend.MakeChecksum(currentValue)), currentValue), nil
	}

	if secretValue.EqualString(currentValue) {
		log.Info("Secret already exists and is up to date", "id", id.String())
		return backend.NewDefaultSecret(id, currentValue), nil
	}

	nextValue := secretValue.Value()
	if subPath != "" {
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
	newId := id.CopyWithChecksum(backend.MakeChecksum(secretValue.Value()))
	return backend.NewDefaultSecret(newId, ""), nil
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
	if subPath == "" {
		err = c.writeAPI.AddSecret(id.VariableId(), value.Value())
		if err != nil {
			return res, handleError(err, id)
		}
		newId := id.CopyWithChecksum(backend.MakeChecksum(value.Value()))
		log.Info("Successfully created new secret", "id", newId.String())
		res = backend.NewDefaultSecret(newId, "")
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
		res = backend.NewDefaultSecret(newId, "")
	}

	return res, err
}
