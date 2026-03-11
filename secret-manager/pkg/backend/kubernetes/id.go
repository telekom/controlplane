// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"fmt"
	"strings"

	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ backend.SecretId = Id{}

func Copy(id Id) Id {
	return id
}

type Id struct {
	Raw      string
	env      string
	team     string
	app      string
	path     string
	checksum string
}

func New(env, team, app, path string, checksum string) Id {
	raw := strings.Join([]string{env, team, app, path, checksum}, backend.Separator)
	return Id{
		Raw:      raw,
		env:      env,
		team:     team,
		app:      app,
		path:     path,
		checksum: checksum,
	}
}

func FromString(raw string) (id Id, err error) {
	parts := strings.Split(raw, backend.Separator)
	if len(parts) != 5 {
		return id, backend.ErrInvalidSecretId(raw)
	}

	id = Id{
		Raw:      raw,
		env:      parts[0],
		team:     parts[1],
		app:      parts[2],
		path:     parts[3],
		checksum: parts[4],
	}

	if id.env == "" {
		return id, backend.ErrInvalidSecretId(raw)
	}
	if id.app != backend.NoApp && id.team == backend.NoTeam {
		return id, backend.ErrInvalidSecretId(raw)
	}

	return id, nil
}

func (id Id) Env() string {
	return id.env
}

func (id Id) String() string {
	return strings.Join([]string{id.env, id.team, id.app, id.path, id.checksum}, ":")
}

func (id Id) Namespace() string {
	if id.app == backend.NoApp {
		// if app is empty, this must be an env or team secrets
		// These are located in the env namespace
		return id.env
	}
	return fmt.Sprintf("%s--%s", id.env, id.team)
}

func (id Id) ObjectKey() client.ObjectKey {
	// env secret
	// namespace == env
	name := "secrets"

	if id.app != backend.NoApp {
		// app secrets
		// name == app-name
		// namespace == env--team
		name = id.app

	} else if id.team != backend.NoTeam {
		// team secrets
		// name == team-name
		// namespace == env
		name = id.team
	}

	return client.ObjectKey{
		Name:      name,
		Namespace: id.Namespace(),
	}
}

func (id Id) CopyWithChecksum(resourceId string) Id {
	new := Copy(id)
	new.checksum = resourceId
	return new
}

func (id Id) Copy() backend.SecretId {
	return Copy(id)
}

// CacheKey returns a stable cache key without the checksum.
// This ensures the same logical secret always maps to the same cache entry
// regardless of value changes.
func (id Id) CacheKey() string {
	return strings.Join([]string{id.env, id.team, id.app, id.path}, backend.Separator)
}

func (id Id) ParentId() backend.SecretId {
	parentPath := id.Path()
	if parentPath == "" {
		return id
	}
	parentId := Copy(id)
	parentId.path = parentPath
	parentId.Raw = strings.Join([]string{parentId.env, parentId.team, parentId.app, parentId.path, backend.NoChecksum}, backend.Separator)
	return parentId
}

func (id Id) SubPath() string {
	return backend.GetSubPath(id.path)
}

func (id Id) Path() string {
	return backend.GetPath(id.path)
}
