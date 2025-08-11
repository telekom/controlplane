// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mocks

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/mock"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/store"
)

func NewApiSubscriptionStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*apiv1.ApiSubscription] {
	mockStore := NewMockObjectStore[*apiv1.ApiSubscription](testing)
	ConfigureApiSubscriptionStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureApiSubscriptionStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*apiv1.ApiSubscription]) {
	apiSubscription := GetApiSubscription(testing, apiSubscriptionFileName)
	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.Anything,
	).Return(
		&store.ListResponse[*apiv1.ApiSubscription]{
			Items: []*apiv1.ApiSubscription{apiSubscription}}, nil).Maybe()

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.Anything,
	).Return(apiSubscription, nil).Maybe()
}
