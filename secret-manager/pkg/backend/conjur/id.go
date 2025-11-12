// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur

import (
	"os"
	"strings"

	"github.com/telekom/controlplane/secret-manager/pkg/backend"
)

var RootPolicyPath = ""

func init() {
	rpp := os.Getenv("CONJUR_ROOT_POLICY_PATH")
	if rpp != "" {
		RootPolicyPath = clean(rpp)
	}
}

var _ backend.SecretId = ConjurSecretId{}

type ConjurSecretId struct {
	Raw      string
	env      string
	team     string
	app      string
	path     string
	checksum string
}

func Copy(c ConjurSecretId) ConjurSecretId {
	return c
}

func New(env, team, app, path string, checksum string) ConjurSecretId {
	raw := strings.Join([]string{env, team, app, path, checksum}, backend.Separator)
	return ConjurSecretId{
		Raw:      raw,
		env:      env,
		team:     team,
		app:      app,
		path:     path,
		checksum: checksum,
	}
}

func FromString(raw string) (id ConjurSecretId, err error) {
	parts := strings.Split(raw, backend.Separator)
	if len(parts) != 5 {
		return id, backend.ErrInvalidSecretId(raw)
	}

	id = ConjurSecretId{
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

func (c ConjurSecretId) Env() string {
	return c.env
}

func (c ConjurSecretId) String() string {
	return c.Raw
}

func (c ConjurSecretId) VariableId() string {
	// We just get the first part of the path as this is the secret-name
	// The other parts are the subpath
	path := strings.SplitN(c.path, "/", 2)[0]
	if c.team == "" {
		str := strings.Join([]string{RootPolicyPath, c.env, path}, "/")
		return clean(str)
	}
	str := strings.Join([]string{RootPolicyPath, c.env, c.team, c.app, path}, "/")
	return clean(str)
}

func (c ConjurSecretId) SubPath() string {
	return backend.GetSubPath(c.path)
}

func (c ConjurSecretId) Path() string {
	return backend.GetPath(c.path)
}

func (c ConjurSecretId) CopyWithChecksum(checksum string) ConjurSecretId {
	new := Copy(c)
	new.checksum = checksum
	new.Raw = strings.Join([]string{new.env, new.team, new.app, new.path, new.checksum}, backend.Separator)
	return new
}

func (c ConjurSecretId) Copy() backend.SecretId {
	return Copy(c)
}

func clean(id string) string {
	id = strings.TrimSpace(id)
	id = strings.TrimPrefix(id, "/")
	return strings.ReplaceAll(id, "//", "/")
}
