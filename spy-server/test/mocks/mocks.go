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
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	eventv1 "github.com/telekom/controlplane/event/api/v1"

	"github.com/telekom/controlplane/spy-server/test/mocks/data"
)

const (
	applicationFileName       = "application.json"
	apiExposureFileName       = "apiExposure.json"
	apiSubscriptionFileName   = "apiSubscription.json"
	zoneFileName              = "zone.json"
	approvalFileName          = "approval.json"
	eventExposureFileName     = "eventExposure.json"
	eventSubscriptionFileName = "eventSubscription.json"
	eventTypeFileName         = "eventType.json"
)

func GetApplication(testing ginkgo.FullGinkgoTInterface, filePath string) *applicationv1.Application {
	file := data.ReadFile(testing, filePath)
	var obj applicationv1.Application
	err := json.Unmarshal(file, &obj)
	require.NoError(testing, err)
	return &obj
}

func GetApiExposure(testing ginkgo.FullGinkgoTInterface, filePath string) *apiv1.ApiExposure {
	file := data.ReadFile(testing, filePath)
	var obj apiv1.ApiExposure
	err := json.Unmarshal(file, &obj)
	require.NoError(testing, err)
	return &obj
}

func GetApiSubscription(testing ginkgo.FullGinkgoTInterface, filePath string) *apiv1.ApiSubscription {
	file := data.ReadFile(testing, filePath)
	var obj apiv1.ApiSubscription
	err := json.Unmarshal(file, &obj)
	require.NoError(testing, err)
	return &obj
}

func GetZone(testing ginkgo.FullGinkgoTInterface, filePath string) *adminv1.Zone {
	file := data.ReadFile(testing, filePath)
	var obj adminv1.Zone
	err := json.Unmarshal(file, &obj)
	require.NoError(testing, err)
	return &obj
}

func GetApproval(testing ginkgo.FullGinkgoTInterface, filePath string) *approvalv1.Approval {
	file := data.ReadFile(testing, filePath)
	var obj approvalv1.Approval
	err := json.Unmarshal(file, &obj)
	require.NoError(testing, err)
	return &obj
}

func GetEventExposure(testing ginkgo.FullGinkgoTInterface, filePath string) *eventv1.EventExposure {
	file := data.ReadFile(testing, filePath)
	var obj eventv1.EventExposure
	err := json.Unmarshal(file, &obj)
	require.NoError(testing, err)
	return &obj
}

func GetEventSubscription(testing ginkgo.FullGinkgoTInterface, filePath string) *eventv1.EventSubscription {
	file := data.ReadFile(testing, filePath)
	var obj eventv1.EventSubscription
	err := json.Unmarshal(file, &obj)
	require.NoError(testing, err)
	return &obj
}

func GetEventType(testing ginkgo.FullGinkgoTInterface, filePath string) *eventv1.EventType {
	file := data.ReadFile(testing, filePath)
	var obj eventv1.EventType
	err := json.Unmarshal(file, &obj)
	require.NoError(testing, err)
	return &obj
}
