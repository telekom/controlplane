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
	if id.app != "" && id.team == "" {
		return id, backend.ErrInvalidSecretId(raw)
	}

	return id, nil
}

func (id Id) Env() string {
	return id.env
}

func (id Id) String() string {
	return fmt.Sprintf("%s:%s:%s:%s:%s", id.env, id.team, id.app, id.path, id.checksum)
}

func (id Id) Namespace() string {
	if id.app == "" {
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

	if id.app != "" {
		// app secrets
		// name == app-name
		// namespace == env--team
		name = id.app

	} else if id.team != "" {
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

// JsonPath returns the path to the secret in the JSON object
func (id Id) JsonPath() (key string, subPath string) {
	parts := strings.SplitN(id.path, "/", 2)
	if len(parts) == 1 {
		return id.path, ""
	}
	return parts[0], parts[1]
}
