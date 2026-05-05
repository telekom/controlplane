// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"
	"fmt"
	texttemplate "text/template"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	handlers "github.com/telekom/controlplane/notification/internal/handler"
	"github.com/telekom/controlplane/notification/internal/sender/adapter"
	"github.com/telekom/controlplane/notification/internal/templatecache"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockSender implements sender.NotificationSender for testing.
type mockSender struct {
	err       error
	callCount int
	lastArgs  mockSenderArgs
}

type mockSenderArgs struct {
	subject     string
	body        string
	attachments []adapter.Attachment
}

func (m *mockSender) ProcessNotification(_ context.Context, _ *notificationv1.NotificationChannel, subject, body string, attachments []adapter.Attachment) error {
	m.callCount++
	m.lastArgs = mockSenderArgs{subject: subject, body: body, attachments: attachments}
	return m.err
}

func newNotification(purpose string, channels []commontypes.ObjectRef, properties string) *notificationv1.Notification {
	return &notificationv1.Notification{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "n1",
			Namespace: "default",
		},
		Spec: notificationv1.NotificationSpec{
			Purpose: purpose,
			Sender: notificationv1.Sender{
				Type: notificationv1.SenderTypeUser,
				Name: "test-user",
			},
			Channels: channels,
			Properties: runtime.RawExtension{
				Raw: []byte(properties),
			},
		},
	}
}

func newReadyEmailChannel(recipients []string) *notificationv1.NotificationChannel {
	ch := &notificationv1.NotificationChannel{
		ObjectMeta: metav1.ObjectMeta{Name: "team--mail", Namespace: "default"},
		Spec:       notificationv1.NotificationChannelSpec{Email: &notificationv1.EmailConfig{Recipients: recipients}},
	}
	ch.Status.Conditions = []metav1.Condition{{
		Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(), Reason: "Provisioned", Message: "Ready",
	}}
	return ch
}

func newReadyChatChannel(name, namespace, webhookURL string) *notificationv1.NotificationChannel {
	ch := &notificationv1.NotificationChannel{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       notificationv1.NotificationChannelSpec{MsTeams: &notificationv1.MsTeamsConfig{WebhookURL: webhookURL}},
	}
	ch.Status.Conditions = []metav1.Condition{{
		Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(), Reason: "Provisioned", Message: "Ready",
	}}
	return ch
}

func newTemplateWrapper(subject, body string) *templatecache.TemplateWrapper {
	return &templatecache.TemplateWrapper{
		SubjectTemplate: texttemplate.Must(texttemplate.New("subject").Parse(subject)),
		BodyTemplate:    texttemplate.Must(texttemplate.New("body").Parse(body)),
	}
}

// newReadyNotificationTemplate returns a k8s NotificationTemplate with a Ready condition.
func newReadyNotificationTemplate(name, namespace string) *notificationv1.NotificationTemplate {
	return &notificationv1.NotificationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: notificationv1.NotificationTemplateSpec{
			Purpose:         "test",
			ChannelType:     "mail",
			SubjectTemplate: "subj",
			Template:        "body",
			Schema:          runtime.RawExtension{Raw: []byte(`{}`)},
		},
		Status: notificationv1.NotificationTemplateStatus{
			Conditions: []metav1.Condition{{
				Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(), Reason: "Provisioned", Message: "Ready",
			}},
		},
	}
}

func channelRef(name string) commontypes.ObjectRef {
	return commontypes.ObjectRef{Name: name, Namespace: "default"}
}

func setupCtx(fakeClient *fakeclient.MockJanitorClient) context.Context {
	ctx := context.Background()
	ctx = cclient.WithClient(ctx, fakeClient)
	ctx = contextutil.WithEnv(ctx, testEnvironment)
	return ctx
}

// expectGetChannel sets up a mock for fetching a NotificationChannel by ref.
func expectGetChannel(fc *fakeclient.MockJanitorClient, ch *notificationv1.NotificationChannel) {
	fc.EXPECT().
		Get(mock.Anything, k8stypes.NamespacedName{Name: ch.Name, Namespace: ch.Namespace}, mock.AnythingOfType("*v1.NotificationChannel"), mock.Anything).
		Run(func(_ context.Context, _ k8stypes.NamespacedName, obj k8sclient.Object, _ ...k8sclient.GetOption) {
			*obj.(*notificationv1.NotificationChannel) = *ch
		}).
		Return(nil).Once()
}

