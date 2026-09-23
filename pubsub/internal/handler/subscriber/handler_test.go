// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package subscriber

import (
	"context"
	"fmt"
	"io"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	file_api "github.com/telekom/controlplane/file-manager/api"
	filefake "github.com/telekom/controlplane/file-manager/api/fake"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/pubsub/internal/service"
	"github.com/telekom/controlplane/pubsub/test/mocks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const testEnvironment = "test-env"

func newTestSubscriber() *pubsubv1.Subscriber {
	return &pubsubv1.Subscriber{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-subscriber",
			Namespace: "default",
		},
		Spec: pubsubv1.SubscriberSpec{
			Publisher: ctypes.ObjectRef{
				Name:      "test-publisher",
				Namespace: "default",
			},
			SubscriberId: "my-consumer-app",
			Delivery: pubsubv1.SubscriptionDelivery{
				Type:     pubsubv1.DeliveryTypeCallback,
				Payload:  pubsubv1.PayloadTypeData,
				Callback: "https://my-app.example.com/events",
			},
		},
	}
}

func newTestPublisher() *pubsubv1.Publisher {
	pub := &pubsubv1.Publisher{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-publisher",
			Namespace: "default",
		},
		Spec: pubsubv1.PublisherSpec{
			EventStore: ctypes.ObjectRef{
				Name:      "test-eventstore",
				Namespace: "default",
			},
			EventType:   "de.telekom.test.event.v1",
			PublisherId: "test-app",
			JsonSchema:  "schema-file-id-123",
		},
	}
	meta.SetStatusCondition(&pub.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return pub
}

func newTestEventStore() *pubsubv1.EventStore {
	es := &pubsubv1.EventStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-eventstore",
			Namespace: "default",
		},
		Spec: pubsubv1.EventStoreSpec{
			Url:          "https://config-server.example.com",
			TokenUrl:     "https://auth.example.com/token",
			ClientId:     "client-id",
			ClientSecret: "client-secret",
		},
	}
	meta.SetStatusCondition(&es.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return es
}

