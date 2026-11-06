// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	csstore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/inmemory"
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
	csmocks "github.com/telekom/controlplane/common-server/test/mocks"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/discovery-server/internal/api"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/secret-manager/api/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These specs exercise the subscriber endpoints against real in-memory
// stores (seeded through OnUpdate, the informer callback) and the real
// secret wrapper. The dynamic client only satisfies the store constructor;
// no Kubernetes API behavior is under test.

const (
	filterProviderNs  = "poc--eni--hyperion"
	filterProviderApp = "my-app"
	filterAppId       = "eni--hyperion--my-app"
	resolvableSecret  = "resolvable-secret"
	pagedMatchCount   = 510
	pagedUnrelated    = 510
)

func newCacheStore[T csstore.Object](ctx context.Context, gvk schema.GroupVersionKind, resource string) *inmemory.InmemoryObjectStore[T] {
	gvr := gvk.GroupVersion().WithResource(resource)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: gvk.Kind + "List"})
	s := inmemory.NewOrDie[T](ctx, inmemory.StoreOpts{Client: client, GVR: gvr, GVK: gvk})
	Eventually(s.Ready).WithTimeout(10 * time.Second).WithPolling(20 * time.Millisecond).Should(BeTrue())
	return s
}

type cacheUpdater interface {
	OnUpdate(ctx context.Context, obj *unstructured.Unstructured) error
}

// seed writes obj into the cache through the informer callback.
func seed(ctx context.Context, s cacheUpdater, obj runtime.Object) {
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	Expect(err).NotTo(HaveOccurred())
	Expect(s.OnUpdate(ctx, &unstructured.Unstructured{Object: content})).To(Succeed())
}

func newFilterApiExposure(name, basePath string) *apiv1.ApiExposure {
	return &apiv1.ApiExposure{
		TypeMeta: metav1.TypeMeta{APIVersion: apiv1.GroupVersion.String(), Kind: "ApiExposure"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      filterProviderApp + "--" + name,
			Namespace: filterProviderNs,
			Labels:    map[string]string{"cp.ei.telekom.de/application": filterProviderApp},
		},
		Spec: apiv1.ApiExposureSpec{ApiBasePath: basePath},
	}
}

