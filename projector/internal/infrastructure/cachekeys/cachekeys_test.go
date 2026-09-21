// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cachekeys_test

import (
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CacheKeys", func() {

	// --- Zone ---

	Describe("Zone", func() {
		It("returns the correct entity type and lookup key", func() {
			et, lk := cachekeys.Zone("eu-central")
			Expect(et).To(Equal("zone"))
			Expect(lk).To(Equal("eu-central"))
		})

		It("uses the name verbatim as lookup key", func() {
			_, lk := cachekeys.Zone("us-east-1")
			Expect(lk).To(Equal("us-east-1"))
		})
	})

	// --- Group ---

	Describe("Group", func() {
		It("returns the correct entity type and lookup key", func() {
			et, lk := cachekeys.Group("platform")
			Expect(et).To(Equal("group"))
			Expect(lk).To(Equal("platform"))
		})
	})

	// --- Team ---

	Describe("Team", func() {
		It("returns the correct entity type and lookup key", func() {
			et, lk := cachekeys.Team("hyperion")
			Expect(et).To(Equal("team"))
			Expect(lk).To(Equal("hyperion"))
		})
	})

	// --- Application ---

	Describe("Application", func() {
		It("returns the correct entity type and composite key", func() {
			et, lk := cachekeys.Application("my-app", "hyperion")
			Expect(et).To(Equal("application"))
			Expect(lk).To(Equal("my-app:hyperion"))
		})

		It("produces different keys for different teams", func() {
			_, lk1 := cachekeys.Application("app", "team-a")
			_, lk2 := cachekeys.Application("app", "team-b")
			Expect(lk1).NotTo(Equal(lk2))
		})

		It("produces different keys for different app names", func() {
			_, lk1 := cachekeys.Application("app-1", "team")
			_, lk2 := cachekeys.Application("app-2", "team")
			Expect(lk1).NotTo(Equal(lk2))
		})
	})

	// --- ApiExposure ---

	Describe("ApiExposure", func() {
		It("returns the correct entity type and composite key", func() {
			et, lk := cachekeys.APIExposure("/api/v1", "my-app", "hyperion")
			Expect(et).To(Equal("apiexposure"))
			Expect(lk).To(Equal("/api/v1:my-app:hyperion"))
		})

		It("produces different keys for different base paths", func() {
			_, lk1 := cachekeys.APIExposure("/api/v1", "app", "team")
			_, lk2 := cachekeys.APIExposure("/api/v2", "app", "team")
			Expect(lk1).NotTo(Equal(lk2))
		})

		It("produces different keys for different apps", func() {
			_, lk1 := cachekeys.APIExposure("/api/v1", "app-a", "team")
			_, lk2 := cachekeys.APIExposure("/api/v1", "app-b", "team")
			Expect(lk1).NotTo(Equal(lk2))
		})
	})

	// --- ApiExposureByBasePath ---

	Describe("ApiExposureByBasePath", func() {
		It("returns apiexposure entity type with bp: prefix", func() {
			et, lk := cachekeys.APIExposureByBasePath("/api/v1")
			Expect(et).To(Equal("apiexposure"))
			Expect(lk).To(Equal("bp:/api/v1"))
		})

		It("does not collide with full ApiExposure key", func() {
			_, lk1 := cachekeys.APIExposureByBasePath("/api/v1")
			_, lk2 := cachekeys.APIExposure("/api/v1", "app", "team")
			Expect(lk1).NotTo(Equal(lk2))
		})
	})

	// --- ApiSubscriptionMeta ---

	Describe("ApiSubscriptionMeta", func() {
		It("returns the correct entity type and meta-prefixed key", func() {
			et, lk := cachekeys.APISubscriptionMeta("default", "my-sub")
			Expect(et).To(Equal("apisubscription"))
			Expect(lk).To(Equal("meta:default:my-sub"))
		})

		It("produces different keys for different namespaces", func() {
			_, lk1 := cachekeys.APISubscriptionMeta("ns-a", "sub")
			_, lk2 := cachekeys.APISubscriptionMeta("ns-b", "sub")
			Expect(lk1).NotTo(Equal(lk2))
		})
	})

	// --- Approval ---

	Describe("Approval", func() {
		It("returns the correct entity type and composite key", func() {
			et, lk := cachekeys.Approval("default", "my-approval")
			Expect(et).To(Equal("approval"))
			Expect(lk).To(Equal("default:my-approval"))
		})

		It("produces different keys for different namespaces", func() {
			_, lk1 := cachekeys.Approval("ns-a", "approval")
			_, lk2 := cachekeys.Approval("ns-b", "approval")
			Expect(lk1).NotTo(Equal(lk2))
		})
	})

	// --- ApprovalRequest ---

	Describe("ApprovalRequest", func() {
		It("returns the correct entity type and composite key", func() {
			et, lk := cachekeys.ApprovalRequest("default", "my-ar")
			Expect(et).To(Equal("approvalrequest"))
			Expect(lk).To(Equal("default:my-ar"))
		})

		It("does not collide with Approval keys", func() {
			_, lkApproval := cachekeys.Approval("ns", "name")
			etApproval, _ := cachekeys.Approval("ns", "name")
			etAR, _ := cachekeys.ApprovalRequest("ns", "name")
			// Same lookup key format but different entity types prevent collision.
			Expect(etApproval).NotTo(Equal(etAR))
			_ = lkApproval // suppress unused
		})
	})

	// --- McpServer ---

	Describe("McpServer", func() {
		It("returns the correct entity type and composite key", func() {
			et, lk := cachekeys.McpServer("/mcp/v1", "hyperion")
			Expect(et).To(Equal("mcpserver"))
			Expect(lk).To(Equal("/mcp/v1:hyperion"))
		})

		It("produces different keys for different teams", func() {
			_, lk1 := cachekeys.McpServer("/mcp/v1", "team-a")
			_, lk2 := cachekeys.McpServer("/mcp/v1", "team-b")
			Expect(lk1).NotTo(Equal(lk2))
		})
	})

	// --- ActiveMcpServer ---

	Describe("ActiveMcpServer", func() {
		It("returns the correct entity type and lookup key", func() {
			et, lk := cachekeys.ActiveMcpServer("/mcp/v1")
			Expect(et).To(Equal("mcpserver_active"))
			Expect(lk).To(Equal("/mcp/v1"))
		})

		It("does not collide with McpServer key", func() {
			etActive, _ := cachekeys.ActiveMcpServer("/mcp/v1")
			etFull, _ := cachekeys.McpServer("/mcp/v1", "team")
			Expect(etActive).NotTo(Equal(etFull))
		})
	})

	// --- AgentCard ---

	Describe("AgentCard", func() {
		It("returns the correct entity type and composite key", func() {
			et, lk := cachekeys.AgentCard("/agent/v1", "hyperion")
			Expect(et).To(Equal("agentcard"))
			Expect(lk).To(Equal("/agent/v1:hyperion"))
		})

		It("produces different keys for different teams", func() {
			_, lk1 := cachekeys.AgentCard("/agent/v1", "team-a")
			_, lk2 := cachekeys.AgentCard("/agent/v1", "team-b")
			Expect(lk1).NotTo(Equal(lk2))
		})
	})

	// --- ActiveAgentCard ---

	Describe("ActiveAgentCard", func() {
		It("returns the correct entity type and lookup key", func() {
			et, lk := cachekeys.ActiveAgentCard("/agent/v1")
			Expect(et).To(Equal("agentcard_active"))
			Expect(lk).To(Equal("/agent/v1"))
		})

		It("does not collide with AgentCard key", func() {
			etActive, _ := cachekeys.ActiveAgentCard("/agent/v1")
			etFull, _ := cachekeys.AgentCard("/agent/v1", "team")
			Expect(etActive).NotTo(Equal(etFull))
		})
	})

	// --- AgenticExposure ---

	Describe("AgenticExposure", func() {
		It("returns the correct entity type and composite key", func() {
			et, lk := cachekeys.AgenticExposure("/mcp/v1", "my-app", "hyperion")
			Expect(et).To(Equal("agenticexposure"))
			Expect(lk).To(Equal("/mcp/v1:my-app:hyperion"))
		})

		It("produces different keys for different base paths", func() {
			_, lk1 := cachekeys.AgenticExposure("/mcp/v1", "app", "team")
			_, lk2 := cachekeys.AgenticExposure("/mcp/v2", "app", "team")
			Expect(lk1).NotTo(Equal(lk2))
		})

		It("produces different keys for different apps", func() {
			_, lk1 := cachekeys.AgenticExposure("/mcp/v1", "app-a", "team")
			_, lk2 := cachekeys.AgenticExposure("/mcp/v1", "app-b", "team")
			Expect(lk1).NotTo(Equal(lk2))
		})
	})

	// --- AgenticExposureByBasePath ---

	Describe("AgenticExposureByBasePath", func() {
		It("returns agenticexposure entity type with bp: prefix", func() {
			et, lk := cachekeys.AgenticExposureByBasePath("/mcp/v1")
			Expect(et).To(Equal("agenticexposure"))
			Expect(lk).To(Equal("bp:/mcp/v1"))
		})

		It("does not collide with full AgenticExposure key", func() {
			_, lk1 := cachekeys.AgenticExposureByBasePath("/mcp/v1")
			_, lk2 := cachekeys.AgenticExposure("/mcp/v1", "app", "team")
			Expect(lk1).NotTo(Equal(lk2))
		})
	})

	// --- AgenticSubscriptionMeta ---

	Describe("AgenticSubscriptionMeta", func() {
		It("returns the correct entity type and meta-prefixed key", func() {
			et, lk := cachekeys.AgenticSubscriptionMeta("default", "my-sub")
			Expect(et).To(Equal("agenticsubscription"))
			Expect(lk).To(Equal("meta:default:my-sub"))
		})

		It("produces different keys for different namespaces", func() {
			_, lk1 := cachekeys.AgenticSubscriptionMeta("ns-a", "sub")
			_, lk2 := cachekeys.AgenticSubscriptionMeta("ns-b", "sub")
			Expect(lk1).NotTo(Equal(lk2))
		})
	})

	// --- Edge cases ---

	Describe("Edge cases", func() {
		It("handles empty strings for Zone", func() {
			et, lk := cachekeys.Zone("")
			Expect(et).To(Equal("zone"))
			Expect(lk).To(Equal(""))
		})

		It("handles empty strings for Application", func() {
			et, lk := cachekeys.Application("", "")
			Expect(et).To(Equal("application"))
			Expect(lk).To(Equal(":"))
		})

		It("handles special characters in names", func() {
			_, lk := cachekeys.Application("app/with:special", "team@name")
			Expect(lk).To(Equal("app/with:special:team@name"))
		})

		It("handles empty strings for ApiSubscriptionMeta", func() {
			_, lk := cachekeys.APISubscriptionMeta("", "")
			Expect(lk).To(Equal("meta::"))
		})

		It("handles empty strings for ApiExposureByBasePath", func() {
			_, lk := cachekeys.APIExposureByBasePath("")
			Expect(lk).To(Equal("bp:"))
		})
	})
})
