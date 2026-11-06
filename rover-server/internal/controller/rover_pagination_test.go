// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	cstore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func roverCallsSince(start int, method string) [][]any {
	m := stores.RoverStore.(*mocks.MockObjectStore[*roverv1.Rover])
	var calls [][]any
	for _, c := range m.Calls[start:] {
		if c.Method == method {
			calls = append(calls, c.Arguments[1:])
		}
	}
	return calls
}

func roverCallCount() int {
	return len(stores.RoverStore.(*mocks.MockObjectStore[*roverv1.Rover]).Calls)
}

func getAsTeam(target string) *http.Response {
	res, err := ExecuteRequest(httptest.NewRequest(http.MethodGet, target, nil), teamToken)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(res.Body.Close()).To(Succeed()) })
	return res
}

func nameQuery(count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = fmt.Sprintf("names=missing-%d", i)
	}
	return strings.Join(parts, "&")
}

var _ = Describe("Rover pagination", func() {
	DescribeTable("returns exactly one store page at the page boundary",
		func(count int) {
			storeMock := mocks.NewMockObjectStore[*roverv1.Rover](GinkgoT())
			storeMock.EXPECT().List(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, opts cstore.ListOpts) (*cstore.ListResponse[*roverv1.Rover], error) {
				Expect(opts.Limit).To(Equal(2))
				page := &cstore.ListResponse[*roverv1.Rover]{}
				for i := 0; i < min(count, opts.Limit); i++ {
					page.Items = append(page.Items, mocks.GetRover(GinkgoT(), mocks.RoverFileName).DeepCopy())
				}
				if count > opts.Limit {
					page.Links.Next = "next-rover"
				}
				return page, nil
			}).Once()
			ctrl := NewRoverController(stores)
			ctrl.Store = storeMock
			res, err := ctrl.GetApplicationsInfo(teamContext(), api.GetApplicationsInfoParams{Limit: 2})
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Applications).To(HaveLen(min(count, 2)))
			if count > 2 {
				Expect(res.UnderscoreLinks.Next).To(Equal("next-rover"))
			} else {
				Expect(res.UnderscoreLinks.Next).To(BeEmpty())
			}
		},
		Entry("fewer than limit", 1),
		Entry("exactly limit", 2),
		Entry("more than limit", 3),
	)
	It("propagates named lookup errors other than not found", func() {
		storeMock := mocks.NewMockObjectStore[*roverv1.Rover](GinkgoT())
		storeMock.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "broken").Return(nil, context.Canceled).Once()
		ctrl := NewRoverController(stores)
		ctrl.Store = storeMock
		_, err := ctrl.GetApplicationsInfo(teamContext(), api.GetApplicationsInfoParams{Names: []string{"broken"}})
		Expect(err).To(MatchError(context.Canceled))
	})
	DescribeTable("lists one scoped page with the effective limit",
		func(target string, limit int, cursor, self string) {
			start := roverCallCount()
			res := getAsTeam(target)
			Expect(res.StatusCode).To(Equal(http.StatusOK))
			var body struct {
				Links *api.Links `json:"_links"`
			}
			Expect(json.NewDecoder(res.Body).Decode(&body)).To(Succeed())
			lists := roverCallsSince(start, "List")
			Expect(lists).To(HaveLen(1))
			opts := lists[0][0].(cstore.ListOpts)
			Expect(opts.Limit).To(Equal(limit))
			Expect(opts.Cursor).To(Equal(cursor))
			Expect(opts.Prefix).To(Equal("poc--eni--hyperion/"))
			Expect(body.Links).NotTo(BeNil())
			Expect(body.Links.Self).To(Equal(self))
		},
		Entry("rovers default", "/rovers", 20, "", "http://example.com/rovers?limit=20"),
		Entry("rovers custom", "/rovers?limit=5&cursor=a%2Fb", 5, "a/b", "http://example.com/rovers?limit=5"),
		Entry("info default", "/rovers/info", 20, "", "http://example.com/rovers/info?limit=20"),
		Entry("info custom", "/rovers/info?limit=1&cursor=a%2Fb", 1, "a/b", "http://example.com/rovers/info?limit=1"),
	)
	DescribeTable("validates route-specific limits",
		func(target string, status int) {
			res := getAsTeam(target)
			Expect(res.StatusCode).To(Equal(status))
			if status == http.StatusBadRequest {
				Expect(res.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			}
		},
		Entry("minimum", "/rovers?limit=1", http.StatusOK),
		Entry("maximum", "/rovers?limit=20", http.StatusOK),
		Entry("zero", "/rovers?limit=0", http.StatusBadRequest),
		Entry("above maximum", "/rovers?limit=21", http.StatusBadRequest),
		Entry("info maximum", "/rovers/info?limit=20", http.StatusOK),
		Entry("info zero", "/rovers/info?limit=0", http.StatusBadRequest),
		Entry("info above maximum", "/rovers/info?limit=21", http.StatusBadRequest),
		Entry("info nonnumeric", "/rovers/info?limit=abc", http.StatusBadRequest),
		Entry("other limit unchanged", "/apispecifications?limit=21", http.StatusBadRequest),
	)
	It("looks up names sorted and deduplicated in the team namespace without listing", func() {
		start := roverCallCount()
		res := getAsTeam("/rovers/info?names=zzz-missing&names=rover-local-sub&names=rover-local-sub&limit=1&cursor=ignored")
		Expect(res.StatusCode).To(Equal(http.StatusOK))
		var raw map[string]json.RawMessage
		Expect(json.NewDecoder(res.Body).Decode(&raw)).To(Succeed())
		Expect(raw).NotTo(HaveKey("_links"))
		var apps []api.ApplicationInfo
		Expect(json.Unmarshal(raw["applications"], &apps)).To(Succeed())
		Expect(apps).To(HaveLen(1))
		Expect(apps[0].Name).To(Equal("rover-local-sub"))
		Expect(roverCallsSince(start, "List")).To(BeEmpty())
		Expect(roverCallsSince(start, "Get")).To(Equal([][]any{
			{"poc--eni--hyperion", "rover-local-sub"}, {"poc--eni--hyperion", "zzz-missing"},
		}))
	})
	It("accepts 100 names", func() {
		Expect(getAsTeam("/rovers/info?" + nameQuery(100)).StatusCode).To(Equal(http.StatusOK))
	})
	It("rejects 101 names", func() {
		Expect(getAsTeam("/rovers/info?" + nameQuery(101)).StatusCode).To(Equal(http.StatusBadRequest))
	})
	It("also checks the names cap in the controller", func() {
		_, err := NewRoverController(stores).GetApplicationsInfo(teamContext(), api.GetApplicationsInfoParams{Names: make([]string, 101)})
		var problem problems.Problem
		Expect(errors.As(err, &problem)).To(BeTrue())
		Expect(problem.Code()).To(Equal(http.StatusBadRequest))
	})
})