// expectGetChannelError sets up a mock for a failing channel fetch.
func expectGetChannelError(fc *fakeclient.MockJanitorClient, name, namespace string, err error) {
	fc.EXPECT().
		Get(mock.Anything, k8stypes.NamespacedName{Name: name, Namespace: namespace}, mock.AnythingOfType("*v1.NotificationChannel"), mock.Anything).
		Return(err).Once()
}

// expectGetTemplate sets up a mock for fetching a NotificationTemplate (used by resolveTemplate's else branch).
func expectGetTemplate(fc *fakeclient.MockJanitorClient, templateName string, tpl *notificationv1.NotificationTemplate) {
	fc.EXPECT().
		Get(mock.Anything, k8stypes.NamespacedName{Name: templateName, Namespace: testEnvironment}, mock.AnythingOfType("*v1.NotificationTemplate"), mock.Anything).
		Run(func(_ context.Context, _ k8stypes.NamespacedName, obj k8sclient.Object, _ ...k8sclient.GetOption) {
			*obj.(*notificationv1.NotificationTemplate) = *tpl
		}).
		Return(nil).Once()
}

// expectGetTemplateError sets up a mock for a failing template fetch.
func expectGetTemplateError(fc *fakeclient.MockJanitorClient, templateName string, err error) {
	fc.EXPECT().
		Get(mock.Anything, k8stypes.NamespacedName{Name: templateName, Namespace: testEnvironment}, mock.AnythingOfType("*v1.NotificationTemplate"), mock.Anything).
		Return(err).Once()
}

