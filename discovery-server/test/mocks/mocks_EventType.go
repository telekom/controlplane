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

func NewEventTypeStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*eventv1.EventType] {
	mockStore := csmocks.NewMockObjectStore[*eventv1.EventType](testing)
	ConfigureEventTypeStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureEventTypeStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *csmocks.MockObjectStore[*eventv1.EventType]) {
	eventType := GetEventType(testing, eventTypeFileName)

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.Anything,
	).Return(eventType, nil).Maybe()

	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.Anything,
	).Return(
		&store.ListResponse[*eventv1.EventType]{
			Items: []*eventv1.EventType{eventType},
		}, nil).Maybe()
}
