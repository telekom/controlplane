// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identity_client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	organisationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/handler/team/handler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const TeamNameSuffix = "team-user"

type token struct {
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Environment  string `json:"environment"`
	GeneratedAt  int64  `json:"generated_at"`
}

func buildIdentityClientObj(owner *organisationv1.Team) *identityv1.Client {
	name := owner.Spec.Group + handler.Separator + owner.Spec.Name + handler.Separator + TeamNameSuffix
	return &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: owner.Status.Namespace,
		},
	}
}

// buildToken generates a token for authorization against the identity service.
func buildToken(clientId, clientSecret, env string, t time.Time) (string, error) {
	tokenStruct := token{
		ClientId:     clientId,
		ClientSecret: clientSecret,
		Environment:  env,
		GeneratedAt:  t.Unix(),
	}

	tokenJson, err := json.Marshal(tokenStruct)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token struct: %w", err)
	}

	return base64.StdEncoding.EncodeToString(tokenJson), nil
}

func decodeToken(stringToken string) (token, error) {
	decoded, err := base64.StdEncoding.DecodeString(stringToken)
	if err != nil {
		return token{}, fmt.Errorf("failed to decode token: %w", err)
	}

	var t token
	if err := json.Unmarshal(decoded, &t); err != nil {
		return token{}, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	return t, nil
}
