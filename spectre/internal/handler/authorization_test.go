// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

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
		Status: applicationv1.ApplicationStatus{
			ClientId: "team-alpha--consumer-app",
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
		Status: applicationv1.ApplicationStatus{
			ClientId: "team-beta--provider-app",
		},
	}
}

func baseSpectreApp() *spectrev1.SpectreApplication {
	return &spectrev1.SpectreApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa-consumer-app",
			Namespace: "team-ns",
			UID:       "spectre-app-uid-001",
		},
		Spec: spectrev1.SpectreApplicationSpec{
			DeliveryType: "server_sent_event",
		},
		Status: spectrev1.SpectreApplicationStatus{
			Id: "consumer-app",
		},
	}
}

func basePlacement() PlacementIntent {
	return PlacementIntent{
		ApiExposureName:             "api-exposure-orders",
		ApiExposureNamespace:        "provider-ns",
		CaptureRouteName:            "route-orders",
		CaptureRouteNamespace:       "zone-ns",
		CaptureZoneName:             "zone-aws",
		CaptureZoneNamespace:        "zones",
		CaptureEventStoreName:       "store-aws",
		CaptureEventStoreNamespace:  "zones",
		CallbackOriginZoneName:      "zone-aws",
		CallbackOriginZoneNamespace: "zones",
		DeliveryZoneName:            "zone-aws",
		DeliveryZoneNamespace:       "zones",
		DeliveryEventStoreName:      "store-aws",
		DeliveryEventStoreNamespace: "zones",
		CallbackBaseURL:             "https://spectre-bridge.aws.svc:8443",
	}
}

