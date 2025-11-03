// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder_test

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/types"

	"github.com/telekom/controlplane/common/pkg/test"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/api/v1/builder"
)

var _ = Describe("NotificationBuilder", func() {

	var k8sScheme *runtime.Scheme
	var ctx context.Context
	var fakeClient *fakeclient.MockJanitorClient

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)

		k8sScheme = runtime.NewScheme()
		err := notificationv1.AddToScheme(k8sScheme)
		Expect(err).NotTo(HaveOccurred())
		err = test.AddToScheme(k8sScheme)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("Basic builder functions", func() {
		It("creates a new notification builder", func() {
			notificationBuilder := builder.New()
			Expect(notificationBuilder).NotTo(BeNil())
		})
	})

	Context("WithNamespace", func() {
		It("sets the namespace correctly", func() {
			notificationBuilder := builder.New().WithNamespace("test-namespace")
			notificationBuilder = notificationBuilder.WithPurpose("test-purpose")                                               // Needed to pass validation
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test")                       // Needed to pass validation
			notification, err := notificationBuilder.Build(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(notification.Namespace).To(Equal("test-namespace"))
		})

		It("does not override an already set namespace", func() {
			notificationBuilder := builder.New().WithNamespace("first-namespace").WithNamespace("second-namespace")
			notificationBuilder = notificationBuilder.WithPurpose("test-purpose")                                               // Needed to pass validation
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test")                       // Needed to pass validation
			notification, err := notificationBuilder.Build(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(notification.Namespace).To(Equal("first-namespace"))
		})
	})

	Context("WithOwner", func() {
		It("handles nil owner", func() {
			notificationBuilder := builder.New().WithOwner(nil)
			notificationBuilder = notificationBuilder.WithNamespace("test-namespace").WithPurpose("test-purpose")               // Needed to pass validation
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test")                       // Needed to pass validation
			notification, err := notificationBuilder.Build(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(notification.Namespace).To(Equal("test-namespace"))
		})

		It("sets owner information", func() {
			owner := test.NewObject("owner-name", "owner-namespace")
			owner.SetLabels(map[string]string{"owner-label-key": "owner-label-value"})
			owner.SetAnnotations(map[string]string{"owner-annotation-key": "owner-annotation-value"})

			notificationBuilder := builder.New().WithOwner(owner)
			notificationBuilder = notificationBuilder.WithPurpose("test-purpose")                                               // Needed to pass validation
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test")                       // Needed to pass validation
			notification, err := notificationBuilder.Build(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(notification.Namespace).To(Equal("owner-namespace"))
			Expect(notification.Labels).NotTo(BeNil())
			Expect(notification.Labels).To(HaveKeyWithValue("owner-label-key", "owner-label-value"))
			Expect(notification.Annotations).NotTo(BeNil())
			Expect(notification.Annotations).To(HaveKeyWithValue("owner-annotation-key", "owner-annotation-value"))
		})
	})

	Context("WithPurpose", func() {
		It("sets the purpose correctly", func() {
			notificationBuilder := builder.New().WithNamespace("test-namespace").WithPurpose("test-purpose")
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test")                       // Needed to pass validation
			notification, err := notificationBuilder.Build(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(notification.Spec.Purpose).To(Equal("test-purpose"))
		})

		It("requires a purpose to build", func() {
			notificationBuilder := builder.New().WithNamespace("test-namespace")
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test")                       // Needed to pass validation
			_, err := notificationBuilder.Build(ctx)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("purpose is required"))
		})
	})

	Context("WithSender", func() {
		It("sets the sender correctly for system sender", func() {
			notificationBuilder := builder.New().WithNamespace("test-namespace").WithPurpose("test-purpose").WithSender(notificationv1.SenderTypeSystem, "system-name")
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation

			notification, err := notificationBuilder.Build(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(notification.Spec.Sender.Type).To(Equal(notificationv1.SenderTypeSystem))
			Expect(notification.Spec.Sender.Name).To(Equal("system-name"))
		})

		It("sets the sender correctly for user sender", func() {
			notificationBuilder := builder.New().WithNamespace("test-namespace").WithPurpose("test-purpose").WithSender(notificationv1.SenderTypeUser, "user-name")
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation

			notification, err := notificationBuilder.Build(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(notification.Spec.Sender.Type).To(Equal(notificationv1.SenderTypeUser))
			Expect(notification.Spec.Sender.Name).To(Equal("user-name"))
		})
	})

	Context("WithChannels", func() {
		It("adds channels correctly", func() {
			notificationBuilder := builder.New().WithNamespace("test-namespace").WithPurpose("test-purpose").
				WithChannels(types.ObjectRef{Name: "channel1", Namespace: "default"}, types.ObjectRef{Name: "channel2", Namespace: "default"})
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test") // Needed to pass validation

			notification, err := notificationBuilder.Build(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(notification.Spec.Channels).To(ContainElements(types.ObjectRef{Name: "channel1", Namespace: "default"}, types.ObjectRef{Name: "channel2", Namespace: "default"}))
		})

		It("allows adding multiple channels in sequence", func() {
			notificationBuilder := builder.New().WithNamespace("test-namespace").WithPurpose("test-purpose")
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "channel1", Namespace: "default"})
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "channel2", Namespace: "default"}, types.ObjectRef{Name: "channel3", Namespace: "default"})
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test") // Needed to pass validation

			notification, err := notificationBuilder.Build(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(notification.Spec.Channels).To(ContainElements(
				types.ObjectRef{Name: "channel1", Namespace: "default"},
				types.ObjectRef{Name: "channel2", Namespace: "default"},
				types.ObjectRef{Name: "channel3", Namespace: "default"},
			))
			Expect(len(notification.Spec.Channels)).To(Equal(3))
		})
	})

	Context("WithDefaultChannels", func() {
		It("adds all channels from the namespace", func() {
			// Setup the expected response for the List operation
			channelList := &notificationv1.NotificationChannelList{
				Items: []notificationv1.NotificationChannel{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "channel1", Namespace: "test-namespace"},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "channel2", Namespace: "test-namespace"},
					},
				},
			}

			// Setup the mock expectation
			returnChannels := func(_ context.Context, objList client.ObjectList, listOpts ...client.ListOption) error {
				Expect(listOpts).To(HaveLen(1))
				Expect(listOpts[0]).To(Equal(client.InNamespace("test-namespace")))

				if n, ok := objList.(*notificationv1.NotificationChannelList); ok {
					n.Items = channelList.Items
					return nil
				}
				return errors.New("unexpected type")
			}

			fakeClient.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.NotificationChannelList"), mock.Anything).
				RunAndReturn(returnChannels).Once()

			// Execute the function under test
			notificationBuilder := builder.New().WithNamespace("test-namespace").WithPurpose("test-purpose")
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test") // Needed to pass validation
			notificationBuilder = notificationBuilder.WithDefaultChannels(ctx, "test-namespace")

			notification, err := notificationBuilder.Build(ctx)

			// Verify results
			Expect(err).NotTo(HaveOccurred())
			Expect(notification.Spec.Channels).To(ContainElements(types.ObjectRef{Name: "channel1", Namespace: "test-namespace"}, types.ObjectRef{Name: "channel2", Namespace: "test-namespace"}))
			Expect(len(notification.Spec.Channels)).To(Equal(2))

			// Verify mock expectations were met
			fakeClient.AssertExpectations(GinkgoT())
		})

		It("handles error from client", func() {
			// Setup the mock to return an error
			testErr := errors.New("mock list error")
			fakeClient.On("List", mock.Anything, &notificationv1.NotificationChannelList{}, mock.Anything).Return(testErr).Once()

			// Execute the function under test
			notificationBuilder := builder.New().WithNamespace("test-namespace").WithPurpose("test-purpose")
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test") // Needed to pass validation
			notificationBuilder = notificationBuilder.WithDefaultChannels(ctx, "test-namespace")

			// The error should be stored in the builder
			_, err := notificationBuilder.Build(ctx)

			// Verify results
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list notification channels"))

			// Verify mock expectations were met
			fakeClient.AssertExpectations(GinkgoT())
		})
	})

	Context("WithProperties", func() {
		It("adds properties correctly", func() {
			properties := map[string]any{
				"key1": "value1",
				"key2": 42,
				"key3": true,
				"nested": map[string]any{
					"nestedKey": "nestedValue",
				},
			}

			notificationBuilder := builder.New().WithNamespace("test-namespace").WithPurpose("test-purpose")
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test")                       // Needed to pass validation
			notificationBuilder = notificationBuilder.WithProperties(properties)

			notification, err := notificationBuilder.Build(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Verify properties are stored correctly
			Expect(notification.Spec.Properties.Raw).NotTo(BeNil())

			// Parse and verify the properties
			parsedProperties := make(map[string]any)
			err = json.Unmarshal(notification.Spec.Properties.Raw, &parsedProperties)
			Expect(err).NotTo(HaveOccurred())
			Expect(parsedProperties["key1"]).To(Equal("value1"))
			Expect(parsedProperties["key2"]).To(Equal(float64(42))) // JSON numbers are float64
			Expect(parsedProperties["key3"]).To(BeTrue())
			Expect(parsedProperties["nested"]).To(HaveKeyWithValue("nestedKey", "nestedValue"))
		})

		It("handles error with invalid properties", func() {
			// Create an invalid property that can't be marshaled to JSON
			invalidProperties := map[string]any{
				"invalidKey": func() {}, // Functions can't be marshaled to JSON
			}

			notificationBuilder := builder.New().WithNamespace("test-namespace").WithPurpose("test-purpose")
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test")                       // Needed to pass validation
			notificationBuilder = notificationBuilder.WithProperties(invalidProperties)

			_, err := notificationBuilder.Build(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to marshal properties"))
		})
	})

	Context("Build", func() {
		It("requires namespace to build", func() {
			// Only set purpose, missing namespace
			notificationBuilder := builder.New().WithPurpose("test-purpose")
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test")                       // Needed to pass validation
			_, err := notificationBuilder.Build(ctx)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("namespace is required"))
		})

		It("requires purpose to build", func() {
			// Only set namespace, missing purpose
			notificationBuilder := builder.New().WithNamespace("test-namespace")
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test")                       // Needed to pass validation
			_, err := notificationBuilder.Build(ctx)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("purpose is required"))
		})

		It("returns the first error encountered", func() {
			properties := map[string]any{
				"invalidKey": func() {}, // Functions can't be marshaled to JSON
			}

			// This will fail at WithProperties and should return that error even though namespace and purpose are missing
			notificationBuilder := builder.New().WithProperties(properties)
			_, err := notificationBuilder.Build(ctx)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to marshal properties"))
		})

		It("builds a valid notification with all required fields", func() {
			// Need to call Build twice to handle the name generation logic
			notificationBuilder := builder.New().WithNamespace("test-namespace").WithPurpose("test-purpose")
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test")                       // Needed to pass validation
			// First build sets the built flag
			notification, err := notificationBuilder.Build(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(notification).NotTo(BeNil())
			Expect(notification.Namespace).To(Equal("test-namespace"))
			Expect(notification.Spec.Purpose).To(Equal("test-purpose"))

			// Second build should set the name
			notification, err = notificationBuilder.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(notification.Name).NotTo(BeEmpty()) // Name should be generated now
		})

		It("sets the name based on purpose and spec hash", func() {
			// Need to call Build twice to handle the name generation logic
			notificationBuilder := builder.New().WithNamespace("test-namespace").WithPurpose("test-purpose")
			notificationBuilder = notificationBuilder.WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}) // Needed to pass validation
			notificationBuilder = notificationBuilder.WithSender(notificationv1.SenderTypeSystem, "test")                       // Needed to pass validation
			// First build sets the built flag
			_, err := notificationBuilder.Build(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Second build should set the name
			notification, err := notificationBuilder.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(notification.Name).To(HavePrefix("test-purpose--"))

			// The name should contain a hash, which means it should be longer than just the prefix
			Expect(len(notification.Name)).To(BeNumerically(">", len("test-purpose--")))
		})
	})

	Context("Send", func() {

		It("creates a notification resource via the k8s client", func() {
			// Setup the expected notification
			notificationBuilder := builder.New().
				WithOwner(test.NewObject("owner-name", "owner-namespace")).
				WithNamespace("test-namespace").
				WithPurpose("test-purpose").
				WithChannels(types.ObjectRef{Name: "test-channel", Namespace: "default"}).
				WithSender(notificationv1.SenderTypeSystem, "test")

			// Build the notification first
			_, err := notificationBuilder.Build(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Setup the mock expectations
			// We need to setup the expected behavior for CreateOrUpdate
			returnSuccess := func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				// Call the mutate function to ensure it works correctly
				err := mutate()
				Expect(err).NotTo(HaveOccurred())

				// Verify that the object is the correct type and contains the expected values
				notification, ok := obj.(*notificationv1.Notification)
				Expect(ok).To(BeTrue())
				Expect(notification.Namespace).To(Equal("owner-namespace"))
				Expect(notification.Spec.Purpose).To(Equal("test-purpose"))
				Expect(notification.Spec.Channels).To(ContainElement(types.ObjectRef{Name: "test-channel", Namespace: "default"}))
				Expect(notification.Spec.Sender.Type).To(Equal(notificationv1.SenderTypeSystem))
				Expect(notification.Spec.Sender.Name).To(Equal("test"))

				Expect(notification.ObjectMeta.OwnerReferences[0].Name).To(Equal("owner-name"))
				Expect(notification.ObjectMeta.OwnerReferences[0].Kind).To(Equal("TestResource"))

				// Verify labels are set
				Expect(notification.Labels).NotTo(BeNil())

				return controllerutil.OperationResultCreated, nil
			}

			fakeClient.EXPECT().
				Scheme().Return(k8sScheme).Once() // Scheme is called to setup owner reference

			fakeClient.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.Notification"), mock.Anything).
				RunAndReturn(returnSuccess).Once()

			// Send the notification
			notification, err := notificationBuilder.Send(ctx)

			// Verify the result
			Expect(err).NotTo(HaveOccurred())
			Expect(notification).NotTo(BeNil())
			Expect(notification.Namespace).To(Equal("owner-namespace"))
			Expect(notification.Spec.Purpose).To(Equal("test-purpose"))
			Expect(notification.Spec.Channels).To(ContainElement(types.ObjectRef{Name: "test-channel", Namespace: "default"}))
			Expect(notification.Spec.Sender.Type).To(Equal(notificationv1.SenderTypeSystem))
			Expect(notification.Spec.Sender.Name).To(Equal("test"))

			// Verify mock expectations were met
			fakeClient.AssertExpectations(GinkgoT())
		})
	})

})
