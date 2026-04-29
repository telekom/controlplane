// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"reflect"

	"github.com/telekom/controlplane/identity/pkg/api"
)

func containsAllProtocolMappers(existingClientMappers, newClientMappers *[]api.ProtocolMapperRepresentation) bool {
	if newClientMappers == nil {
		return true
	}
	if existingClientMappers == nil {
		return len(*newClientMappers) == 0
	}
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
	if existingMapper == nil || newMapper == nil {
		return existingMapper == newMapper
	}
	if existingMapper.Name == nil || newMapper.Name == nil ||
		existingMapper.Protocol == nil || newMapper.Protocol == nil ||
		existingMapper.ProtocolMapper == nil || newMapper.ProtocolMapper == nil {
		return false
	}
	return *existingMapper.Name == *newMapper.Name &&
		*existingMapper.Protocol == *newMapper.Protocol &&
		*existingMapper.ProtocolMapper == *newMapper.ProtocolMapper &&
		reflect.DeepEqual(existingMapper.Config, newMapper.Config)
}

func MergeProtocolMappers(existingMappers,
	newMappers *[]api.ProtocolMapperRepresentation) *[]api.ProtocolMapperRepresentation {
	if newMappers == nil {
		return existingMappers
	}
	if existingMappers == nil {
		return newMappers
	}
	for _, mapper := range *newMappers {
		found := false
		for i, existingMapper := range *existingMappers {
			if existingMapper.Name != nil && mapper.Name != nil && *existingMapper.Name == *mapper.Name {
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