var _ = Describe("SubscriberHandler", func() {
	var (
		ctx             context.Context
		fakeClient      *fakeclient.MockJanitorClient
		configSvcMock   *mocks.MockConfigService
		fileManagerMock *filefake.MockFileManager
		handler         *SubscriberHandler
		origGetCfgSvc   func(*pubsubv1.EventStore) service.ConfigService
		origGetFileMgr  func() file_api.FileManager
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctx = contextutil.WithEnv(ctx, testEnvironment)

		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)

		configSvcMock = mocks.NewMockConfigService(GinkgoT())
		fileManagerMock = filefake.NewMockFileManager(GinkgoT())

		handler = &SubscriberHandler{}

		// Override getConfigService to return our mock
		origGetCfgSvc = getConfigService
		getConfigService = func(_ *pubsubv1.EventStore) service.ConfigService {
			return configSvcMock
		}

		// Override getFileManager to return our mock
		origGetFileMgr = getFileManager
		getFileManager = func() file_api.FileManager {
			return fileManagerMock
		}
	})

	AfterEach(func() {
		getConfigService = origGetCfgSvc
		getFileManager = origGetFileMgr
	})

	Describe("CreateOrUpdate", func() {
		DescribeTable("uses the effective Horizon environment for statusless IDs and payloads",
			func(storeEnvironment, expectedEnvironment, expectedID string) {
				obj := newTestSubscriber()
				publisher := newTestPublisher()
				publisher.Spec.JsonSchema = ""
				eventStore := newTestEventStore()
				eventStore.Spec.OverwriteEnvironmentName = storeEnvironment

				fakeClient.EXPECT().Get(ctx, obj.Spec.Publisher.K8s(), mock.AnythingOfType("*v1.Publisher")).
					Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
						*out.(*pubsubv1.Publisher) = *publisher
					}).Return(nil)
				fakeClient.EXPECT().Get(ctx, publisher.Spec.EventStore.K8s(), mock.AnythingOfType("*v1.EventStore")).
					Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
						*out.(*pubsubv1.EventStore) = *eventStore
					}).Return(nil)
				configSvcMock.EXPECT().PutSubscription(ctx, expectedID, mock.AnythingOfType("service.SubscriptionResource")).
					Run(func(_ context.Context, id string, resource service.SubscriptionResource) {
						Expect(resource.Metadata.Name).To(Equal(id))
						Expect(resource.Metadata.Namespace).To(Equal(service.DefaultDataplaneNamespace))
						Expect(resource.Spec.Environment).To(Equal(expectedEnvironment))
						Expect(resource.Spec.Subscription.SubscriptionId).To(Equal(id))
					}).Return(nil)

				Expect(handler.CreateOrUpdate(ctx, obj)).To(Succeed())
				Expect(obj.Status.SubscriptionId).To(Equal(expectedID))
			},
			Entry("from an EventStore override", "legacy-horizon", "legacy-horizon", "a54dad1115b720028d75c405ad33ea71928352aa"),
			Entry("from the context for a legacy empty EventStore", "", testEnvironment, GenerateSubscriptionID(testEnvironment, "de.telekom.test.event.v1", "my-consumer-app")),
		)

		It("preserves a stored ID while using the current effective environment", func() {
			obj := newTestSubscriber()
			obj.Status.SubscriptionId = "persisted-id"
			publisher := newTestPublisher()
			publisher.Spec.JsonSchema = ""
			eventStore := newTestEventStore()
			eventStore.Spec.OverwriteEnvironmentName = "changed-horizon"

			fakeClient.EXPECT().Get(ctx, obj.Spec.Publisher.K8s(), mock.AnythingOfType("*v1.Publisher")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.Publisher) = *publisher
				}).Return(nil)
			fakeClient.EXPECT().Get(ctx, publisher.Spec.EventStore.K8s(), mock.AnythingOfType("*v1.EventStore")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.EventStore) = *eventStore
				}).Return(nil)
			configSvcMock.EXPECT().PutSubscription(ctx, "persisted-id", mock.AnythingOfType("service.SubscriptionResource")).
				Run(func(_ context.Context, _ string, resource service.SubscriptionResource) {
					Expect(resource.Metadata.Name).To(Equal("persisted-id"))
					Expect(resource.Spec.Subscription.SubscriptionId).To(Equal("persisted-id"))
					Expect(resource.Spec.Environment).To(Equal("changed-horizon"))
				}).Return(nil)

			Expect(handler.CreateOrUpdate(ctx, obj)).To(Succeed())
			Expect(obj.Status.SubscriptionId).To(Equal("persisted-id"))
		})

		It("should register subscription and set Ready condition on success", func() {
			obj := newTestSubscriber()
			publisher := newTestPublisher()
			eventStore := newTestEventStore()

			By("Setting up mock expectations for GetPublisher")
			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, mock.AnythingOfType("*v1.Publisher")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.Publisher) = *publisher
				}).
				Return(nil)

			By("Setting up mock expectations for GetEventStore")
			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, mock.AnythingOfType("*v1.EventStore")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.EventStore) = *eventStore
				}).
				Return(nil)

			By("Setting up mock expectation for DownloadFile")
			fileManagerMock.EXPECT().
				DownloadFile(mock.Anything, publisher.Spec.JsonSchema, mock.Anything).
				Run(func(_ context.Context, _ string, w io.Writer) {
					_, _ = w.Write([]byte(`{"type":"object"}`))
				}).
				Return(&file_api.FileDownloadResponse{ContentType: "application/json"}, nil)

			By("Setting up mock expectation for PutSubscription")
			configSvcMock.EXPECT().
				PutSubscription(ctx, mock.AnythingOfType("string"), mock.AnythingOfType("service.SubscriptionResource")).
				Return(nil)

			By("Calling CreateOrUpdate")
			err := handler.CreateOrUpdate(ctx, obj)

			By("Verifying success")
			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.SubscriptionId).ToNot(BeEmpty())
			Expect(meta.IsStatusConditionTrue(obj.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond.Reason).To(Equal("SubscriberReady"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Done"))
		})

		It("should use existing SubscriptionId from status", func() {
			obj := newTestSubscriber()
			obj.Status.SubscriptionId = "existing-subscription-id"
			publisher := newTestPublisher()
			eventStore := newTestEventStore()

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, mock.AnythingOfType("*v1.Publisher")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.Publisher) = *publisher
				}).
				Return(nil)

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, mock.AnythingOfType("*v1.EventStore")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.EventStore) = *eventStore
				}).
				Return(nil)

			fileManagerMock.EXPECT().
				DownloadFile(mock.Anything, publisher.Spec.JsonSchema, mock.Anything).
				Run(func(_ context.Context, _ string, w io.Writer) {
					_, _ = w.Write([]byte(`{"type":"object"}`))
				}).
				Return(&file_api.FileDownloadResponse{ContentType: "application/json"}, nil)

			configSvcMock.EXPECT().
				PutSubscription(ctx, "existing-subscription-id", mock.AnythingOfType("service.SubscriptionResource")).
				Return(nil)

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.SubscriptionId).To(Equal("existing-subscription-id"))
		})

		It("should return error when Publisher cannot be resolved", func() {
			obj := newTestSubscriber()

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, mock.AnythingOfType("*v1.Publisher")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: "pubsub.cp.ei.telekom.de", Resource: "publishers"}, "test-publisher"))

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to resolve Publisher"))
		})

		It("should return error when EventStore cannot be resolved", func() {
			obj := newTestSubscriber()
			publisher := newTestPublisher()

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, mock.AnythingOfType("*v1.Publisher")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.Publisher) = *publisher
				}).
				Return(nil)

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, mock.AnythingOfType("*v1.EventStore")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: "pubsub.cp.ei.telekom.de", Resource: "eventstores"}, "test-eventstore"))

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to resolve EventStore"))
		})

		It("should return error when PutSubscription fails", func() {
			obj := newTestSubscriber()
			publisher := newTestPublisher()
			eventStore := newTestEventStore()

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, mock.AnythingOfType("*v1.Publisher")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.Publisher) = *publisher
				}).
				Return(nil)

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, mock.AnythingOfType("*v1.EventStore")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.EventStore) = *eventStore
				}).
				Return(nil)

			fileManagerMock.EXPECT().
				DownloadFile(mock.Anything, publisher.Spec.JsonSchema, mock.Anything).
				Run(func(_ context.Context, _ string, w io.Writer) {
					_, _ = w.Write([]byte(`{"type":"object"}`))
				}).
				Return(&file_api.FileDownloadResponse{ContentType: "application/json"}, nil)

			configSvcMock.EXPECT().
				PutSubscription(ctx, mock.AnythingOfType("string"), mock.AnythingOfType("service.SubscriptionResource")).
				Return(fmt.Errorf("connection timeout"))

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to register subscription"))
		})
	})

	Describe("Delete", func() {
		DescribeTable("uses the effective Horizon environment for deletion",
			func(storeEnvironment, storedID, expectedEnvironment, expectedID string) {
				obj := newTestSubscriber()
				obj.Status.SubscriptionId = storedID
				publisher := newTestPublisher()
				eventStore := newTestEventStore()
				eventStore.Spec.OverwriteEnvironmentName = storeEnvironment

				fakeClient.EXPECT().Get(ctx, obj.Spec.Publisher.K8s(), mock.AnythingOfType("*v1.Publisher")).
					Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
						*out.(*pubsubv1.Publisher) = *publisher
					}).Return(nil)
				fakeClient.EXPECT().Get(ctx, publisher.Spec.EventStore.K8s(), mock.AnythingOfType("*v1.EventStore")).
					Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
						*out.(*pubsubv1.EventStore) = *eventStore
					}).Return(nil)
				configSvcMock.EXPECT().DeleteSubscription(ctx, expectedID, mock.AnythingOfType("service.SubscriptionResource")).
					Run(func(_ context.Context, id string, resource service.SubscriptionResource) {
						Expect(resource.Metadata.Name).To(Equal(id))
						Expect(resource.Metadata.Namespace).To(Equal(service.DefaultDataplaneNamespace))
						Expect(resource.Spec.Environment).To(Equal(expectedEnvironment))
						Expect(resource.Spec.Subscription.SubscriptionId).To(Equal(id))
					}).Return(nil)

				Expect(handler.Delete(ctx, obj)).To(Succeed())
			},
			Entry("for a statusless subscription", "legacy-horizon", "", "legacy-horizon", "a54dad1115b720028d75c405ad33ea71928352aa"),
			Entry("for a legacy empty store", "", "", testEnvironment, GenerateSubscriptionID(testEnvironment, "de.telekom.test.event.v1", "my-consumer-app")),
			Entry("while preserving a stored ID after an environment change", "changed-horizon", "persisted-id", "changed-horizon", "persisted-id"),
		)

		It("should deregister subscription on success", func() {
			obj := newTestSubscriber()
			publisher := newTestPublisher()
			eventStore := newTestEventStore()

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, mock.AnythingOfType("*v1.Publisher")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.Publisher) = *publisher
				}).
				Return(nil)

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, mock.AnythingOfType("*v1.EventStore")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.EventStore) = *eventStore
				}).
				Return(nil)

			configSvcMock.EXPECT().
				DeleteSubscription(ctx, mock.AnythingOfType("string"), mock.AnythingOfType("service.SubscriptionResource")).
				Return(nil)

			err := handler.Delete(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should skip cleanup when Publisher is already deleted", func() {
			obj := newTestSubscriber()

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, mock.AnythingOfType("*v1.Publisher")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: "pubsub.cp.ei.telekom.de", Resource: "publishers"}, "test-publisher"))

			err := handler.Delete(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail cleanup when EventStore is already deleted", func() {
			obj := newTestSubscriber()
			publisher := newTestPublisher()

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, mock.AnythingOfType("*v1.Publisher")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.Publisher) = *publisher
				}).
				Return(nil)

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, mock.AnythingOfType("*v1.EventStore")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: "pubsub.cp.ei.telekom.de", Resource: "eventstores"}, "test-eventstore"))

			err := handler.Delete(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to resolve EventStore"))
		})

		It("should return error when Publisher Get fails with unexpected error", func() {
			obj := newTestSubscriber()

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, mock.AnythingOfType("*v1.Publisher")).
				Return(fmt.Errorf("connection refused"))

			err := handler.Delete(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to resolve Publisher"))
		})

		It("should return error when EventStore Get fails with unexpected error", func() {
			obj := newTestSubscriber()
			publisher := newTestPublisher()

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, mock.AnythingOfType("*v1.Publisher")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.Publisher) = *publisher
				}).
				Return(nil)

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, mock.AnythingOfType("*v1.EventStore")).
				Return(fmt.Errorf("connection refused"))

			err := handler.Delete(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to resolve EventStore"))
		})

		It("should return error when DeleteSubscription fails", func() {
			obj := newTestSubscriber()
			publisher := newTestPublisher()
			eventStore := newTestEventStore()

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, mock.AnythingOfType("*v1.Publisher")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.Publisher) = *publisher
				}).
				Return(nil)

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, mock.AnythingOfType("*v1.EventStore")).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.EventStore) = *eventStore
				}).
				Return(nil)

			configSvcMock.EXPECT().
				DeleteSubscription(ctx, mock.AnythingOfType("string"), mock.AnythingOfType("service.SubscriptionResource")).
				Return(fmt.Errorf("service unavailable"))

			err := handler.Delete(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to deregister subscription"))
		})
	})
})
