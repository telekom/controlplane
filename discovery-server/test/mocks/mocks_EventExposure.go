// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mocks

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common-server/pkg/store"
	csmocks "github.com/telekom/controlplane/common-server/test/mocks"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
)

func NewEventExposureStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*eventv1.EventExposure] {
	mockStore := csmocks.NewMockObjectStore[*eventv1.EventExposure](testing)
	ConfigureEventExposureStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureEventExposureStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *csmocks.MockObjectStore[*eventv1.EventExposure]) {
	eventExposure := GetEventExposure(testing, eventExposureFileName)

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.Anything,
	).Return(eventExposure, nil).Maybe()

	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.Anything,
	).Return(
		&store.ListResponse[*eventv1.EventExposure]{
			Items: []*eventv1.EventExposure{eventExposure}}, nil).Maybe()
}
