// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mocks

import (
	"encoding/json"

	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"gopkg.in/yaml.v3"

	"github.com/telekom/controlplane/rover-server/test/mocks/data"
)

const (
	ApiSpecificationFileName   = "apiSpecification.json"
	EventSpecificationFileName = "eventSpecification.json"
	EventSubscriptionFileName  = "eventSubscription.json"
	RoadmapFileName            = "roadmap.json"
	OpenApiFileName            = "openapi.yaml"
	apiSubscriptionFileName    = "apiSubscription.json"
	apiExposureFileName        = "apiExposure.json"
	apiFileName                = "api.json"
	applicationFileName        = "application.json"
	RoverFileName              = "rover.json"
	zoneFileName               = "zone.json"
)

func GetRover(testing ginkgo.FullGinkgoTInterface, filePath string) *roverv1.Rover {
	file := data.ReadFile(testing, filePath)
	var rover roverv1.Rover
	err := json.Unmarshal(file, &rover)
	require.NoError(testing, err)

	return &rover
}

func GetApiSubscription(testing ginkgo.FullGinkgoTInterface, filePath string) *apiv1.ApiSubscription {
	file := data.ReadFile(testing, filePath)
	var apiSubscription apiv1.ApiSubscription
	err := json.Unmarshal(file, &apiSubscription)
	require.NoError(testing, err)

	return &apiSubscription
}

func GetApi(testing ginkgo.FullGinkgoTInterface, filePath string) *apiv1.Api {
	file := data.ReadFile(testing, filePath)
	var api apiv1.Api
	err := json.Unmarshal(file, &api)
	require.NoError(testing, err)

	return &api
}

func GetEventSubscription(testing ginkgo.FullGinkgoTInterface, filePath string) *eventv1.EventSubscription {
	file := data.ReadFile(testing, filePath)
	var eventSubscription eventv1.EventSubscription
	err := json.Unmarshal(file, &eventSubscription)
	require.NoError(testing, err)

	return &eventSubscription
}

func GetApiExposure(testing ginkgo.FullGinkgoTInterface, filePath string) *apiv1.ApiExposure {
	file := data.ReadFile(testing, filePath)
	var apiExposure apiv1.ApiExposure
	err := json.Unmarshal(file, &apiExposure)
	require.NoError(testing, err)

	return &apiExposure
}

func GetApplication(testing ginkgo.FullGinkgoTInterface, filePath string) *applicationv1.Application {
	file := data.ReadFile(testing, filePath)
	var application applicationv1.Application
	err := json.Unmarshal(file, &application)
	require.NoError(testing, err)

	return &application
}

func GetZone(testing ginkgo.FullGinkgoTInterface, filePath string) *adminv1.Zone {
	file := data.ReadFile(testing, filePath)
	var zone adminv1.Zone
	err := json.Unmarshal(file, &zone)
	require.NoError(testing, err)

	return &zone
}

func GetApiSpecification(testing ginkgo.FullGinkgoTInterface, filePath string) *roverv1.ApiSpecification {
	file := data.ReadFile(testing, filePath)
	var apiSpecification roverv1.ApiSpecification
	err := json.Unmarshal(file, &apiSpecification)
	require.NoError(testing, err)

	return &apiSpecification
}

func GetEventSpecification(testing ginkgo.FullGinkgoTInterface, filePath string) *roverv1.EventSpecification {
	file := data.ReadFile(testing, filePath)
	var eventSpecification roverv1.EventSpecification
	err := json.Unmarshal(file, &eventSpecification)
	require.NoError(testing, err)

	return &eventSpecification
}

func GetRoadmap(testing ginkgo.FullGinkgoTInterface, filePath string) *roverv1.Roadmap {
	file := data.ReadFile(testing, filePath)
	var roadmap roverv1.Roadmap
	err := json.Unmarshal(file, &roadmap)
	require.NoError(testing, err)

	return &roadmap
}

func GetOpenApi(testing ginkgo.FullGinkgoTInterface, filePath string) *map[string]any {
	file := data.ReadFile(testing, filePath)
	var openapi map[string]any
	err := yaml.Unmarshal(file, &openapi)
	require.NoError(testing, err)

	return &openapi
}