var _ = Describe("Notification Handler - Unit Tests", func() {
	var (
		fakeClient *fakeclient.MockJanitorClient
		ctx        context.Context
		mSender    *mockSender
		cache      *templatecache.TemplateCache
		handler    *handlers.NotificationHandler
	)

	BeforeEach(func() {
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = setupCtx(fakeClient)
		mSender = &mockSender{}
		cache = templatecache.New()
		handler = &handlers.NotificationHandler{
			NotificationSender: mSender,
			TemplateCache:      cache,
		}
	})

	// --- alreadySent ---

	Context("when a channel was already successfully sent", func() {
		It("should skip the channel and not call sender again", func() {
			ref := channelRef("team--mail")
			notification := newNotification("test", []commontypes.ObjectRef{ref}, `{"key":"val"}`)
			notification.Status.States = map[string]notificationv1.SendState{
				"default/team--mail": {Sent: true, ErrorMessage: "Successfully sent"},
			}

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(mSender.callCount).To(Equal(0))
			Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		})
	})

	// --- getChannelByRef errors ---

	Context("when the channel cannot be fetched", func() {
		It("should block and record error in status", func() {
			ref := channelRef("missing-channel")
			notification := newNotification("test", []commontypes.ObjectRef{ref}, `{"key":"val"}`)

			expectGetChannelError(fakeClient, "missing-channel", "default", fmt.Errorf("not found"))

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(mSender.callCount).To(Equal(0))

			Expect(notification.Status.States).To(HaveKey("default/missing-channel"))
			Expect(notification.Status.States["default/missing-channel"].Sent).To(BeFalse())
			Expect(notification.Status.States["default/missing-channel"].ErrorMessage).To(ContainSubstring("not found"))
		})
	})

	Context("when the channel is not ready", func() {
		It("should block and record error in status", func() {
			ref := channelRef("not-ready-ch")
			notification := newNotification("test", []commontypes.ObjectRef{ref}, `{"key":"val"}`)

			notReadyCh := &notificationv1.NotificationChannel{
				ObjectMeta: metav1.ObjectMeta{Name: "not-ready-ch", Namespace: "default"},
				Spec:       notificationv1.NotificationChannelSpec{Email: &notificationv1.EmailConfig{Recipients: []string{"a@b.c"}}},
			}
			// No ready condition — EnsureReady will fail

			expectGetChannel(fakeClient, notReadyCh)

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(mSender.callCount).To(Equal(0))

			Expect(notification.Status.States["default/not-ready-ch"].Sent).To(BeFalse())
			Expect(notification.Status.States["default/not-ready-ch"].ErrorMessage).To(ContainSubstring("not ready"))
		})
	})

	// --- resolveTemplate: template not in cache ---

	Context("when template is not in cache", func() {
		It("should block and record error in status", func() {
			channel := newReadyEmailChannel([]string{"a@b.c"})
			ref := channelRef("team--mail")
			notification := newNotification("my-purpose", []commontypes.ObjectRef{ref}, `{"key":"val"}`)

			expectGetChannel(fakeClient, channel)
			// No cache entry — resolveTemplate returns error immediately (the !ok branch)

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(mSender.callCount).To(Equal(0))

			Expect(notification.Status.States["default/team--mail"].Sent).To(BeFalse())
			Expect(notification.Status.States["default/team--mail"].ErrorMessage).To(ContainSubstring("No template found in cache"))
		})
	})

	// --- resolveTemplate: cache hit, but k8s template not found ---

	Context("when template is in cache but k8s template Get fails", func() {
		It("should block and record error in status", func() {
			channel := newReadyEmailChannel([]string{"a@b.c"})
			ref := channelRef("team--mail")
			notification := newNotification("my-purpose", []commontypes.ObjectRef{ref}, `{"key":"val"}`)

			cache.Set("my-purpose--mail", newTemplateWrapper("subj", "body"))

			expectGetChannel(fakeClient, channel)
			expectGetTemplateError(fakeClient, "my-purpose--mail", fmt.Errorf("template not found in k8s"))

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(mSender.callCount).To(Equal(0))

			Expect(notification.Status.States["default/team--mail"].Sent).To(BeFalse())
			Expect(notification.Status.States["default/team--mail"].ErrorMessage).To(ContainSubstring("template not found in k8s"))
		})
	})

	// --- Happy path: cache hit, k8s template ready, sender succeeds ---

	Context("when template is cached and sender succeeds", func() {
		It("should send the notification and mark as ready", func() {
			channel := newReadyEmailChannel([]string{"a@b.c"})
			ref := channelRef("team--mail")
			notification := newNotification("welcome", []commontypes.ObjectRef{ref},
				`{"subject":"Hello","body":"World"}`)

			cache.Set("welcome--mail", newTemplateWrapper("Subject: {{.subject}}", "Body: {{.body}}"))

			expectGetChannel(fakeClient, channel)
			expectGetTemplate(fakeClient, "welcome--mail", newReadyNotificationTemplate("welcome--mail", testEnvironment))

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(mSender.callCount).To(Equal(1))
			Expect(mSender.lastArgs.subject).To(Equal("Subject: Hello"))
			Expect(mSender.lastArgs.body).To(Equal("Body: World"))

			Expect(notification.Status.States["default/team--mail"].Sent).To(BeTrue())
			Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		})
	})

	// --- Sender failure: sets retrying condition ---

	Context("when sender returns an error", func() {
		It("should record the error and set retrying condition", func() {
			channel := newReadyEmailChannel([]string{"a@b.c"})
			ref := channelRef("team--mail")
			notification := newNotification("welcome", []commontypes.ObjectRef{ref},
				`{"subject":"Hello","body":"World"}`)

			cache.Set("welcome--mail", newTemplateWrapper("{{.subject}}", "{{.body}}"))
			mSender.err = fmt.Errorf("SMTP timeout")

			expectGetChannel(fakeClient, channel)
			expectGetTemplate(fakeClient, "welcome--mail", newReadyNotificationTemplate("welcome--mail", testEnvironment))

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())

			Expect(notification.Status.States["default/team--mail"].Sent).To(BeFalse())
			Expect(notification.Status.States["default/team--mail"].ErrorMessage).To(ContainSubstring("SMTP timeout"))
			Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeReady)).To(BeFalse())
		})
	})

	// --- Multiple channels: partial success ---

	Context("when one channel succeeds and another cannot be fetched", func() {
		It("should record both results", func() {
			emailCh := newReadyEmailChannel([]string{"a@b.c"})

			notification := newNotification("welcome",
				[]commontypes.ObjectRef{
					channelRef("team--mail"),
					channelRef("team--chat"),
				},
				`{"subject":"Hi","body":"There"}`)

			cache.Set("welcome--mail", newTemplateWrapper("{{.subject}}", "{{.body}}"))

			expectGetChannel(fakeClient, emailCh)
			expectGetTemplate(fakeClient, "welcome--mail", newReadyNotificationTemplate("welcome--mail", testEnvironment))
			expectGetChannelError(fakeClient, "team--chat", "default", fmt.Errorf("channel not found"))

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())

			Expect(notification.Status.States["default/team--mail"].Sent).To(BeTrue())
			Expect(notification.Status.States["default/team--chat"].Sent).To(BeFalse())
			Expect(notification.Status.States["default/team--chat"].ErrorMessage).To(ContainSubstring("channel not found"))
		})
	})

	// --- Invalid JSON properties ---

	Context("when notification properties have invalid JSON", func() {
		It("should record unmarshal error in status", func() {
			channel := newReadyEmailChannel([]string{"a@b.c"})
			ref := channelRef("team--mail")
			notification := newNotification("welcome", []commontypes.ObjectRef{ref}, `{invalid json}`)

			cache.Set("welcome--mail", newTemplateWrapper("{{.subject}}", "{{.body}}"))

			expectGetChannel(fakeClient, channel)
			expectGetTemplate(fakeClient, "welcome--mail", newReadyNotificationTemplate("welcome--mail", testEnvironment))

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(mSender.callCount).To(Equal(0))

			Expect(notification.Status.States["default/team--mail"].Sent).To(BeFalse())
			Expect(notification.Status.States["default/team--mail"].ErrorMessage).To(ContainSubstring("unmarshal"))
		})
	})

	// --- Subject template render error ---

	Context("when subject template rendering fails", func() {
		It("should record the render error", func() {
			channel := newReadyEmailChannel([]string{"a@b.c"})
			ref := channelRef("team--mail")
			notification := newNotification("welcome", []commontypes.ObjectRef{ref}, `{"subject":"val"}`)

			cache.Set("welcome--mail", &templatecache.TemplateWrapper{
				SubjectTemplate: texttemplate.Must(texttemplate.New("subject").Parse("{{ call .subject }}")),
				BodyTemplate:    texttemplate.Must(texttemplate.New("body").Parse("ok")),
			})

			expectGetChannel(fakeClient, channel)
			expectGetTemplate(fakeClient, "welcome--mail", newReadyNotificationTemplate("welcome--mail", testEnvironment))

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(mSender.callCount).To(Equal(0))

			Expect(notification.Status.States["default/team--mail"].Sent).To(BeFalse())
			Expect(notification.Status.States["default/team--mail"].ErrorMessage).To(ContainSubstring("failed to execute template"))
		})
	})

	// --- Attachments are rendered and forwarded ---

	Context("when template has attachments", func() {
		It("should render attachments and pass them to sender", func() {
			channel := newReadyEmailChannel([]string{"a@b.c"})
			ref := channelRef("team--mail")
			notification := newNotification("invite", []commontypes.ObjectRef{ref}, `{"name":"Alice"}`)

			wrapper := newTemplateWrapper("subj", "body")
			wrapper.Attachments = []templatecache.ParsedAttachment{{
				Filename:        "invite.txt",
				ContentType:     "text/plain",
				ContentTemplate: texttemplate.Must(texttemplate.New("att-0").Parse("Hello {{.name}}")),
			}}
			cache.Set("invite--mail", wrapper)

			expectGetChannel(fakeClient, channel)
			expectGetTemplate(fakeClient, "invite--mail", newReadyNotificationTemplate("invite--mail", testEnvironment))

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(mSender.callCount).To(Equal(1))
			Expect(mSender.lastArgs.attachments).To(HaveLen(1))
			Expect(mSender.lastArgs.attachments[0].Filename).To(Equal("invite.txt"))
			Expect(mSender.lastArgs.attachments[0].ContentType).To(Equal("text/plain"))
			Expect(string(mSender.lastArgs.attachments[0].Content)).To(Equal("Hello Alice"))
		})
	})

	// --- toAdapterAttachments: nil when no attachments ---

	Context("when template has no attachments", func() {
		It("should pass nil attachments to sender", func() {
			channel := newReadyEmailChannel([]string{"a@b.c"})
			ref := channelRef("team--mail")
			notification := newNotification("simple", []commontypes.ObjectRef{ref}, `{"key":"val"}`)

			cache.Set("simple--mail", newTemplateWrapper("subj", "body"))

			expectGetChannel(fakeClient, channel)
			expectGetTemplate(fakeClient, "simple--mail", newReadyNotificationTemplate("simple--mail", testEnvironment))

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(mSender.callCount).To(Equal(1))
			Expect(mSender.lastArgs.attachments).To(BeNil())
		})
	})

	// --- Delete ---

	Describe("Delete", func() {
		It("should return nil without error", func() {
			notification := newNotification("test", nil, `{}`)
			err := handler.Delete(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// --- hasFailedSendAttempt (tested via conditions) ---

	Context("when one channel was already sent and another fails to resolve", func() {
		It("should set blocked condition", func() {
			notification := newNotification("test",
				[]commontypes.ObjectRef{channelRef("ch-ok"), channelRef("ch-fail")},
				`{"key":"val"}`)

			notification.Status.States = map[string]notificationv1.SendState{
				"default/ch-ok": {Sent: true, ErrorMessage: "Successfully sent"},
			}

			expectGetChannelError(fakeClient, "ch-fail", "default", fmt.Errorf("not found"))

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())

			Expect(notification.Status.States["default/ch-ok"].Sent).To(BeTrue())
			Expect(notification.Status.States["default/ch-fail"].Sent).To(BeFalse())
		})
	})

	// --- addResultToStatus: initializes nil map ---

	Context("when status.States is nil", func() {
		It("should initialize the map and add the result", func() {
			channel := newReadyEmailChannel([]string{"a@b.c"})
			ref := channelRef("team--mail")
			notification := newNotification("welcome", []commontypes.ObjectRef{ref},
				`{"subject":"Hi","body":"There"}`)
			Expect(notification.Status.States).To(BeNil())

			cache.Set("welcome--mail", newTemplateWrapper("{{.subject}}", "{{.body}}"))

			expectGetChannel(fakeClient, channel)
			expectGetTemplate(fakeClient, "welcome--mail", newReadyNotificationTemplate("welcome--mail", testEnvironment))

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(notification.Status.States).NotTo(BeNil())
			Expect(notification.Status.States).To(HaveKey("default/team--mail"))
		})
	})

	// --- buildTemplateName (tested via cache lookup for chat type) ---

	Context("buildTemplateName", func() {
		It("should build correct template name for chat channel", func() {
			chatCh := newReadyChatChannel("team--chat", "default", "https://hook.example.com")
			ref := channelRef("team--chat")
			notification := newNotification("api-approved", []commontypes.ObjectRef{ref}, `{"key":"val"}`)

			cache.Set("api-approved--chat", newTemplateWrapper("subj", "body"))

			expectGetChannel(fakeClient, chatCh)
			expectGetTemplate(fakeClient, "api-approved--chat", newReadyNotificationTemplate("api-approved--chat", testEnvironment))

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(mSender.callCount).To(Equal(1))
		})
	})

	// --- findChannelsForNotification (no explicit channels) ---

	Context("when notification has no explicit channels", func() {
		It("should list channels from namespace", func() {
			notification := newNotification("welcome", nil, `{"subject":"Hi","body":"There"}`)

			emailCh := newReadyEmailChannel([]string{"a@b.c"})
			cache.Set("welcome--mail", newTemplateWrapper("{{.subject}}", "{{.body}}"))

			fakeClient.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.NotificationChannelList"), mock.Anything).
				Run(func(_ context.Context, list k8sclient.ObjectList, _ ...k8sclient.ListOption) {
					chList := list.(*notificationv1.NotificationChannelList)
					chList.Items = []notificationv1.NotificationChannel{*emailCh}
				}).
				Return(nil).Once()

			expectGetChannel(fakeClient, emailCh)
			expectGetTemplate(fakeClient, "welcome--mail", newReadyNotificationTemplate("welcome--mail", testEnvironment))

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(mSender.callCount).To(Equal(1))
		})

		It("should handle List error gracefully with zero channels", func() {
			notification := newNotification("welcome", nil, `{"key":"val"}`)

			fakeClient.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.NotificationChannelList"), mock.Anything).
				Return(fmt.Errorf("list error")).Once()

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			// No channels to process, should be ready
			Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		})
	})

	// --- resolveTemplate: k8s template not ready ---

	Context("when k8s template exists but is not ready", func() {
		It("should block and record error in status", func() {
			channel := newReadyEmailChannel([]string{"a@b.c"})
			ref := channelRef("team--mail")
			notification := newNotification("welcome", []commontypes.ObjectRef{ref}, `{"key":"val"}`)

			cache.Set("welcome--mail", newTemplateWrapper("subj", "body"))

			notReadyTpl := &notificationv1.NotificationTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "welcome--mail", Namespace: testEnvironment},
				Spec: notificationv1.NotificationTemplateSpec{
					Purpose: "welcome", ChannelType: "mail",
					SubjectTemplate: "subj", Template: "body",
					Schema: runtime.RawExtension{Raw: []byte(`{}`)},
				},
				// No ready condition
			}

			expectGetChannel(fakeClient, channel)
			expectGetTemplate(fakeClient, "welcome--mail", notReadyTpl)

			err := handler.CreateOrUpdate(ctx, notification)
			Expect(err).NotTo(HaveOccurred())
			Expect(mSender.callCount).To(Equal(0))

			Expect(notification.Status.States["default/team--mail"].Sent).To(BeFalse())
			Expect(notification.Status.States["default/team--mail"].ErrorMessage).To(ContainSubstring("not ready"))
		})
	})
})
