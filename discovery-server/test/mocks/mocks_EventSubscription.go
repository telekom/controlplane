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

func NewEventSubscriptionStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*eventv1.EventSubscription] {
	mockStore := csmocks.NewMockObjectStore[*eventv1.EventSubscription](testing)
	ConfigureEventSubscriptionStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureEventSubscriptionStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *csmocks.MockObjectStore[*eventv1.EventSubscription]) {
	eventSubscription := GetEventSubscription(testing, eventSubscriptionFileName)

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.Anything,
	).Return(eventSubscription, nil).Maybe()

	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.Anything,
	).Return(
		&store.ListResponse[*eventv1.EventSubscription]{
			Items: []*eventv1.EventSubscription{eventSubscription},
		}, nil).Maybe()
}
