// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mocks

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func NewApiSpecificationStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*roverv1.ApiSpecification] {
	mockStore := NewMockObjectStore[*roverv1.ApiSpecification](testing)
	ConfigureApiSpecificationStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureApiSpecificationStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.ApiSpecification]) {
	configureApiSpecification(testing, mockedStore)
	configureNotFound(mockedStore)
}

func configureApiSpecification(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.ApiSpecification]) {
	apiSpecification := GetApiSpecification(testing, ApiSpecificationFileName) // apispec-sample

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == "poc--eni--hyperion"
		}),
		mock.MatchedBy(func(s string) bool {
			return s == "apispec-sample"
		}),
	).Return(apiSpecification, nil).Maybe()

	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.Anything,
	).Return(
		&store.ListResponse[*roverv1.ApiSpecification]{
			Items: []*roverv1.ApiSpecification{apiSpecification}}, nil).Maybe()

	mockedStore.EXPECT().Delete(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == "poc--eni--hyperion"
		}),
		mock.MatchedBy(func(s string) bool {
			return s == "apispec-sample"
		}),
	).Return(nil).Maybe()

	mockedStore.EXPECT().CreateOrReplace(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("*v1.ApiSpecification"),
	).Return(nil).Maybe()
}

func configureNotFound(mockedStore *MockObjectStore[*roverv1.ApiSpecification]) {
	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s != "apispec-sample"
		}),
	).Return(nil, problems.NotFound("apispec not found")).Maybe()

	mockedStore.EXPECT().Delete(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s != "apispec-sample"
		}),
	).Return(problems.NotFound("apispec not found")).Maybe()
}
