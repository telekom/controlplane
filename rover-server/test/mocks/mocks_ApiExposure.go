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

func NewApiExposureStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*apiv1.ApiExposure] {
	mockStore := NewMockObjectStore[*apiv1.ApiExposure](testing)
	ConfigureApiExposureStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureApiExposureStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*apiv1.ApiExposure]) {
	apiExposure := GetApiExposure(testing, apiExposureFileName)

	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.Anything,
	).Return(
		&store.ListResponse[*apiv1.ApiExposure]{
			Items: []*apiv1.ApiExposure{apiExposure}}, nil).Maybe()

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.Anything,
	).Return(apiExposure, nil).Maybe()
}
