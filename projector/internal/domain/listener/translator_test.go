// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package listener

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	projectorruntime "github.com/telekom/controlplane/projector/internal/runtime"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Listener translator", func() {
	const namespace = "prod--observer"
	var (
		obj        *spectrev1.Listener
		app        *spectrev1.SpectreApplication
		translator *Translator
	)
	BeforeEach(func() {
		obj = &spectrev1.Listener{ObjectMeta: metav1.ObjectMeta{Name: "listener", Namespace: namespace}, Spec: spectrev1.ListenerSpec{
			Application: ctypes.ObjectRef{Name: "spectre-app"},
			Consumer:    ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Name: "consumer", Namespace: "prod--consumer"}},
			Provider:    ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Name: "provider", Namespace: "prod--provider"}},
			ApiListener: &spectrev1.ApiListener{ApiBasePath: "/api", RequestFilter: &spectrev1.ListenerFilter{
				Trigger: map[string]string{"method": "GET"}, Payload: []string{"$.id"},
			}},
		}}
		app = &spectrev1.SpectreApplication{ObjectMeta: metav1.ObjectMeta{Name: "spectre-app", Namespace: namespace},
			Spec: spectrev1.SpectreApplicationSpec{Application: ctypes.TypedObjectRef{
				ObjectRef: ctypes.ObjectRef{Name: "observer", Namespace: namespace},
			}}}
		scheme := runtime.NewScheme()
		Expect(spectrev1.AddToScheme(scheme)).To(Succeed())
		translator = &Translator{Reader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(app).Build()}
	})

	It("maps the observer, status, and filters", func() {
		obj.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", Message: "capturing"}}
		data, err := translator.Translate(context.Background(), obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(data.Observer).To(Equal(Key{Namespace: namespace, Name: "observer"}))
		Expect(data.Consumer.Name).To(Equal("consumer"))
		Expect(data.Provider.Name).To(Equal("provider"))
		Expect(data.StatusPhase).To(Equal("READY"))
		Expect(data.StatusMessage).To(Equal("capturing"))
		Expect(data.RequestFilter.Trigger).To(HaveKeyWithValue("method", "GET"))
		Expect(data.RequestFilter.Payload).To(Equal([]string{"$.id"}))
		Expect(data.ResponseFilter).To(BeNil())
	})

	It("retries when SpectreApplication is absent", func() {
		var removed Key
		translator.OnMissingApplication = func(_ context.Context, key Key) error { removed = key; return nil }
		obj.Spec.Application.Name = "missing"
		_, err := translator.Translate(context.Background(), obj)
		Expect(projectorruntime.IsDependencyMissing(err)).To(BeTrue())
		Expect(removed).To(Equal(Key{Namespace: namespace, Name: "listener"}))
	})

	It("skips unsupported event-only listeners", func() {
		obj.Spec.ApiListener = nil
		obj.Spec.EventListener = &spectrev1.EventListener{EventType: "event"}
		skip, _ := translator.ShouldSkip(obj)
		Expect(skip).To(BeTrue())
	})

	It("skips listeners with both API and unsupported event capture", func() {
		obj.Spec.EventListener = &spectrev1.EventListener{EventType: "event"}
		skip, _ := translator.ShouldSkip(obj)
		Expect(skip).To(BeTrue())
	})

	It("skips incomplete cross-namespace references", func() {
		obj.Spec.Consumer.Namespace = ""
		skip, _ := translator.ShouldSkip(obj)
		Expect(skip).To(BeTrue())
	})

	It("derives deletion identity without the last object", func() {
		key, err := translator.KeyFromDelete(types.NamespacedName{Namespace: namespace, Name: "listener"}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(key).To(Equal(Key{Namespace: namespace, Name: "listener"}))
	})
})
