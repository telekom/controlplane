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
	// Client creds flow
	ClientId     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	ClientKey    string `json:"clientKey,omitempty"`
	// Password flow
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	// shared props
	Scopes       string `json:"scopes,omitempty"`
	TokenRequest string `json:"tokenRequest,omitempty"`
	GrantType    string `json:"grantType,omitempty"`
}

type BasicAuthCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
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

type RoutingConfig struct {
	*JumperConfig `json:",inline"`
	RemoteApiUrl  string `json:"remoteApiUrl,omitzero"`
	ApiBasePath   string `json:"apiBasePath,omitzero"`
	Realm         string `json:"realm,omitempty"`
	Environment   string `json:"environment,omitempty"`
	Issuer        string `json:"issuer,omitempty"`
	ClientId      string `json:"clientId,omitempty"`
	ClientSecret  string `json:"clientSecret,omitempty"`
	// TargetZoneName is used to determine if the zone is currently available using zoneHealthCheckService
	TargetZoneName string `json:"targetZoneName,omitempty"`

	TokenEndpoint string `json:"tokenEndpoint,omitempty"`
}

type RoutingConfigs []*RoutingConfig

func (rcs *RoutingConfigs) Add(config *RoutingConfig) {
	*rcs = append(*rcs, config)
}

func (rcs *RoutingConfigs) Get(index int) *RoutingConfig {
	if index < 0 || index >= len(*rcs) {
		return nil
	}
	return (*rcs)[index]
}

func (rcs *RoutingConfigs) Len() int {
	return len(*rcs)
}

type LoadBalancingConfig struct{}

func ToBase64OrDie[T any](cfg T) string {
	b, err := json.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	base64Str := base64.StdEncoding.EncodeToString(b)
	return base64Str
}

func FromBase64[T any](base64Str string) (*T, error) {
	b, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("empty base64 string")
	}

	var cfg *T
	err = json.Unmarshal(b, &cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
