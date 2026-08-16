// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/internal/api"
	roverstore "github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Resource pagination", func() {
	It("round trips an aggregate cursor", func() {
		cursor := resourceCursor{Version: 1, Kind: 5, Cursor: "next:/?team=grüße;offset=7"}

		encoded, err := encodeResourceCursor(cursor)
		Expect(err).NotTo(HaveOccurred())
		Expect(encoded).To(Equal("eyJ2IjoxLCJrIjo1LCJjIjoibmV4dDovP3RlYW09Z3LDvMOfZTtvZmZzZXQ9NyJ9"))

		decoded, err := decodeResourceCursor(encoded)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded).To(Equal(cursor))
	})

	DescribeTable("rejects invalid aggregate cursors",
		func(cursor string) {
			_, err := decodeResourceCursor(cursor)
			Expect(err).To(HaveOccurred())
			var problem problems.Problem
			Expect(errors.As(err, &problem)).To(BeTrue())
			Expect(problem.Code()).To(Equal(http.StatusBadRequest))
		},
		Entry("invalid base64", "%%%"),
		Entry("invalid JSON", encodeJSON(`{`)),
		Entry("unknown version", encodeJSON(`{"v":2,"k":0,"c":""}`)),
		Entry("negative kind", encodeJSON(`{"v":1,"k":-1,"c":""}`)),
		Entry("kind past final store", encodeJSON(`{"v":1,"k":6,"c":""}`)),
	)

	It("shares one limit across resource kinds", func() {
		fixture := newResourceFixture()
		fixture.rovers = page(rover("rover"))
		fixture.apiSpecifications = page(apiSpecification("api"))
		fixture.mcpSpecifications = page(mcpSpecification("mcp"))

		response, err := fixture.controller.GetAll(teamContext(), resourceParams(2, ""))

		Expect(err).NotTo(HaveOccurred())
		Expect(response.Items).To(HaveLen(2))
		Expect(response.Items[0].Kind).To(Equal("Rover"))
		Expect(response.Items[1].Kind).To(Equal("ApiSpecification"))
		Expect(response.UnderscoreLinks.Next).NotTo(BeEmpty())
		fixture.expectCalls(
			listCall{kind: "Rover", limit: 2},
			listCall{kind: "ApiSpecification", limit: 1},
			listCall{kind: "EventSpecification", limit: 1},
			listCall{kind: "Roadmap", limit: 1},
			listCall{kind: "ApiChangelog", limit: 1},
			listCall{kind: "McpSpecification", limit: 1},
		)
	})

	It("continues within the same store without duplicates", func() {
		fixture := newResourceFixture()
		fixture.rovers = func(opts store.ListOpts) (*store.ListResponse[*roverv1.Rover], error) {
			if opts.Cursor == "" {
				return pageWithLinks("first", "second", rover("first")), nil
			}
			return pageWithLinks("second", "", rover("second")), nil
		}

		first, err := fixture.controller.GetAll(teamContext(), resourceParams(1, ""))
		Expect(err).NotTo(HaveOccurred())
		second, err := fixture.controller.GetAll(teamContext(), resourceParams(1, first.UnderscoreLinks.Next))

		Expect(err).NotTo(HaveOccurred())
		Expect(first.Items).To(ConsistOf(HaveField("Name", "first")))
		Expect(second.Items).To(ConsistOf(HaveField("Name", "second")))
		Expect(second.UnderscoreLinks.Self).To(Equal(first.UnderscoreLinks.Next))
		Expect(fixture.calls[1]).To(Equal(listCall{kind: "Rover", limit: 1, cursor: "second"}))
	})

	It("crosses empty stores", func() {
		fixture := newResourceFixture()
		fixture.apiSpecifications = page(apiSpecification("api"))

		response, err := fixture.controller.GetAll(teamContext(), resourceParams(1, ""))

		Expect(err).NotTo(HaveOccurred())
		Expect(response.Items).To(ConsistOf(HaveField("Kind", "ApiSpecification")))
		fixture.expectCalls(
			listCall{kind: "Rover", limit: 1},
			listCall{kind: "ApiSpecification", limit: 1},
			listCall{kind: "EventSpecification", limit: 1},
			listCall{kind: "Roadmap", limit: 1},
			listCall{kind: "ApiChangelog", limit: 1},
			listCall{kind: "McpSpecification", limit: 1},
		)
	})

	It("does not emit an empty terminal page", func() {
		fixture := newResourceFixture()
		fixture.rovers = page(rover("only"))

		response, err := fixture.controller.GetAll(teamContext(), resourceParams(1, ""))

		Expect(err).NotTo(HaveOccurred())
		Expect(response.Items).To(HaveLen(1))
		Expect(response.UnderscoreLinks.Next).To(BeEmpty())
		Expect(fixture.calls).To(HaveLen(6))
	})

	It("continues at the next non-empty kind", func() {
		fixture := newResourceFixture()
		fixture.rovers = page(rover("rover"))
		fixture.mcpSpecifications = func(store.ListOpts) (*store.ListResponse[*roverv1.McpSpecification], error) {
			return pageWithLinks("mcp-self", "", mcpSpecification("mcp")), nil
		}

		first, err := fixture.controller.GetAll(teamContext(), resourceParams(1, ""))
		Expect(err).NotTo(HaveOccurred())
		cursor, err := decodeResourceCursor(first.UnderscoreLinks.Next)
		Expect(err).NotTo(HaveOccurred())
		Expect(cursor).To(Equal(resourceCursor{Version: 1, Kind: 5, Cursor: "mcp-self"}))

		second, err := fixture.controller.GetAll(teamContext(), resourceParams(1, first.UnderscoreLinks.Next))
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Items).To(ConsistOf(HaveField("Kind", "McpSpecification")))
	})

	It("returns an empty first page only when every store is empty", func() {
		fixture := newResourceFixture()

		response, err := fixture.controller.GetAll(teamContext(), resourceParams(1, ""))

		Expect(err).NotTo(HaveOccurred())
		Expect(response.Items).To(BeEmpty())
		Expect(response.UnderscoreLinks.Self).To(BeEmpty())
		Expect(response.UnderscoreLinks.Next).To(BeEmpty())
		Expect(fixture.calls).To(HaveLen(6))
	})

	It("includes MCP specifications", func() {
		fixture := newResourceFixture()
		fixture.mcpSpecifications = page(mcpSpecification("mcp"))

		response, err := fixture.controller.GetAll(teamContext(), resourceParams(1, ""))

		Expect(err).NotTo(HaveOccurred())
		Expect(response.Items).To(ConsistOf(HaveField("Kind", "McpSpecification")))
	})

	It("uses composite resource IDs in paths", func() {
		fixture := newResourceFixture()
		fixture.rovers = page(rover("name"))

		response, err := fixture.controller.GetAll(teamContext(), resourceParams(1, ""))

		Expect(err).NotTo(HaveOccurred())
		Expect(response.Items[0].Path).To(Equal("/rovers/eni--hyperion--name"))
	})

	It("traverses more than 500 resources", func() {
		fixture := newResourceFixture()
		expectedNames := make([]string, 501)
		for i := range expectedNames {
			expectedNames[i] = fmt.Sprintf("rover-%03d", i)
		}
		fixture.rovers = func(opts store.ListOpts) (*store.ListResponse[*roverv1.Rover], error) {
			start := 0
			if opts.Cursor != "" {
				start, _ = strconv.Atoi(strings.TrimPrefix(opts.Cursor, "cursor-"))
			}
			end := min(start+opts.Limit, len(expectedNames))
			items := make([]*roverv1.Rover, 0, end-start)
			for _, name := range expectedNames[start:end] {
				items = append(items, rover(name))
			}
			next := ""
			if end < len(expectedNames) {
				next = fmt.Sprintf("cursor-%d", end)
			}
			return pageWithLinks(fmt.Sprintf("cursor-%d", start), next, items...), nil
		}

		var collected []string
		cursor := ""
		for {
			response, err := fixture.controller.GetAll(teamContext(), resourceParams(20, cursor))
			Expect(err).NotTo(HaveOccurred())
			for _, item := range response.Items {
				collected = append(collected, item.Name)
			}
			cursor = response.UnderscoreLinks.Next
			if cursor == "" {
				break
			}
		}

		Expect(collected).To(HaveLen(501))
		Expect(collected).To(ConsistOf(expectedNames))
		Expect(slices.Compact(collected)).To(HaveLen(501))
	})

	It("rejects a backing store cursor that makes no progress", func() {
		fixture := newResourceFixture()
		fixture.rovers = func(opts store.ListOpts) (*store.ListResponse[*roverv1.Rover], error) {
			return pageWithLinks(opts.Cursor, opts.Cursor, rover("stuck")), nil
		}
		cursor, err := encodeResourceCursor(resourceCursor{Version: 1, Kind: 0, Cursor: "same"})
		Expect(err).NotTo(HaveOccurred())

		response, err := fixture.controller.GetAll(teamContext(), resourceParams(1, cursor))

		Expect(err).To(MatchError(ContainSubstring("Rover cursor did not advance")))
		Expect(response).To(BeNil())
	})

	DescribeTable("returns no partial response when a store fails",
		func(kind string, fail func(*resourceFixture)) {
			fixture := newResourceFixture()
			fail(fixture)

			response, err := fixture.controller.GetAll(teamContext(), resourceParams(20, ""))

			Expect(err).To(MatchError(And(ContainSubstring("listing "+kind), ContainSubstring("secret datastore detail"))))
			Expect(response).To(BeNil())
		},
		Entry("Rover", "Rover", func(f *resourceFixture) { f.rovers = failedPage[*roverv1.Rover] }),
		Entry("ApiSpecification", "ApiSpecification", func(f *resourceFixture) { f.apiSpecifications = failedPage[*roverv1.ApiSpecification] }),
		Entry("EventSpecification", "EventSpecification", func(f *resourceFixture) { f.eventSpecifications = failedPage[*roverv1.EventSpecification] }),
		Entry("Roadmap", "Roadmap", func(f *resourceFixture) { f.roadmaps = failedPage[*roverv1.Roadmap] }),
		Entry("ApiChangelog", "ApiChangelog", func(f *resourceFixture) { f.apiChangelogs = failedPage[*roverv1.ApiChangelog] }),
		Entry("McpSpecification", "McpSpecification", func(f *resourceFixture) { f.mcpSpecifications = failedPage[*roverv1.McpSpecification] }),
	)
})

