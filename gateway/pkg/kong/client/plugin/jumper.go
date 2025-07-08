// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"encoding/base64"
	"encoding/json"

	"github.com/pkg/errors"
)

const LocalhostProxyUrl = "http://localhost:8080/proxy"

type ConsumerId string

type OauthCredentials struct {
	ClientId     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Scopes       string `json:"scopes,omitempty"`
	TokenRequest string `json:"tokenRequest,omitempty"`
	GrantType    string `json:"grantType,omitempty"`
}

type BasicAuthCredentials struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	GrantType string `json:"grantType,omitempty"`
}

type LoadBalancing struct {
	Servers []LoadBalancingServer `json:"servers"`
}

type LoadBalancingServer struct {
	Upstream string `json:"upstream"`
	Weight   int    `json:"weight,omitempty"`
}

type JumperConfig struct {
	OAuth         map[ConsumerId]OauthCredentials     `json:"oauth,omitempty"`
	BasicAuth     map[ConsumerId]BasicAuthCredentials `json:"basicAuth,omitempty"`
	LoadBalancing *LoadBalancing                      `json:"loadBalancing,omitempty"`
}

func NewJumperConfig() *JumperConfig {
	return &JumperConfig{
		OAuth:     map[ConsumerId]OauthCredentials{},
		BasicAuth: map[ConsumerId]BasicAuthCredentials{},
	}
}

func ToBase64OrDie(cfg *JumperConfig) string {
	b, err := json.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	base64Str := base64.StdEncoding.EncodeToString(b)
	return base64Str
}

func FromBase64(base64Str string) (*JumperConfig, error) {
	b, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("empty base64 string")
	}

	var cfg *JumperConfig
	err = json.Unmarshal(b, &cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
