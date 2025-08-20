// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package accesstoken

import "github.com/pkg/errors"

var _ AccessToken = (*StaticAccessToken)(nil)

type StaticAccessToken struct {
	Token string `yaml:"token" json:"token"`
}

func NewStaticAccessToken(token string) *StaticAccessToken {
	return &StaticAccessToken{
		Token: token,
	}
}

func (s *StaticAccessToken) Read() (string, error) {
	if s.Token == "" {
		return "", errors.New("static access token is empty")
	}
	return s.Token, nil
}
