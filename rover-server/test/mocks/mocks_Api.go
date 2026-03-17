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

func NewApiStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*apiv1.Api] {
	mockStore := NewMockObjectStore[*apiv1.Api](testing)
	ConfigureApiStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureApiStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*apiv1.Api]) {
	api := GetApi(testing, apiFileName)
	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.Anything,
	).Return(
		&store.ListResponse[*apiv1.Api]{
			Items: []*apiv1.Api{api}}, nil).Maybe()

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.Anything,
	).Return(api, nil).Maybe()
}