func newFilterApiSubscription(namespace, name, basePath, clientSecret string) *apiv1.ApiSubscription {
	return &apiv1.ApiSubscription{
		TypeMeta:   metav1.TypeMeta{APIVersion: apiv1.GroupVersion.String(), Kind: "ApiSubscription"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: apiv1.ApiSubscriptionSpec{
			ApiBasePath: basePath,
			Zone:        ctypes.ObjectRef{Name: "dataplane1", Namespace: "poc"},
			Security: &apiv1.SubscriberSecurity{
				M2M: &apiv1.SubscriberMachine2MachineAuthentication{
					Client: &apiv1.OAuth2ClientCredentials{ClientId: "client", ClientSecret: clientSecret},
				},
			},
		},
	}
}

func newFilterEventExposure(name, eventType string) *eventv1.EventExposure {
	return &eventv1.EventExposure{
		TypeMeta: metav1.TypeMeta{APIVersion: eventv1.GroupVersion.String(), Kind: "EventExposure"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      filterProviderApp + "--" + name,
			Namespace: filterProviderNs,
			Labels:    map[string]string{"cp.ei.telekom.de/application": filterProviderApp},
		},
		Spec: eventv1.EventExposureSpec{EventType: eventType},
	}
}

func newFilterEventSubscription(namespace, name, eventType string) *eventv1.EventSubscription {
	return &eventv1.EventSubscription{
		TypeMeta:   metav1.TypeMeta{APIVersion: eventv1.GroupVersion.String(), Kind: "EventSubscription"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: eventv1.EventSubscriptionSpec{
			EventType: eventType,
			Zone:      ctypes.ObjectRef{Name: "dataplane1", Namespace: "poc"},
		},
	}
}

func apiSubIdentities(items []api.ApiSubscriptionResponse) []string {
	ids := make([]string, 0, len(items))
	for i := range items {
		item := &items[i]
		ids = append(ids, item.Team.Hub+"/"+item.Team.Name+"/"+item.Name)
	}
	return ids
}

func eventSubIdentities(items []api.EventSubscriptionResponse) []string {
	ids := make([]string, 0, len(items))
	for i := range items {
		item := &items[i]
		ids = append(ids, item.Team.Hub+"/"+item.Team.Name+"/"+item.Name)
	}
	return ids
}

func pagedIdentities(prefix string, from, to int) []string {
	ids := make([]string, 0, to-from)
	for i := from; i < to; i++ {
		ids = append(ids, fmt.Sprintf("other/teamb/%s-%04d", prefix, i))
	}
	return ids
}

var _ = Describe("Subscriber filtering against real stores", Ordered, ContinueOnFailure, func() {
	var (
		storeCtx    context.Context
		storeCancel context.CancelFunc
		reqCtx      context.Context

		apiExposures  *inmemory.InmemoryObjectStore[*apiv1.ApiExposure]
		apiSubs       *inmemory.InmemoryObjectStore[*apiv1.ApiSubscription]
		apiSubsListed int
		evExposures   *inmemory.InmemoryObjectStore[*eventv1.EventExposure]
		evSubs        *inmemory.InmemoryObjectStore[*eventv1.EventSubscription]
		evSubsListed  int
		appStore      *inmemory.InmemoryObjectStore[*applicationv1.Application]
		zoneStore     *inmemory.InmemoryObjectStore[*adminv1.Zone]
		approvals     *inmemory.InmemoryObjectStore[*approvalv1.Approval]
		secretMgr     *fake.MockSecretManager

		apiCtrl   *apiExposureController
		eventCtrl *eventExposureController
	)

	BeforeAll(func() {
		storeCtx, storeCancel = context.WithCancel(context.Background())
		reqCtx = security.ToContext(context.Background(), &security.BusinessContext{
			Environment: "poc", Group: "eni", Team: "hyperion",
			ClientType: security.ClientTypeTeam, AccessType: security.AccessTypeReadWrite,
		})

		apiExposures = newCacheStore[*apiv1.ApiExposure](storeCtx, apiv1.GroupVersion.WithKind("ApiExposure"), "apiexposures")
		apiSubs = newCacheStore[*apiv1.ApiSubscription](storeCtx, apiv1.GroupVersion.WithKind("ApiSubscription"), "apisubscriptions")
		evExposures = newCacheStore[*eventv1.EventExposure](storeCtx, eventv1.GroupVersion.WithKind("EventExposure"), "eventexposures")
		evSubs = newCacheStore[*eventv1.EventSubscription](storeCtx, eventv1.GroupVersion.WithKind("EventSubscription"), "eventsubscriptions")

		appStore = newCacheStore[*applicationv1.Application](storeCtx, applicationv1.GroupVersion.WithKind("Application"), "applications")
		zoneStore = newCacheStore[*adminv1.Zone](storeCtx, adminv1.GroupVersion.WithKind("Zone"), "zones")
		approvals = newCacheStore[*approvalv1.Approval](storeCtx, approvalv1.GroupVersion.WithKind("Approval"), "approvals")

		// Unrelated subscriptions whose secrets cannot be resolved. Resolving
		// any of them would fail the whole request.
		for i := range 3 {
			seed(storeCtx, apiSubs, newFilterApiSubscription("poc--other--team", fmt.Sprintf("unrelated-%d", i),
				fmt.Sprintf("/unrelated/v%d", i), "$<missing-unrelated>"))
		}
		seed(storeCtx, apiSubs, newFilterApiSubscription("poc--other--team", "near-miss",
			"/filter/api/v1/extra", "$<missing-near-miss>"))
	})

	AfterAll(func() {
		storeCancel()
	})

	BeforeEach(func() {
		secretMgr = fake.NewMockSecretManager(GinkgoT())
		resolver := secrets.NewSecretManagerResolver(secretMgr)

		// Count returned items while exercising the real store's filtering.
		apiSubsListed = 0
		apiSubStore := csmocks.NewMockObjectStore[*apiv1.ApiSubscription](GinkgoT())
		apiSubStore.EXPECT().List(mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, opts csstore.ListOpts) (*csstore.ListResponse[*apiv1.ApiSubscription], error) {
				resp, err := apiSubs.List(ctx, opts)
				if err == nil {
					apiSubsListed += len(resp.Items)
				}
				return resp, err
			},
		).Maybe()
		evSubsListed = 0
		evSubStore := csmocks.NewMockObjectStore[*eventv1.EventSubscription](GinkgoT())
		evSubStore.EXPECT().List(mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, opts csstore.ListOpts) (*csstore.ListResponse[*eventv1.EventSubscription], error) {
				resp, err := evSubs.List(ctx, opts)
				if err == nil {
					evSubsListed += len(resp.Items)
				}
				return resp, err
			},
		).Maybe()

		filterStores := &sstore.Stores{
			APIExposureStore:           apiExposures,
			APIExposureSecretStore:     secrets.WrapStore(apiExposures, sstore.SecretsForKinds["ApiExposure"], resolver),
			APISubscriptionStore:       apiSubStore,
			APISubscriptionSecretStore: secrets.WrapStore(apiSubs, sstore.SecretsForKinds["ApiSubscription"], resolver),
			ApplicationStore:           appStore,
			ZoneStore:                  zoneStore,
			ApprovalStore:              approvals,
			EventExposureStore:         evExposures,
			EventSubscriptionStore:     evSubStore,
		}
		apiCtrl = &apiExposureController{stores: filterStores}
		eventCtrl = &eventExposureController{stores: filterStores}
	})

	Context("API exposure subscriptions", func() {
		It("returns only exact cross-team matches with masked credentials and no secret resolution", func() {
			seed(storeCtx, apiExposures, newFilterApiExposure("filter-api-v1", "/filter/api/v1"))
			seed(storeCtx, apiSubs, newFilterApiSubscription("poc--other--teamb", "match-b", "/filter/api/v1", "$<"+resolvableSecret+">"))
			seed(storeCtx, apiSubs, newFilterApiSubscription("poc--eni--teamc", "match-c", "/filter/api/v1", "$<"+resolvableSecret+">"))

			resp, err := apiCtrl.GetSubscriptions(reqCtx, filterAppId, "filter-api-v1", api.GetAllExposureApiSubscriptionsParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Paging.Total).To(Equal(2))
			Expect(apiSubIdentities(resp.Items)).To(ConsistOf("other/teamb/match-b", "eni/teamc/match-c"))
			for _, item := range resp.Items {
				Expect(item.BasePath).To(Equal("/filter/api/v1"))
				oauth2, oErr := item.Security.AsOAuth2()
				Expect(oErr).NotTo(HaveOccurred())
				Expect(oauth2.ClientId).To(Equal("**********"))
				Expect(oauth2.ClientSecret).To(Equal("**********"))
			}
			Expect(apiSubsListed).To(Equal(2))
			Expect(secretMgr.AssertNotCalled(GinkgoT(), "Get", mock.Anything, mock.Anything)).To(BeTrue())
		})

		It("masks a missing matching subscription secret without resolving it", func() {
			seed(storeCtx, apiExposures, newFilterApiExposure("filter-missing-v1", "/filter/missing/v1"))
			seed(storeCtx, apiSubs, newFilterApiSubscription("poc--other--teamb", "match-missing", "/filter/missing/v1", "$<missing-matching>"))
			resp, err := apiCtrl.GetSubscriptions(reqCtx, filterAppId, "filter-missing-v1", api.GetAllExposureApiSubscriptionsParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Items).To(HaveLen(1))
			oauth2, err := resp.Items[0].Security.AsOAuth2()
			Expect(err).NotTo(HaveOccurred())
			Expect(oauth2.ClientSecret).To(Equal("**********"))
			Expect(secretMgr.AssertNotCalled(GinkgoT(), "Get", mock.Anything, mock.Anything)).To(BeTrue())
		})

		It("returns an empty list when nothing matches", func() {
			seed(storeCtx, apiExposures, newFilterApiExposure("filter-none-v1", "/filter/none/v1"))

			resp, err := apiCtrl.GetSubscriptions(reqCtx, filterAppId, "filter-none-v1", api.GetAllExposureApiSubscriptionsParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Items).To(BeEmpty())
			Expect(resp.Paging.Total).To(BeZero())
		})

		It("paginates more than one store batch of matches", func() {
			seed(storeCtx, apiExposures, newFilterApiExposure("filter-paged-v1", "/filter/paged/v1"))
			for i := range pagedMatchCount {
				seed(storeCtx, apiSubs, newFilterApiSubscription("poc--other--teamb", fmt.Sprintf("paged-%04d", i), "/filter/paged/v1", "plain"))
			}
			for i := range pagedUnrelated {
				seed(storeCtx, apiSubs, newFilterApiSubscription("poc--other--teamb", fmt.Sprintf("paged-other-%04d", i), "/filter/other/v1", "plain"))
			}

			resp, err := apiCtrl.GetSubscriptions(reqCtx, filterAppId, "filter-paged-v1",
				api.GetAllExposureApiSubscriptionsParams{Offset: 500, Limit: 20})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Paging.Total).To(Equal(pagedMatchCount))
			Expect(resp.Paging.Page).To(Equal(26))
			Expect(resp.Paging.LastPage).To(Equal(26))
			Expect(apiSubIdentities(resp.Items)).To(Equal(pagedIdentities("paged", 500, pagedMatchCount)))
			for _, item := range resp.Items {
				Expect(item.BasePath).To(Equal("/filter/paged/v1"))
				oauth2, err := item.Security.AsOAuth2()
				Expect(err).NotTo(HaveOccurred())
				Expect(oauth2.ClientSecret).To(Equal("**********"))
			}
			Expect(apiSubsListed).To(Equal(pagedMatchCount))
			Expect(resp.UnderscoreLinks.Next).To(BeEmpty())
			Expect(resp.UnderscoreLinks.Prev).To(HaveSuffix("?offset=480&limit=20"))
		})
	})

	Context("Event exposure subscriptions", func() {
		It("returns only exact cross-team matches from the store", func() {
			seed(storeCtx, evExposures, newFilterEventExposure("filter-event", "de.telekom.filter.v1"))
			seed(storeCtx, evSubs, newFilterEventSubscription("poc--other--teamb", "ev-match-b", "de.telekom.filter.v1"))
			seed(storeCtx, evSubs, newFilterEventSubscription("poc--eni--teamc", "ev-match-c", "de.telekom.filter.v1"))
			seed(storeCtx, evSubs, newFilterEventSubscription("poc--other--teamb", "ev-near-miss", "de.telekom.filter.v1.extra"))
			seed(storeCtx, evSubs, newFilterEventSubscription("poc--other--teamb", "ev-unrelated", "de.telekom.unrelated.v1"))

			resp, err := eventCtrl.GetSubscriptions(reqCtx, filterAppId, "filter-event", api.GetAllExposureEventSubscriptionsParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Paging.Total).To(Equal(2))
			Expect(eventSubIdentities(resp.Items)).To(ConsistOf("other/teamb/ev-match-b", "eni/teamc/ev-match-c"))
			for _, item := range resp.Items {
				Expect(item.EventType).To(Equal("de.telekom.filter.v1"))
			}
			Expect(evSubsListed).To(Equal(2))
		})

		It("returns an empty list when nothing matches", func() {
			seed(storeCtx, evExposures, newFilterEventExposure("filter-event-none", "de.telekom.none.v1"))

			resp, err := eventCtrl.GetSubscriptions(reqCtx, filterAppId, "filter-event-none", api.GetAllExposureEventSubscriptionsParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Items).To(BeEmpty())
			Expect(resp.Paging.Total).To(BeZero())
			Expect(evSubsListed).To(BeZero())
		})

		It("paginates more than one store batch of matches", func() {
			seed(storeCtx, evExposures, newFilterEventExposure("filter-event-paged", "de.telekom.paged.v1"))
			for i := range pagedMatchCount {
				seed(storeCtx, evSubs, newFilterEventSubscription("poc--other--teamb", fmt.Sprintf("ev-paged-%04d", i), "de.telekom.paged.v1"))
			}
			for i := range pagedUnrelated {
				seed(storeCtx, evSubs, newFilterEventSubscription("poc--other--teamb", fmt.Sprintf("ev-paged-other-%04d", i), "de.telekom.other.v1"))
			}

			resp, err := eventCtrl.GetSubscriptions(reqCtx, filterAppId, "filter-event-paged",
				api.GetAllExposureEventSubscriptionsParams{Offset: 500, Limit: 20})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Paging.Total).To(Equal(pagedMatchCount))
			Expect(eventSubIdentities(resp.Items)).To(Equal(pagedIdentities("ev-paged", 500, pagedMatchCount)))
			for _, item := range resp.Items {
				Expect(item.EventType).To(Equal("de.telekom.paged.v1"))
			}
			Expect(resp.UnderscoreLinks.Next).To(BeEmpty())
			Expect(resp.UnderscoreLinks.Prev).To(HaveSuffix("?offset=480&limit=20"))
			Expect(evSubsListed).To(Equal(pagedMatchCount))
		})
	})
})