func encodeJSON(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

type listCall struct {
	kind   string
	limit  int
	cursor string
}

type resourceFixture struct {
	controller          *ResourcesControllerImpl
	calls               []listCall
	rovers              func(store.ListOpts) (*store.ListResponse[*roverv1.Rover], error)
	apiSpecifications   func(store.ListOpts) (*store.ListResponse[*roverv1.ApiSpecification], error)
	eventSpecifications func(store.ListOpts) (*store.ListResponse[*roverv1.EventSpecification], error)
	roadmaps            func(store.ListOpts) (*store.ListResponse[*roverv1.Roadmap], error)
	apiChangelogs       func(store.ListOpts) (*store.ListResponse[*roverv1.ApiChangelog], error)
	mcpSpecifications   func(store.ListOpts) (*store.ListResponse[*roverv1.McpSpecification], error)
}

func newResourceFixture() *resourceFixture {
	f := &resourceFixture{
		rovers:              page[*roverv1.Rover](),
		apiSpecifications:   page[*roverv1.ApiSpecification](),
		eventSpecifications: page[*roverv1.EventSpecification](),
		roadmaps:            page[*roverv1.Roadmap](),
		apiChangelogs:       page[*roverv1.ApiChangelog](),
		mcpSpecifications:   page[*roverv1.McpSpecification](),
	}
	stores := &roverstore.Stores{}
	stores.RoverStore = resourceStore(f, "Rover", func(opts store.ListOpts) (*store.ListResponse[*roverv1.Rover], error) { return f.rovers(opts) })
	stores.APISpecificationStore = resourceStore(f, "ApiSpecification", func(opts store.ListOpts) (*store.ListResponse[*roverv1.ApiSpecification], error) {
		return f.apiSpecifications(opts)
	})
	stores.EventSpecificationStore = resourceStore(f, "EventSpecification", func(opts store.ListOpts) (*store.ListResponse[*roverv1.EventSpecification], error) {
		return f.eventSpecifications(opts)
	})
	stores.RoadmapStore = resourceStore(f, "Roadmap", func(opts store.ListOpts) (*store.ListResponse[*roverv1.Roadmap], error) { return f.roadmaps(opts) })
	stores.ApiChangelogStore = resourceStore(f, "ApiChangelog", func(opts store.ListOpts) (*store.ListResponse[*roverv1.ApiChangelog], error) {
		return f.apiChangelogs(opts)
	})
	stores.McpSpecificationStore = resourceStore(f, "McpSpecification", func(opts store.ListOpts) (*store.ListResponse[*roverv1.McpSpecification], error) {
		return f.mcpSpecifications(opts)
	})
	f.controller = NewResourcesController(stores)
	return f
}

func resourceStore[T store.Object](f *resourceFixture, kind string, list func(store.ListOpts) (*store.ListResponse[T], error)) *mocks.MockObjectStore[T] {
	mockStore := mocks.NewMockObjectStore[T](GinkgoT())
	mockStore.EXPECT().List(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, opts store.ListOpts) (*store.ListResponse[T], error) {
		Expect(opts.Prefix).To(Equal("poc--eni--hyperion/"))
		f.calls = append(f.calls, listCall{kind: kind, limit: opts.Limit, cursor: opts.Cursor})
		return list(opts)
	}).Maybe()
	return mockStore
}

