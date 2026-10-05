// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpspecification_test

import (
	"context"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"github.com/telekom/controlplane/rover/internal/handler/mcpspecification"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newMcpSpecification(basePath string) *roverv1.McpSpecification {
	return &roverv1.McpSpecification{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roverv1.MakeMcpSpecificationName(basePath),
			Namespace: "test-env--grp--team",
			UID:       "spec-uid-1",
		},
		Spec: roverv1.McpSpecificationSpec{
			BasePath:      basePath,
			Version:       "1.0.0",
			Name:          "Test MCP Server",
			Description:   "A test MCP server",
			Specification: "file-id-123",
			Hash:          "spec-hash-123",
			Category:      "other",
			Oauth2Scopes:  []string{"read", "write"},
		},
	}
}

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	Expect(roverv1.AddToScheme(s)).To(Succeed())
	Expect(agenticv1.AddToScheme(s)).To(Succeed())
	return s
}

var _ = Describe("McpSpecificationHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *mcpspecification.McpSpecificationHandler
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &mcpspecification.McpSpecificationHandler{}
		scheme = newScheme()
	})

	Describe("CreateOrUpdate", func() {
		It("should create an McpServer with correct spec fields", func() {
			spec := newMcpSpecification("/mcp/weather/v1")

			fakeClient.EXPECT().Scheme().Return(scheme).Maybe()
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.McpServer"), mock.Anything).
				Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
					Expect(mutate()).To(Succeed())

					server, ok := obj.(*agenticv1.McpServer)
					Expect(ok).To(BeTrue())
					Expect(server.Name).To(Equal("mcp-weather-v1"))
					Expect(server.Spec.BasePath).To(Equal("/mcp/weather/v1"))
					Expect(server.Spec.Version).To(Equal("1.0.0"))
					Expect(server.Spec.Name).To(Equal("Test MCP Server"))
					Expect(server.Spec.Description).To(Equal("A test MCP server"))
					Expect(server.Spec.Specification).To(Equal("file-id-123"))
					Expect(server.Spec.Hash).To(Equal("spec-hash-123"))
					Expect(server.Spec.Category).To(Equal("other"))
					Expect(server.Spec.Oauth2Scopes).To(Equal([]string{"read", "write"}))
					Expect(server.Labels).To(HaveKeyWithValue(
						agenticv1.AgenticBasePathLabelKey,
						labelutil.NormalizeLabelValue("/mcp/weather/v1"),
					))
				}).
				Return(controllerutil.OperationResultCreated, nil)

			fakeClient.EXPECT().AnyChanged().Return(true)

			Expect(h.CreateOrUpdate(ctx, spec)).To(Succeed())
			Expect(spec.Status.McpServer.Name).To(Equal("mcp-weather-v1"))
			Expect(spec.Status.McpServer.Namespace).To(Equal("test-env--grp--team"))
		})
	})
})
