// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mocks

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/mock"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/store"
)

func NewApplicationStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*applicationv1.Application] {
	mockStore := NewMockObjectStore[*applicationv1.Application](testing)
	ConfigureApplicationStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureApplicationStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*applicationv1.Application]) {
	application := GetApplication(testing, applicationFileName)

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.Anything,
	).Return(application, nil).Maybe()

	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.Anything,
	).Return(
		&store.ListResponse[*applicationv1.Application]{
			Items: []*applicationv1.Application{application}}, nil).Maybe()

	mockedStore.EXPECT().CreateOrReplace(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("*v1.Application"),
	).Return(nil).Maybe()
}
