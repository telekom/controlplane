// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mocks

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/mock"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/store"
)

func NewZoneStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*adminv1.Zone] {
	mockStore := NewMockObjectStore[*adminv1.Zone](testing)
	ConfigureZoneStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureZoneStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*adminv1.Zone]) {
	zone := GetZone(testing, zoneFileName)
	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.Anything,
	).Return(zone, nil).Maybe()
}
