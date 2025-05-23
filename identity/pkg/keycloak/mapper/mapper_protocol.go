// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"reflect"

	"k8s.io/utils/ptr"

	"github.com/telekom/controlplane/identity/pkg/api"
)

func MapToProtocolMapperRepresentation() api.ProtocolMapperRepresentation {
	return api.ProtocolMapperRepresentation{
		Name:           ptr.To("Client ID"),
		Protocol:       ptr.To("openid-connect"),
		ProtocolMapper: ptr.To("oidc-usersessionmodel-note-mapper"),
		Config: &map[string]interface{}{
			"user.session.note":    "clientId",
			"id.token.claim":       "true",
			"access.token.claim":   "true",
			"userinfo.token.claim": "false",
			"claim.name":           "clientId",
			"jsonType.label":       "String",
		},
	}
}

func containsAllProtocolMappers(existingClientMappers, newClientMappers *[]api.ProtocolMapperRepresentation) bool {
	for _, mapper2 := range *newClientMappers {
		found := false
		for _, mapper1 := range *existingClientMappers {
			if CompareProtocolMapperRepresentation(&mapper1, &mapper2) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func CompareProtocolMapperRepresentation(existingMapper, newMapper *api.ProtocolMapperRepresentation) bool {
	return *existingMapper.Name == *newMapper.Name &&
		*existingMapper.Protocol == *newMapper.Protocol &&
		*existingMapper.ProtocolMapper == *newMapper.ProtocolMapper &&
		reflect.DeepEqual(existingMapper.Config, newMapper.Config)
}

func MergeProtocolMappers(existingMappers,
	newMappers *[]api.ProtocolMapperRepresentation) *[]api.ProtocolMapperRepresentation {
	for _, mapper := range *newMappers {
		found := false
		for i, existingMapper := range *existingMappers {
			if *existingMapper.Name == *mapper.Name {
				(*existingMappers)[i] = *MergeProtocolMapperRepresentation(&existingMapper, &mapper)
				found = true
				break
			}
		}
		if !found {
			*existingMappers = append(*existingMappers, mapper)
		}
	}

	return existingMappers
}

func MergeProtocolMapperRepresentation(existingMapper,
	newMapper *api.ProtocolMapperRepresentation) *api.ProtocolMapperRepresentation {
	// ID stays the same
	existingMapper.Name = newMapper.Name
	existingMapper.Protocol = newMapper.Protocol
	existingMapper.ProtocolMapper = newMapper.ProtocolMapper
	existingMapper.Config = newMapper.Config

	return existingMapper
}
