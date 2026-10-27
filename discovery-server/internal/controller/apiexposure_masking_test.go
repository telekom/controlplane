// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/stretchr/testify/mock"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	securitymock "github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
	csstore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
	csmocks "github.com/telekom/controlplane/common-server/test/mocks"
	"github.com/telekom/controlplane/discovery-server/internal/api"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
	"github.com/telekom/controlplane/discovery-server/test/mocks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	exposureSubsPath = "/applications/eni--hyperion--my-app/apiexposures/eni-distr-v1/apisubscriptions"
	ownerSubPath     = "/applications/eni--hyperion--my-app/apisubscriptions/eni-distr-v1"
	storeCursorPage2 = "poc--other--team/page-2/"
	resolvedFake     = "FAKE-RESOLVED-OWNER-PASSWORD"
)

var (
	teamReadToken  = securitymock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:team:read"})
	groupReadToken = securitymock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:group:read"})
	adminReadToken = securitymock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:admin:read"})
)

// countingReplacer counts ReplaceAll calls and either fails or substitutes
// secret references in Basic passwords with a fake resolved value.
type countingReplacer struct {
	calls atomic.Int32
	fail  bool
}

func (r *countingReplacer) ReplaceAll(_ context.Context, obj any, _ []string) (any, error) {
	r.calls.Add(1)
	if r.fail {
		return nil, errors.New("secret resolver must not be used")
	}
	sub, ok := obj.(*apiv1.ApiSubscription)
	if !ok {
		return obj, nil
	}
	out := sub.DeepCopy()
	if out.Spec.Security != nil && out.Spec.Security.M2M != nil && out.Spec.Security.M2M.Basic != nil &&
		strings.HasPrefix(out.Spec.Security.M2M.Basic.Password, "$<") {
		out.Spec.Security.M2M.Basic.Password = resolvedFake
	}
	return out, nil
}

func subscriptionFixture(namespace, name, basePath string, security *apiv1.SubscriberSecurity) *apiv1.ApiSubscription {
	sub := mocks.GetApiSubscription(GinkgoT(), "apiSubscription.json").DeepCopy()
	sub.Namespace = namespace
	sub.Name = name
	sub.Spec.ApiBasePath = basePath
	sub.Spec.Security = security
	return sub
}

var _ = Describe("Subscriber credential projections", func() {
	It("masks existing fields across mixed items without adding missing credentials", func() {
		security := func(raw string) api.SubscriberSecurity {
			var s api.SubscriberSecurity
			Expect(s.UnmarshalJSON([]byte(raw))).To(Succeed())
			return s
		}
		paging := api.Paging{Page: 1, LastPage: 1, Total: 5}
		links := api.Links{First: "/first", Last: "/last"}
		resp := &api.ApiSubscriptionListResponse{
			Items: []api.ApiSubscriptionResponse{
				{Name: "basic", Security: security(`{"type":"basicAuth","username":"u","password":""}`)},
				{Name: "none"},
				{Name: "tokens", Security: security(
					`{"type":"oauth2","clientId":"long-client-id","refreshToken":"r","rotateToken":"t","scopes":["s"]}`,
				)},
				{Name: "null", Security: security(`null`)},
				{Name: "secret-only", Security: security(`{"type":"oauth2","clientSecret":"long-secret"}`)},
			},
			Paging:          paging,
			UnderscoreLinks: links,
		}

		_, err := secrets.NewObfuscator().ReplaceAll(context.Background(), resp, subscriberCredentialPaths)
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Paging).To(Equal(paging))
		Expect(resp.UnderscoreLinks).To(Equal(links))
		Expect(resp.Items).To(HaveLen(5))
		Expect(resp.Items).To(WithTransform(func(items []api.ApiSubscriptionResponse) []string {
			names := make([]string, len(items))
			for i := range items {
				names[i] = items[i].Name
			}
			return names
		}, Equal([]string{"basic", "none", "tokens", "null", "secret-only"})))
		sec := func(i int) string {
			raw, marshalErr := resp.Items[i].Security.MarshalJSON()
			Expect(marshalErr).ToNot(HaveOccurred())
			return string(raw)
		}
		Expect(sec(0)).To(MatchJSON(`{"type":"basicAuth","username":"**********","password":"**********"}`))
		Expect(resp.Items[1].Security).To(Equal(api.SubscriberSecurity{}))
		Expect(sec(2)).To(MatchJSON(
			`{"type":"oauth2","clientId":"**********","refreshToken":"**********","rotateToken":"**********","scopes":["s"]}`,
		))
		Expect(sec(3)).To(MatchJSON(`null`))
		Expect(sec(4)).To(MatchJSON(`{"type":"oauth2","clientSecret":"**********"}`))
	})
})

