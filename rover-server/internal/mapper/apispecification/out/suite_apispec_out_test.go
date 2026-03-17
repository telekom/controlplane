// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
)

var (
	ctx              context.Context
	apiSpecification *roverv1.ApiSpecification
	openapi          *map[string]any
)

type ContextKey string

func TestMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mapper Suite")
}

var _ = BeforeSuite(func() {
	ctx = context.WithValue(context.TODO(), ContextKey("test"), "test")
	store.ApiStore = mocks.NewApiStoreMock(GinkgoT())

	apiSpecification = mocks.GetApiSpecification(GinkgoT(), mocks.ApiSpecificationFileName)
	openapi = mocks.GetOpenApi(GinkgoT(), mocks.OpenApiFileName)
})
