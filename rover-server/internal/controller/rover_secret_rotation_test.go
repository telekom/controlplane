// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/rover-server/internal/api"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("GetSecretRotationStatus", func() {

	const resourceId = "eni--hyperion--rover-local-sub"

	newRover := func() *roverv1.Rover {
		return mocks.GetRover(GinkgoT(), mocks.RoverFileName)
	}

	newApp := func(gen int64, conditions ...metav1.Condition) *applicationv1.Application {
		return &applicationv1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "rover-local-sub",
				Namespace:  "poc--eni--hyperion",
				Generation: gen,
				UID:        types.UID("app-uid"),
			},
			Spec: applicationv1.ApplicationSpec{
				NeedsClient: true,
			},
			Status: applicationv1.ApplicationStatus{
				ClientId:     "hyperion--rover-local-sub",
				ClientSecret: "current-secret",
				Conditions:   conditions,
			},
		}
	}

	readyCond := func(observedGen int64) metav1.Condition {
		return metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "SubResourceProvisioned",
			Message:            "All sub-resources are up to date",
			ObservedGeneration: observedGen,
		}
	}

	notReadyCond := func(observedGen int64) metav1.Condition {
		return metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "SubResourceProvisioning",
			Message:            "At least one sub-resource has been created or updated",
			ObservedGeneration: observedGen,
		}
	}

	rotationCond := func(status metav1.ConditionStatus, reason string, observedGen int64) metav1.Condition {
		return metav1.Condition{
			Type:               applicationv1.SecretRotationConditionType,
			Status:             status,
			Reason:             reason,
			ObservedGeneration: observedGen,
		}
	}

	newController := func(rover *roverv1.Rover, application *applicationv1.Application) *RoverController {
		roverMock := mocks.NewMockObjectStore[*roverv1.Rover](GinkgoT())
		roverMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(rover, nil).Maybe()

		appMock := mocks.NewMockObjectStore[*applicationv1.Application](GinkgoT())
		appMock.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(application, nil).Maybe()

		testStores := &s.Stores{}
		testStores.RoverStore = roverMock
		testStores.RoverSecretStore = roverMock
		testStores.ApplicationSecretStore = appMock

		return NewRoverController(testStores)
	}

	newCtx := func() context.Context {
		return security.ToContext(context.Background(), &security.BusinessContext{
			Environment: "poc",
			Group:       "eni",
			Team:        "hyperion",
			ClientType:  security.ClientTypeGroup,
			AccessType:  security.AccessTypeReadWrite,
		})
	}

	Context("when application is not ready", func() {
		It("should return processing state when Ready condition is missing", func() {
			// No conditions at all — simulates first reconcile not yet done
			ctrl := newController(newRover(), newApp(2))

			res, err := ctrl.GetSecretRotationStatus(newCtx(), resourceId)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.OverallStatus).To(Equal(api.OverallStatusProcessing))
			Expect(res.ProcessingState).To(Equal(api.ProcessingStateProcessing))
			Expect(res.ClientId).To(BeEmpty())
			Expect(res.ClientSecret).To(BeEmpty())
		})

		It("should return processing state when Ready condition is False", func() {
			ctrl := newController(newRover(), newApp(2, notReadyCond(2)))

			res, err := ctrl.GetSecretRotationStatus(newCtx(), resourceId)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.OverallStatus).To(Equal(api.OverallStatusProcessing))
			Expect(res.ProcessingState).To(Equal(api.ProcessingStateProcessing))
			Expect(res.ClientId).To(BeEmpty())
			Expect(res.ClientSecret).To(BeEmpty())
		})

		It("should return processing state when Ready condition has stale ObservedGeneration", func() {
			// Generation=3 but Ready was stamped at gen 2 — controller hasn't caught up
			ctrl := newController(newRover(), newApp(3, readyCond(2)))

			res, err := ctrl.GetSecretRotationStatus(newCtx(), resourceId)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.OverallStatus).To(Equal(api.OverallStatusProcessing))
			Expect(res.ProcessingState).To(Equal(api.ProcessingStateProcessing))
			Expect(res.ClientId).To(BeEmpty())
			Expect(res.ClientSecret).To(BeEmpty())
		})
	})

	Context("when application is ready and no rotation condition exists", func() {
		It("should return complete status with populated data", func() {
			// Non-graceful rotation completed — Ready=True, no SecretRotation condition
			ctrl := newController(newRover(), newApp(2, readyCond(2)))

			res, err := ctrl.GetSecretRotationStatus(newCtx(), resourceId)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.OverallStatus).To(Equal(api.OverallStatusComplete))
			Expect(res.ProcessingState).To(Equal(api.ProcessingStateDone))
			Expect(res.ClientId).To(Equal("hyperion--rover-local-sub"))
			Expect(res.ClientSecret).To(Equal("current-secret"))
		})
	})

	Context("when application is ready and graceful rotation is in progress", func() {
		It("should return processing state", func() {
			app := newApp(2,
				readyCond(2),
				rotationCond(metav1.ConditionFalse, applicationv1.SecretRotationReasonInProgress, 2),
			)
			app.Status.RotatedClientSecret = "old-secret"
			app.Status.RotatedExpiresAt = &metav1.Time{Time: time.Now().Add(24 * time.Hour)}
			ctrl := newController(newRover(), app)

			res, err := ctrl.GetSecretRotationStatus(newCtx(), resourceId)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.OverallStatus).To(Equal(api.OverallStatusProcessing))
			Expect(res.ProcessingState).To(Equal(api.ProcessingStateProcessing))
			Expect(res.ClientId).To(Equal("hyperion--rover-local-sub"))
			Expect(res.ClientSecret).To(Equal("current-secret"))
			Expect(res.RotatedClientSecret).To(Equal("old-secret"))
			Expect(res.RotatedExpiresAt).ToNot(BeZero())
		})
	})

	Context("when application is ready and graceful rotation completed", func() {
		It("should return complete status", func() {
			app := newApp(2,
				readyCond(2),
				rotationCond(metav1.ConditionTrue, applicationv1.SecretRotationReasonSuccess, 2),
			)
			ctrl := newController(newRover(), app)

			res, err := ctrl.GetSecretRotationStatus(newCtx(), resourceId)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.OverallStatus).To(Equal(api.OverallStatusComplete))
			Expect(res.ProcessingState).To(Equal(api.ProcessingStateDone))
			Expect(res.ClientId).To(Equal("hyperion--rover-local-sub"))
			Expect(res.ClientSecret).To(Equal("current-secret"))
		})
	})

	Context("when application is ready but rotation condition is stale", func() {
		It("should return pending state", func() {
			// Ready is current (gen 3), but SecretRotation condition was stamped at gen 2
			app := newApp(3,
				readyCond(3),
				rotationCond(metav1.ConditionTrue, applicationv1.SecretRotationReasonSuccess, 2),
			)
			ctrl := newController(newRover(), app)

			res, err := ctrl.GetSecretRotationStatus(newCtx(), resourceId)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.OverallStatus).To(Equal(api.OverallStatusPending))
			Expect(res.ProcessingState).To(Equal(api.ProcessingStatePending))
		})
	})

	Context("when rover has no application reference", func() {
		It("should return a BadRequest error", func() {
			rover := newRover()
			rover.Status.Application = nil
			ctrl := newController(rover, nil)

			_, err := ctrl.GetSecretRotationStatus(newCtx(), resourceId)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Application not found"))
		})
	})

	Context("HTTP endpoint integration", func() {
		It("should return 200 with correct response for a ready application", func() {
			req := httptest.NewRequest(http.MethodGet, "/rovers/eni--hyperion--rover-local-sub/secret/status", nil)
			responseGroup, err := ExecuteRequest(req, groupToken)
			ExpectStatusOk(responseGroup, err)
		})
	})
})
