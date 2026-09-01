// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mocks

import (
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func NewFileSpecificationStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*roverv1.FileSpecification] {
	mockStore := NewMockObjectStore[*roverv1.FileSpecification](testing)
	ConfigureFileSpecificationStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureFileSpecificationStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.FileSpecification]) {
	configureFileSpecification(testing, mockedStore)
	configureFileSpecificationNotFound(mockedStore)
}

func configureFileSpecification(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*roverv1.FileSpecification]) {
	fileSpecification := GetFileSpecification(testing, FileSpecificationFileName)

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == "poc--eni--galatea"
		}),
		mock.MatchedBy(func(s string) bool {
			return s == "demo-invoices-v1"
		}),
	).Return(fileSpecification, nil).Maybe()

	// List with a prefix that matches our test data (eni/galatea)
	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(opts store.ListOpts) bool {
			return opts.Prefix != "" && strings.HasPrefix("poc--eni--galatea/", opts.Prefix)
		}),
	).Return(
		&store.ListResponse[*roverv1.FileSpecification]{
			Items: []*roverv1.FileSpecification{fileSpecification}}, nil).Maybe()

	// List with a prefix that does NOT match our test data (e.g., different team)
	mockedStore.EXPECT().List(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(opts store.ListOpts) bool {
			return opts.Prefix != "" && !strings.HasPrefix("poc--eni--galatea/", opts.Prefix)
		}),
	).Return(
		&store.ListResponse[*roverv1.FileSpecification]{
			Items: []*roverv1.FileSpecification{}}, nil).Maybe()

	mockedStore.EXPECT().Delete(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == "poc--eni--galatea"
		}),
		mock.MatchedBy(func(s string) bool {
			return s == "demo-invoices-v1"
		}),
	).Return(nil).Maybe()

	mockedStore.EXPECT().CreateOrReplace(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("*v1.FileSpecification"),
	).Return(nil).Maybe()
}

func configureFileSpecificationNotFound(mockedStore *MockObjectStore[*roverv1.FileSpecification]) {
	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s != "demo-invoices-v1"
		}),
	).Return(nil, problems.NotFound("filespec not found")).Maybe()

	mockedStore.EXPECT().Delete(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.MatchedBy(func(s string) bool {
			return s != "demo-invoices-v1"
		}),
	).Return(problems.NotFound("filespec not found")).Maybe()
}
