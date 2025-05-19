// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur

import (
	"io"
	"os"

	"github.com/pkg/errors"

	"github.com/cyberark/conjur-api-go/conjurapi"
	"github.com/cyberark/conjur-api-go/conjurapi/authn"
)

type ConjurAPI interface {
	LoadPolicy(mode conjurapi.PolicyMode, path string, reader io.Reader) (*conjurapi.PolicyResponse, error)
	RetrieveSecret(variableID string) ([]byte, error)
	AddSecret(variableID, value string) error
	RetrieveBatchSecrets(variableIDs []string) (map[string][]byte, error)
	RetrieveSecretWithVersion(variableID string, version int) ([]byte, error)
}

func NewReadOnlyApiOrDie() ConjurAPI {
	api, err := NewApi(true)
	if err != nil {
		panic(errors.Wrap(err, "failed to create read-only API"))
	}
	return api
}

func NewWriteApiOrDie() ConjurAPI {
	api, err := NewApi(false)
	if err != nil {
		panic(errors.Wrap(err, "failed to create write API"))
	}
	return api
}

func NewApi(readyOnly bool) (ConjurAPI, error) {
	config, err := conjurapi.LoadConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load config")
	}
	if readyOnly {
		followerUrl := os.Getenv("CONJUR_FOLLOWER_URL")
		if followerUrl != "" {
			config.ApplianceURL = followerUrl
		}
	}

	apiKey := os.Getenv("CONJUR_AUTHN_API_KEY")
	if apiKey != "" {
		conjur, err := conjurapi.NewClientFromKey(config,
			authn.LoginPair{
				Login:  os.Getenv("CONJUR_AUTHN_LOGIN"),
				APIKey: apiKey,
			},
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create client using api-key")
		}
		return conjur, nil
	}

	conjur, err := conjurapi.NewClientFromEnvironment(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create client using token-authenticator")
	}

	return conjur, nil
}
