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

func NewAPICategoryStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*apiv1.ApiCategory] {
	mockStore := NewMockObjectStore[*apiv1.ApiCategory](testing)
	ConfigureAPICategoryStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureAPICategoryStoreMock(_ ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*apiv1.ApiCategory]) {
	categories := []*apiv1.ApiCategory{
		{Spec: apiv1.ApiCategorySpec{LabelValue: "other", Active: true}},
		{Spec: apiv1.ApiCategorySpec{LabelValue: "test", Active: true}},
		{Spec: apiv1.ApiCategorySpec{LabelValue: "g-api", Active: true}},
		{Spec: apiv1.ApiCategorySpec{LabelValue: "m-api", Active: true}},
		{Spec: apiv1.ApiCategorySpec{LabelValue: "infrastructure", Active: true}},
	}

	mockedStore.EXPECT().List(
		mock.Anything,
		mock.Anything,
	).Return(
		&store.ListResponse[*apiv1.ApiCategory]{Items: categories}, nil).Maybe()
}