func (f *resourceFixture) expectCalls(calls ...listCall) {
	Expect(f.calls).To(Equal(calls))
}

func teamContext() context.Context {
	return security.ToContext(context.Background(), &security.BusinessContext{
		Environment: "poc",
		Group:       "eni",
		Team:        "hyperion",
		ClientType:  security.ClientTypeTeam,
	})
}

func resourceParams(limit int32, cursor string) api.GetAllResourcesParams {
	return api.GetAllResourcesParams{Limit: limit, Cursor: cursor}
}

func failedPage[T store.Object](store.ListOpts) (*store.ListResponse[T], error) {
	return nil, errors.New("secret datastore detail")
}

func page[T store.Object](items ...T) func(store.ListOpts) (*store.ListResponse[T], error) {
	return func(store.ListOpts) (*store.ListResponse[T], error) {
		return &store.ListResponse[T]{Items: items}, nil
	}
}

func pageWithLinks[T store.Object](self, next string, items ...T) *store.ListResponse[T] {
	return &store.ListResponse[T]{Links: store.ListResponseLinks{Self: self, Next: next}, Items: items}
}

func objectMeta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: "poc--eni--hyperion"}
}

func rover(name string) *roverv1.Rover {
	return &roverv1.Rover{ObjectMeta: objectMeta(name)}
}

func apiSpecification(name string) *roverv1.ApiSpecification {
	return &roverv1.ApiSpecification{ObjectMeta: objectMeta(name)}
}

func mcpSpecification(name string) *roverv1.McpSpecification {
	return &roverv1.McpSpecification{ObjectMeta: objectMeta(name)}
}
