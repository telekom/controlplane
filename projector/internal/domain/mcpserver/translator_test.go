// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpserver_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/telekom/controlplane/projector/internal/domain/mcpserver"
)

var _ = Describe("McpServer Translator", func() {
	var t mcpserver.Translator

	Describe("ShouldSkip", func() {
		It("should never skip", func() {
			obj := &agenticv1.McpServer{}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		It("should populate all fields from the CR", func() {
			obj := &agenticv1.McpServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mcp-weather-v1",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": "prod",
					},
				},
				Spec: agenticv1.McpServerSpec{
					BasePath:      "/mcp/weather/v1",
					Version:       "1.0.0",
					Name:          "weather-server",
					Description:   "Weather MCP server",
					Specification: "file-123",
					Category:      "g-api",
					Oauth2Scopes:  []string{"scope-a", "scope-b"},
				},
				Status: agenticv1.McpServerStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "Ready",
							Status:             metav1.ConditionTrue,
							Reason:             "Ready",
							Message:            "all good",
							LastTransitionTime: metav1.Now(),
						},
					},
					Active: true,
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Meta.Namespace).To(Equal("prod--platform--narvi"))
			Expect(data.Meta.Name).To(Equal("mcp-weather-v1"))
			Expect(data.StatusPhase).To(Equal("READY"))
			Expect(data.BasePath).To(Equal("/mcp/weather/v1"))
			Expect(data.Version).To(Equal("1.0.0"))
			Expect(data.Name).To(Equal("weather-server"))
			Expect(data.Description).To(Equal("Weather MCP server"))
			Expect(data.Specification).To(Equal("file-123"))
			Expect(data.Category).To(Equal("g-api"))
			Expect(data.Oauth2Scopes).To(Equal([]string{"scope-a", "scope-b"}))
			Expect(data.Active).To(BeTrue())
			Expect(data.TeamName).To(Equal("platform--narvi"))
		})

		It("should default Oauth2Scopes to an empty slice when nil", func() {
			obj := &agenticv1.McpServer{
				ObjectMeta: metav1.ObjectMeta{Name: "mcp-a", Namespace: "prod--platform--narvi"},
				Spec: agenticv1.McpServerSpec{
					BasePath: "/mcp/a",
					Version:  "1.0.0",
					Name:     "a",
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Oauth2Scopes).NotTo(BeNil())
			Expect(data.Oauth2Scopes).To(BeEmpty())
		})
	})

	Describe("KeyFromObject", func() {
		It("should derive the composite key from base path and namespace-derived team", func() {
			obj := &agenticv1.McpServer{
				ObjectMeta: metav1.ObjectMeta{Namespace: "prod--platform--narvi"},
				Spec:       agenticv1.McpServerSpec{BasePath: "/mcp/weather/v1"},
			}
			key := t.KeyFromObject(obj)
			Expect(key.BasePath).To(Equal("/mcp/weather/v1"))
			Expect(key.TeamName).To(Equal("platform--narvi"))
		})
	})

	Describe("KeyFromDelete", func() {
		It("should use lastKnown when available", func() {
			lastKnown := &agenticv1.McpServer{
				ObjectMeta: metav1.ObjectMeta{Namespace: "prod--platform--narvi"},
				Spec:       agenticv1.McpServerSpec{BasePath: "/mcp/weather/v1"},
			}
			key, err := t.KeyFromDelete(k8stypes.NamespacedName{Name: "mcp-weather-v1", Namespace: "prod--platform--narvi"}, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.BasePath).To(Equal("/mcp/weather/v1"))
			Expect(key.TeamName).To(Equal("platform--narvi"))
		})

		It("should fall back to req.Name when lastKnown is nil", func() {
			key, err := t.KeyFromDelete(k8stypes.NamespacedName{Name: "mcp-weather-v1", Namespace: "prod--platform--narvi"}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.BasePath).To(Equal("mcp-weather-v1"))
			Expect(key.TeamName).To(Equal("platform--narvi"))
		})
	})
})