var _ = Describe("ApiExposure subscriptions credential masking", func() {
	var (
		plainListCalls  atomic.Int32
		secretListCalls atomic.Int32
		replacer        *countingReplacer

		page1, page2 []*apiv1.ApiSubscription
		originals    []*apiv1.ApiSubscription
		credentials  []string
	)

	BeforeEach(func() {
		plainListCalls.Store(0)
		secretListCalls.Store(0)
		replacer = &countingReplacer{fail: true}

		basicRef := subscriptionFixture("poc--other--team", "foreign-app--sub-basic", "/eni/distr/v1",
			&apiv1.SubscriberSecurity{M2M: &apiv1.SubscriberMachine2MachineAuthentication{
				Basic: &apiv1.BasicAuthCredentials{Username: "$<ref:basic-user:FIX1>", Password: "$<ref:basic-pass:FIX2>"},
			}})
		oauthMixed := subscriptionFixture("poc--other--team2", "oauth-app--sub-oauth", "/eni/distr/v1",
			&apiv1.SubscriberSecurity{M2M: &apiv1.SubscriberMachine2MachineAuthentication{
				Client: &apiv1.OAuth2ClientCredentials{
					ClientId:     "FIXTURE-PLAIN-CLIENT-ID",
					ClientSecret: "$<ref:client-secret:FIX3>",
					RefreshToken: "FIXTURE-REFRESH-TOKEN",
				},
				Basic:  &apiv1.BasicAuthCredentials{Username: "FIXTURE-PLAIN-USER", Password: "$<ref:oauth-pass:FIX4>"},
				Scopes: []string{"scope-a"},
			}})
		unrelatedBroken := subscriptionFixture("poc--other--team3", "unrelated--other-v1", "/other/v1",
			&apiv1.SubscriberSecurity{M2M: &apiv1.SubscriberMachine2MachineAuthentication{
				Basic: &apiv1.BasicAuthCredentials{Username: "$<broken", Password: "$<ref:broken"},
			}})
		page2Oauth := subscriptionFixture("poc--other--team4", "late-app--sub-late", "/eni/distr/v1",
			&apiv1.SubscriberSecurity{M2M: &apiv1.SubscriberMachine2MachineAuthentication{
				Client: &apiv1.OAuth2ClientCredentials{ClientId: "$<ref:late-client:FIX5>", ClientSecret: "FIXTURE-PLAIN-SECRET"},
			}})
		page2Unrelated := subscriptionFixture("poc--other--team5", "unrelated2--other-v2", "/other/v2",
			&apiv1.SubscriberSecurity{M2M: &apiv1.SubscriberMachine2MachineAuthentication{
				Client: &apiv1.OAuth2ClientCredentials{ClientId: "$<ref:x", ClientSecret: "$<ref:y"},
			}})

		page1 = []*apiv1.ApiSubscription{basicRef, unrelatedBroken, oauthMixed}
		page2 = []*apiv1.ApiSubscription{page2Unrelated, page2Oauth}
		originals = make([]*apiv1.ApiSubscription, 0, len(page1)+len(page2))
		for _, s := range append(append([]*apiv1.ApiSubscription{}, page1...), page2...) {
			originals = append(originals, s.DeepCopy())
		}
		credentials = []string{
			"$<ref:", "$<broken", "FIX1", "FIX2", "FIX3", "FIX4", "FIX5",
			"FIXTURE-PLAIN-CLIENT-ID", "FIXTURE-REFRESH-TOKEN", "FIXTURE-PLAIN-USER", "FIXTURE-PLAIN-SECRET",
			"subscriber-secret-password", "subscriber-user",
		}

		plain := csmocks.NewMockObjectStore[*apiv1.ApiSubscription](GinkgoT())
		plain.EXPECT().List(mock.Anything, mock.Anything).RunAndReturn(
			func(_ context.Context, opts csstore.ListOpts) (*csstore.ListResponse[*apiv1.ApiSubscription], error) {
				plainListCalls.Add(1)
				if opts.Cursor == storeCursorPage2 {
					return &csstore.ListResponse[*apiv1.ApiSubscription]{Items: page2}, nil
				}
				Expect(opts.Cursor).To(BeEmpty())
				return &csstore.ListResponse[*apiv1.ApiSubscription]{
					Items: page1,
					Links: csstore.ListResponseLinks{Next: storeCursorPage2},
				}, nil
			},
		).Maybe()

		secretInner := csmocks.NewMockObjectStore[*apiv1.ApiSubscription](GinkgoT())
		secretInner.EXPECT().List(mock.Anything, mock.Anything).RunAndReturn(
			func(_ context.Context, _ csstore.ListOpts) (*csstore.ListResponse[*apiv1.ApiSubscription], error) {
				secretListCalls.Add(1)
				return nil, errors.New("secret store must not be used")
			},
		).Maybe()
		secretInner.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
			func(_ context.Context, _, _ string) (*apiv1.ApiSubscription, error) {
				return subscriptionFixture("poc--eni--hyperion", "my-app--eni-distr-v1", "/eni/distr/v1",
					&apiv1.SubscriberSecurity{M2M: &apiv1.SubscriberMachine2MachineAuthentication{
						Basic: &apiv1.BasicAuthCredentials{Username: "owner-user", Password: "$<ref:owner-pass:OWN1>"},
					}}), nil
			},
		).Maybe()

		prevPlain, prevSecret := stores.APISubscriptionStore, stores.APISubscriptionSecretStore
		stores.APISubscriptionStore = plain
		stores.APISubscriptionSecretStore = secrets.WrapStore(secretInner, sstore.SecretsForKinds["ApiSubscription"], replacer)
		DeferCleanup(func() {
			stores.APISubscriptionStore = prevPlain
			stores.APISubscriptionSecretStore = prevSecret
		})
	})

	expectNoSecretAccessAndUnchangedResources := func() {
		Expect(secretListCalls.Load()).To(BeZero(), "secret store List must not be called")
		Expect(replacer.calls.Load()).To(BeZero(), "secret resolver must not be called")
		Expect(plainListCalls.Load()).To(BeEquivalentTo(2), "plain store must be traversed through both cursor pages")
		current := append(append([]*apiv1.ApiSubscription{}, page1...), page2...)
		Expect(current).To(HaveLen(len(originals)))
		for i := range current {
			Expect(current[i]).To(Equal(originals[i]), "cached resource must not be mutated")
		}
	}

	type item struct {
		Name     string          `json:"name"`
		BasePath string          `json:"basePath"`
		Security json.RawMessage `json:"security"`
	}
	type listBody struct {
		Items  []item            `json:"items"`
		Links  map[string]string `json:"_links"`
		Paging map[string]int    `json:"paging"`
	}

	const masked = `"**********"`

	DescribeTable("masks subscriber credentials for every scope without resolving secrets",
		func(token string) {
			req := httptest.NewRequest(http.MethodGet, exposureSubsPath, http.NoBody)
			resp, err := ExecuteRequest(req, token)
			body := ExpectStatusOk(resp, err)

			for _, c := range credentials {
				Expect(body).ToNot(ContainSubstring(c))
			}
			Expect(resp.Header.Get("X-Total-Count")).To(Equal("3"))
			Expect(resp.Header.Get("X-Result-Count")).To(Equal("3"))

			var parsed listBody
			Expect(json.Unmarshal([]byte(body), &parsed)).To(Succeed())
			Expect(parsed.Items).To(HaveLen(3))
			Expect(parsed.Items[0].Name).To(Equal("sub-basic"))
			Expect(parsed.Items[0].Security).To(MatchJSON(
				`{"type":"basicAuth","username":` + masked + `,"password":` + masked + `}`,
			))
			Expect(parsed.Items[1].Name).To(Equal("sub-oauth"))
			Expect(parsed.Items[1].Security).To(MatchJSON(
				`{"type":"oauth2","scopes":["scope-a"],"clientId":` + masked + `,"clientSecret":` + masked +
					`,"username":` + masked + `,"password":` + masked + `}`,
			))
			Expect(parsed.Items[2].Name).To(Equal("sub-late"))
			Expect(parsed.Items[2].Security).To(MatchJSON(
				`{"type":"oauth2","clientId":` + masked + `,"clientSecret":` + masked + `}`,
			))
			for _, it := range parsed.Items {
				Expect(it.BasePath).To(Equal("/eni/distr/v1"))
			}

			expectNoSecretAccessAndUnchangedResources()
		},
		Entry("team read", teamReadToken),
		Entry("team all", teamToken),
		Entry("group read", groupReadToken),
		Entry("group all", groupToken),
		Entry("admin read", adminReadToken),
		Entry("admin all", adminToken),
		Entry("obfuscated", obfuscToken),
	)

	DescribeTable("applies HTTP offset/limit pagination over masked results",
		func(query string, total, result int, firstName string, next, prev bool) {
			req := httptest.NewRequest(http.MethodGet, exposureSubsPath+query, http.NoBody)
			resp, err := ExecuteRequest(req, adminToken)
			body := ExpectStatusOk(resp, err)

			Expect(resp.Header.Get("X-Total-Count")).To(Equal(strconv.Itoa(total)))
			Expect(resp.Header.Get("X-Result-Count")).To(Equal(strconv.Itoa(result)))
			var parsed listBody
			Expect(json.Unmarshal([]byte(body), &parsed)).To(Succeed())
			Expect(parsed.Items).To(HaveLen(result))
			if firstName != "" {
				Expect(parsed.Items[0].Name).To(Equal(firstName))
			}
			Expect(parsed.Links["self"]).To(HavePrefix(exposureSubsPath + "?offset="))
			Expect(parsed.Links).To(HaveKey("first"))
			if next {
				Expect(parsed.Links["next"]).ToNot(BeEmpty())
			} else {
				Expect(parsed.Links["next"]).To(BeEmpty())
			}
			if prev {
				Expect(parsed.Links["prev"]).ToNot(BeEmpty())
			} else {
				Expect(parsed.Links["prev"]).To(BeEmpty())
			}
			for _, c := range credentials {
				Expect(body).ToNot(ContainSubstring(c))
			}
			expectNoSecretAccessAndUnchangedResources()
		},
		Entry("first page", "?offset=0&limit=2", 3, 2, "sub-basic", true, false),
		Entry("nonzero offset reaches store page 2 item", "?offset=2&limit=2", 3, 1, "sub-late", false, true),
		Entry("empty page", "?offset=10&limit=2", 3, 0, "", false, true),
	)

	It("keeps zero-value security omitted as before", func() {
		page1[0].Spec.Security = nil
		originals[0].Spec.Security = nil
		req := httptest.NewRequest(http.MethodGet, exposureSubsPath, http.NoBody)
		resp, err := ExecuteRequest(req, adminToken)
		body := ExpectStatusOk(resp, err)

		var parsed struct {
			Items []map[string]json.RawMessage `json:"items"`
		}
		Expect(json.Unmarshal([]byte(body), &parsed)).To(Succeed())
		Expect(parsed.Items).To(HaveLen(3))
		Expect(parsed.Items[0]["name"]).To(MatchJSON(`"sub-basic"`))
		Expect(parsed.Items[0]).ToNot(HaveKey("security"))
		expectNoSecretAccessAndUnchangedResources()
	})

	It("still returns 403 for an unauthorized team", func() {
		req := httptest.NewRequest(http.MethodGet, exposureSubsPath, http.NoBody)
		resp, err := ExecuteRequest(req, teamNoResToken)
		ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		Expect(secretListCalls.Load()).To(BeZero())
		Expect(replacer.calls.Load()).To(BeZero())
	})

	It("owner-facing read still resolves secrets through the wrapped store", func() {
		replacer.fail = false
		req := httptest.NewRequest(http.MethodGet, ownerSubPath, http.NoBody)
		resp, err := ExecuteRequest(req, teamReadToken)
		body := ExpectStatusOk(resp, err)

		Expect(replacer.calls.Load()).To(BeNumerically(">", 0), "resolver spy must be live for owner reads")
		Expect(body).To(ContainSubstring(resolvedFake))
		Expect(body).To(ContainSubstring("owner-user"))
		Expect(body).ToNot(ContainSubstring("*****"))
	})
})
