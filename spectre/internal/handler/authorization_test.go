// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

// These tests run inside the handler package (white-box) so they can call
// buildAuthorizationIntent and fingerprint() directly without exporting them.

func baseListener() *spectrev1.Listener {
	return &spectrev1.Listener{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-listener",
			Namespace: "team-ns",
			UID:       "listener-uid-001",
		},
		Spec: spectrev1.ListenerSpec{
			Consumer: ctypes.TypedObjectRef{
				ObjectRef: ctypes.ObjectRef{Name: "consumer-app", Namespace: "team-ns"},
			},
			Provider: ctypes.TypedObjectRef{
				ObjectRef: ctypes.ObjectRef{Name: "provider-app", Namespace: "team-ns"},
			},
			Application: ctypes.ObjectRef{Name: "sa-consumer-app", Namespace: "team-ns"},
			ApiListener: &spectrev1.ApiListener{
				ApiBasePath: "/api/v1/orders",
			},
		},
	}
}

func baseConsumerApp() *applicationv1.Application {
	return &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "consumer-app",
			Namespace: "team-ns",
			UID:       "consumer-uid-001",
		},
		Spec: applicationv1.ApplicationSpec{
			Team:      "team-alpha",
			TeamEmail: "alpha@test.com",
		},
	}
}

func baseProviderApp() *applicationv1.Application {
	return &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "provider-app",
			Namespace: "team-ns",
			UID:       "provider-uid-001",
		},
		Spec: applicationv1.ApplicationSpec{
			Team:      "team-beta",
			TeamEmail: "beta@test.com",
		},
	}
}

func baseSpectreApp() *spectrev1.SpectreApplication {
	return &spectrev1.SpectreApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa-consumer-app",
			Namespace: "team-ns",
		},
		Spec: spectrev1.SpectreApplicationSpec{
			DeliveryType: "server_sent_event",
		},
		Status: spectrev1.SpectreApplicationStatus{
			Id: "consumer-app",
		},
	}
}

var _ = Describe("authorization fingerprint", func() {
	var (
		listener    *spectrev1.Listener
		consumerApp *applicationv1.Application
		providerApp *applicationv1.Application
		spectreApp  *spectrev1.SpectreApplication
	)

	BeforeEach(func() {
		listener = baseListener()
		consumerApp = baseConsumerApp()
		providerApp = baseProviderApp()
		spectreApp = baseSpectreApp()
	})

	baseFingerprint := func() string {
		intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp)
		return intent.fingerprint()
	}

	It("should be stable for identical intents", func() {
		fp1 := baseFingerprint()
		fp2 := baseFingerprint()
		Expect(fp1).To(Equal(fp2))
	})

	It("should be at most 63 characters (K8s label safe)", func() {
		fp := baseFingerprint()
		Expect(len(fp)).To(BeNumerically("<=", 63))
	})

	It("should contain only lowercase hex characters", func() {
		fp := baseFingerprint()
		Expect(fp).To(MatchRegexp("^[a-f0-9]+$"))
	})

	It("should change when provider name changes", func() {
		fp1 := baseFingerprint()
		providerApp.Name = "different-provider"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when provider UID changes", func() {
		fp1 := baseFingerprint()
		providerApp.UID = "different-uid"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when consumer name changes", func() {
		fp1 := baseFingerprint()
		consumerApp.Name = "different-consumer"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when consumer UID changes", func() {
		fp1 := baseFingerprint()
		consumerApp.UID = "different-uid"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when consumer namespace changes", func() {
		fp1 := baseFingerprint()
		consumerApp.Namespace = "other-ns"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when provider namespace changes", func() {
		fp1 := baseFingerprint()
		providerApp.Namespace = "other-ns"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when listener application changes", func() {
		fp1 := baseFingerprint()
		spectreApp.Name = "different-sa"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when API base path changes", func() {
		fp1 := baseFingerprint()
		listener.Spec.ApiListener.ApiBasePath = "/api/v2/orders"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when delivery mode changes", func() {
		fp1 := baseFingerprint()
		spectreApp.Spec.DeliveryType = "callback"
		spectreApp.Spec.Callback = "https://example.com/cb"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when callback target changes", func() {
		spectreApp.Spec.DeliveryType = "callback"
		spectreApp.Spec.Callback = "https://example.com/cb1"
		fp1 := baseFingerprint()
		spectreApp.Spec.Callback = "https://example.com/cb2"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when request filter is added", func() {
		fp1 := baseFingerprint()
		listener.Spec.ApiListener.RequestFilter = &spectrev1.ListenerFilter{
			Trigger: map[string]string{"key": "val"},
		}
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when response filter is added", func() {
		fp1 := baseFingerprint()
		listener.Spec.ApiListener.ResponseFilter = &spectrev1.ListenerFilter{
			Payload: []string{"$.data"},
		}
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	Describe("isStaleChild", func() {
		It("should return true when fingerprint label is missing", func() {
			labels := map[string]string{"cp.ei.telekom.de/owner.uid": "uid-001"}
			Expect(isStaleChild(labels, "abc123")).To(BeTrue())
		})

		It("should return true when fingerprint differs", func() {
			labels := map[string]string{
				AuthorizationFingerprintLabelKey: "old-fingerprint",
			}
			Expect(isStaleChild(labels, "new-fingerprint")).To(BeTrue())
		})

		It("should return false when fingerprint matches", func() {
			fp := "abc123"
			labels := map[string]string{
				AuthorizationFingerprintLabelKey: fp,
			}
			Expect(isStaleChild(labels, fp)).To(BeFalse())
		})
	})
})