var _ = Describe("authorization fingerprint", func() {
	var (
		listener    *spectrev1.Listener
		consumerApp *applicationv1.Application
		providerApp *applicationv1.Application
		spectreApp  *spectrev1.SpectreApplication
		observerApp *applicationv1.Application
		placement   PlacementIntent
	)

	BeforeEach(func() {
		listener = baseListener()
		consumerApp = baseConsumerApp()
		providerApp = baseProviderApp()
		spectreApp = baseSpectreApp()
		// Self-only staging: observer == consumer
		observerApp = baseConsumerApp()
		placement = basePlacement()
	})

	baseFingerprint := func() string {
		intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
		return intent.fingerprint()
	}

	It("should populate API group and kind from compile-time constants", func() {
		intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
		Expect(intent.ConsumerGroup).To(Equal("application.cp.ei.telekom.de"))
		Expect(intent.ConsumerKind).To(Equal("Application"))
		Expect(intent.ProviderGroup).To(Equal("application.cp.ei.telekom.de"))
		Expect(intent.ProviderKind).To(Equal("Application"))
		Expect(intent.SpectreAppGroup).To(Equal("spectre.cp.ei.telekom.de"))
		Expect(intent.SpectreAppKind).To(Equal("SpectreApplication"))
		Expect(intent.ObserverGroup).To(Equal("application.cp.ei.telekom.de"))
		Expect(intent.ObserverKind).To(Equal("Application"))
	})

	It("should include group/kind in fingerprint so different CRD types with same name/ns/UID differ", func() {
		intent1 := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
		fp1 := intent1.fingerprint()

		// Manually override group to simulate a hypothetical different CRD type
		intent2 := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
		intent2.ConsumerGroup = "different.group.io"
		fp2 := intent2.fingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

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

	// --- Identity invalidation ---

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

	// --- Filter invalidation (existing tests) ---

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

	It("should produce same fingerprint for nil filters as before implementation", func() {
		// No filters set — RequestFilterJSON and ResponseFilterJSON are "",
		// which maps to "false" in the hash via filterFingerprintValue,
		// matching the old bool-false behavior.
		intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
		Expect(intent.RequestFilterJSON).To(Equal(""))
		Expect(intent.ResponseFilterJSON).To(Equal(""))
		Expect(filterFingerprintValue("")).To(Equal("false"))
	})

	It("should change when request filter content changes", func() {
		listener.Spec.ApiListener.RequestFilter = &spectrev1.ListenerFilter{
			Trigger: map[string]string{"a": "b"},
		}
		fp1 := baseFingerprint()
		listener.Spec.ApiListener.RequestFilter = &spectrev1.ListenerFilter{
			Trigger: map[string]string{"a": "c"},
		}
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when response filter payload changes", func() {
		listener.Spec.ApiListener.ResponseFilter = &spectrev1.ListenerFilter{
			Payload: []string{"x"},
		}
		fp1 := baseFingerprint()
		listener.Spec.ApiListener.ResponseFilter = &spectrev1.ListenerFilter{
			Payload: []string{"y"},
		}
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should not depend on trigger map key ordering", func() {
		listener.Spec.ApiListener.RequestFilter = &spectrev1.ListenerFilter{
			Trigger: map[string]string{"a": "1", "b": "2"},
		}
		fp1 := baseFingerprint()
		listener.Spec.ApiListener.RequestFilter = &spectrev1.ListenerFilter{
			Trigger: map[string]string{"b": "2", "a": "1"},
		}
		fp2 := baseFingerprint()
		Expect(fp1).To(Equal(fp2))
	})

	It("should expose filter JSON in approval properties", func() {
		listener.Spec.ApiListener.RequestFilter = &spectrev1.ListenerFilter{
			Trigger: map[string]string{"key": "val"},
		}
		intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
		props := intent.approvalProperties()
		Expect(props["requestFilter"]).To(BeAssignableToTypeOf(""))
		Expect(props["requestFilter"]).To(ContainSubstring("key"))
	})

	It("should expose false for approval properties when filter is nil", func() {
		intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
		props := intent.approvalProperties()
		Expect(props["requestFilter"]).To(Equal(false))
		Expect(props["responseFilter"]).To(Equal(false))
	})

	// --- v2 policy version ---

	It("should change when PolicyVersion changes", func() {
		fp1 := baseFingerprint()
		intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
		intent.PolicyVersion = "v3"
		fp2 := intent.fingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should always set PolicyVersion to v2", func() {
		intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
		Expect(intent.PolicyVersion).To(Equal("v2"))
	})

	// --- Observer (A) invalidation ---

	It("should change when observer name changes", func() {
		fp1 := baseFingerprint()
		observerApp.Name = "different-observer"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when observer namespace changes", func() {
		fp1 := baseFingerprint()
		observerApp.Namespace = "other-ns"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when observer UID changes", func() {
		fp1 := baseFingerprint()
		observerApp.UID = types.UID("different-observer-uid")
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	// --- SpectreApplication UID ---

	It("should change when SpectreApplication UID changes", func() {
		fp1 := baseFingerprint()
		spectreApp.UID = "different-spectre-uid"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	// --- Team and ClientId identity ---

	It("should change when consumer team changes", func() {
		fp1 := baseFingerprint()
		consumerApp.Spec.Team = "team-gamma"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when consumer clientId changes", func() {
		fp1 := baseFingerprint()
		consumerApp.Status.ClientId = "team-alpha--different-client"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when provider team changes", func() {
		fp1 := baseFingerprint()
		providerApp.Spec.Team = "team-delta"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when provider clientId changes", func() {
		fp1 := baseFingerprint()
		providerApp.Status.ClientId = "team-beta--different-client"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when observer team changes", func() {
		fp1 := baseFingerprint()
		observerApp.Spec.Team = "team-epsilon"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when observer clientId changes", func() {
		fp1 := baseFingerprint()
		observerApp.Status.ClientId = "team-alpha--different-observer"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	// --- Placement invalidation ---

	It("should change when apiExposure name changes", func() {
		fp1 := baseFingerprint()
		placement.ApiExposureName = "different-exposure"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when captureRoute name changes", func() {
		fp1 := baseFingerprint()
		placement.CaptureRouteName = "different-route"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when captureZone changes", func() {
		fp1 := baseFingerprint()
		placement.CaptureZoneName = "zone-cetus"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when captureEventStore changes", func() {
		fp1 := baseFingerprint()
		placement.CaptureEventStoreName = "different-store"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when callbackOriginZone changes", func() {
		fp1 := baseFingerprint()
		placement.CallbackOriginZoneName = "zone-cetus"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when deliveryZone changes", func() {
		fp1 := baseFingerprint()
		placement.DeliveryZoneName = "zone-cetus"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when deliveryEventStore changes", func() {
		fp1 := baseFingerprint()
		placement.DeliveryEventStoreName = "different-store"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	It("should change when callbackBaseURL changes", func() {
		fp1 := baseFingerprint()
		placement.CallbackBaseURL = "https://different.svc:8443"
		fp2 := baseFingerprint()
		Expect(fp1).ToNot(Equal(fp2))
	})

	// --- Stability: irrelevant status/email changes must NOT invalidate ---

	It("should not change when consumerApp.Spec.TeamEmail changes", func() {
		fp1 := baseFingerprint()
		consumerApp.Spec.TeamEmail = "newemail@test.com"
		fp2 := baseFingerprint()
		Expect(fp1).To(Equal(fp2))
	})

	It("should not change when providerApp.Spec.TeamEmail changes", func() {
		fp1 := baseFingerprint()
		providerApp.Spec.TeamEmail = "newemail@test.com"
		fp2 := baseFingerprint()
		Expect(fp1).To(Equal(fp2))
	})

	It("should not change when spectreApp.Status.Id changes", func() {
		fp1 := baseFingerprint()
		spectreApp.Status.Id = "different-app-id"
		fp2 := baseFingerprint()
		Expect(fp1).To(Equal(fp2))
	})

	// --- gateRequestHash ---

	Describe("gateRequestHash", func() {
		It("should produce different hashes for different keys", func() {
			intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
			h1, err := intent.gateRequestHash("listen-provider", "team-alpha", "team-beta")
			Expect(err).ToNot(HaveOccurred())
			h2, err := intent.gateRequestHash("listen-consumer", "team-alpha", "team-beta")
			Expect(err).ToNot(HaveOccurred())
			Expect(h1).ToNot(Equal(h2))
		})

		It("should produce different hashes for different requester teams", func() {
			intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
			h1, err := intent.gateRequestHash("listen-provider", "team-alpha", "team-beta")
			Expect(err).ToNot(HaveOccurred())
			h2, err := intent.gateRequestHash("listen-provider", "team-gamma", "team-beta")
			Expect(err).ToNot(HaveOccurred())
			Expect(h1).ToNot(Equal(h2))
		})

		It("should produce different hashes for different decider teams", func() {
			intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
			h1, err := intent.gateRequestHash("listen-provider", "team-alpha", "team-beta")
			Expect(err).ToNot(HaveOccurred())
			h2, err := intent.gateRequestHash("listen-provider", "team-alpha", "team-delta")
			Expect(err).ToNot(HaveOccurred())
			Expect(h1).ToNot(Equal(h2))
		})

		It("should be stable for identical inputs", func() {
			intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
			h1, err := intent.gateRequestHash("listen-provider", "team-alpha", "team-beta")
			Expect(err).ToNot(HaveOccurred())
			h2, err := intent.gateRequestHash("listen-provider", "team-alpha", "team-beta")
			Expect(err).ToNot(HaveOccurred())
			Expect(h1).To(Equal(h2))
		})

		It("should return error for empty key", func() {
			intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
			_, err := intent.gateRequestHash("", "team-alpha", "team-beta")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("key must not be empty"))
		})

		It("should change when common intent changes", func() {
			intent1 := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
			h1, err := intent1.gateRequestHash("listen-provider", "team-alpha", "team-beta")
			Expect(err).ToNot(HaveOccurred())

			providerApp.Name = "different-provider"
			intent2 := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
			h2, err := intent2.gateRequestHash("listen-provider", "team-alpha", "team-beta")
			Expect(err).ToNot(HaveOccurred())

			Expect(h1).ToNot(Equal(h2))
		})
	})

	// --- approvalProperties v2 fields ---

	Describe("approvalProperties v2", func() {
		It("should include observer field", func() {
			intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
			props := intent.approvalProperties()
			Expect(props["observer"]).To(Equal("team-ns/consumer-app"))
		})

		It("should include policyVersion", func() {
			intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
			props := intent.approvalProperties()
			Expect(props["policyVersion"]).To(Equal("v2"))
		})

		It("should include team fields", func() {
			intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
			props := intent.approvalProperties()
			Expect(props["consumerTeam"]).To(Equal("team-alpha"))
			Expect(props["providerTeam"]).To(Equal("team-beta"))
			Expect(props["observerTeam"]).To(Equal("team-alpha"))
		})

		It("should include placement fields when populated", func() {
			intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
			props := intent.approvalProperties()
			Expect(props["apiExposure"]).To(Equal("provider-ns/api-exposure-orders"))
			Expect(props["captureRoute"]).To(Equal("zone-ns/route-orders"))
			Expect(props["captureZone"]).To(Equal("zones/zone-aws"))
			Expect(props["deliveryZone"]).To(Equal("zones/zone-aws"))
			Expect(props["callbackBaseURL"]).To(Equal("https://spectre-bridge.aws.svc:8443"))
		})

		It("should omit placement fields when empty", func() {
			placement = PlacementIntent{}
			intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
			props := intent.approvalProperties()
			Expect(props).ToNot(HaveKey("apiExposure"))
			Expect(props).ToNot(HaveKey("captureRoute"))
			Expect(props).ToNot(HaveKey("captureZone"))
			Expect(props).ToNot(HaveKey("deliveryZone"))
			Expect(props).ToNot(HaveKey("callbackBaseURL"))
		})

		It("should preserve existing approval property keys", func() {
			intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
			props := intent.approvalProperties()
			// These fields are used by notification templates
			Expect(props["consumer"]).To(Equal("team-ns/consumer-app"))
			Expect(props["provider"]).To(Equal("team-ns/provider-app"))
			Expect(props["listenerApplication"]).To(Equal("team-ns/sa-consumer-app"))
			Expect(props["apiBasePath"]).To(Equal("/api/v1/orders"))
			Expect(props["action"]).To(Equal("listen-provider"))
		})
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
