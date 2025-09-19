// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type TeamToken struct {
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Environment  string `json:"environment"`
	GeneratedAt  int64  `json:"generated_at"`
	ServerUrl    string `json:"server_url"`
	TokenUrl     string `json:"token_url"`
}

// EncodeTeamToken generates a token for authorization against the identity service.
func EncodeTeamToken(token TeamToken, group, team string) (string, error) {
	tokenJson, err := json.Marshal(token)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token struct: %w", err)
	}

	tokenPrefix := fmt.Sprintf("%s--%s--%s", token.Environment, group, team)
	return tokenPrefix + "." + base64.StdEncoding.EncodeToString(tokenJson), nil
}

func DecodeTeamToken(stringToken string) (TeamToken, error) {

	split := strings.SplitN(stringToken, ".", 2)
	if len(split) != 2 {
		return TeamToken{}, fmt.Errorf("failed to decode token. `env--group--team.` prefix is not present")
	}

	decoded, err := base64.StdEncoding.DecodeString(split[1])
	if err != nil {
		return TeamToken{}, fmt.Errorf("failed to decode token: %w", err)
	}

	var t TeamToken
	if err := json.Unmarshal(decoded, &t); err != nil {
		return TeamToken{}, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	return t, nil
}
