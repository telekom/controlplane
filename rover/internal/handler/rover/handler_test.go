// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rover_test

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/telekom/controlplane/common-server/pkg/client"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/test/testutil"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"github.com/telekom/controlplane/rover/internal/handler/rover"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	secretsapifake "github.com/telekom/controlplane/secret-manager/api/fake"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func createRoverObject() *roverv1.Rover {
	return &roverv1.Rover{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rover",
			Namespace: teamNamespace,
		},
		Spec: roverv1.RoverSpec{},
	}
}

var _ = Describe("Rover Handler", func() {
	var ctx context.Context
	var secretManagerMock *secretsapifake.MockSecretManager
	var roverHandler *rover.RoverHandler

	BeforeEach(func() {
		ctx = context.Background()
		ctx = contextutil.WithEnv(ctx, testEnvironment)

		By("Setup Secret Manager Mock")
		secretManagerMock = secretsapifake.NewMockSecretManager(GinkgoT())
		secretsapi.API = func() secretsapi.SecretManager {
			return secretManagerMock
		}

		By("Setup Rover Handler")
		roverHandler = &rover.RoverHandler{}
	})

	Context("Secret Manager Integration", func() {
		It("should successfully delete the resource", func() {
			By("Create Rover Object")
			roverObj := createRoverObject()

			By("Setting up the mock expectations")
			secretManagerMock.EXPECT().DeleteApplication(ctx, testEnvironment, teamId, roverObj.GetName()).Return(nil)

			By("Call Delete on Rover Handler")
			err := roverHandler.Delete(ctx, roverObj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle errors from Secret Manager", func() {
			By("Create Rover Object")
			roverObj := createRoverObject()

			By("Setting up the mock expectations")
			httpErr := client.BlockedErrorf("bad request error (400): some error")
			secretManagerMock.EXPECT().DeleteApplication(ctx, testEnvironment, teamId, roverObj.GetName()).
				Return(httpErr)

			By("Call Delete on Rover Handler")
			err := roverHandler.Delete(ctx, roverObj)
			Expect(err).To(HaveOccurred())
			rootCause := errors.Cause(err)
			Expect(rootCause).To(Equal(httpErr))

			testutil.ExpectConditionToBeFalse(NewGomegaWithT(GinkgoT()), meta.FindStatusCondition(roverObj.GetConditions(), condition.ConditionTypeReady), condition.ReasonError)
		})
	})
})

var _ = Describe("Rover Handler with a blocked listener", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		roverObj   *roverv1.Rover
	)

	BeforeEach(func() {
		config.SetFeatureEnabled(config.FeatureSpectre, true)
		DeferCleanup(config.SetFeatureEnabled, config.FeatureSpectre, false)

		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = contextutil.WithEnv(context.Background(), testEnvironment)
		ctx = cclient.WithClient(ctx, fakeClient)

		roverObj = createRoverObject()
		roverObj.UID = "rover-uid"
		roverObj.Spec.Zone = "zone1"
		roverObj.Spec.Listeners = []roverv1.RoverListener{
			{Consumer: roverObj.Name, Provider: "does-not-exist", ApiBasePath: "/echo/v1"},
		}

		// Everything up to the listener entry: the Application and the
		// SpectreApplication are created, and the provider does not resolve.
		fakeClient.EXPECT().AddKnownTypeToState(mock.Anything).Maybe()
		fakeClient.EXPECT().Get(ctx, mock.Anything, mock.AnythingOfType("*v1.Team")).Return(nil).Once()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Application"), mock.Anything).
			Return(controllerutil.OperationResultCreated, nil).Once()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.SpectreApplication"), mock.Anything).
			Return(controllerutil.OperationResultCreated, nil).Once()
		fakeClient.EXPECT().List(ctx, mock.AnythingOfType("*v1.ApplicationList"), mock.Anything).Return(nil).Once()
	})

	It("should run permissions and cleanup before returning the Blocked error", func() {
		fakeClient.EXPECT().
			Get(ctx, mock.Anything, mock.AnythingOfType("*v1.Listener")).
			Return(apierrors.NewNotFound(spectrev1.GroupVersion.WithResource("listeners").GroupResource(), "listener")).Once()
		fakeClient.EXPECT().CleanupAll(ctx, mock.Anything).Return(0, nil).Once()

		err := (&rover.RoverHandler{}).CreateOrUpdate(ctx, roverObj)

		Expect(err).To(MatchError(ContainSubstring(`application "does-not-exist" not found`)))
		be, ok := stderrors.AsType[ctrlerrors.BlockedError](err)
		Expect(ok && be.IsBlocked()).To(BeTrue(), "expected a Blocked error, got %v", err)
		// handlePermissions is the only step that sets PermissionSets.
		Expect(roverObj.Status.PermissionSets).ToNot(BeNil())
	})

	It("should skip permissions and cleanup on a non-blocked listener error", func() {
		fakeClient.EXPECT().
			Get(ctx, mock.Anything, mock.AnythingOfType("*v1.Listener")).
			Return(fmt.Errorf("api server error")).Once()

		// The strict mock fails the test if CleanupAll is called.
		err := (&rover.RoverHandler{}).CreateOrUpdate(ctx, roverObj)

		Expect(err).To(MatchError(ContainSubstring("api server error")))
		_, ok := stderrors.AsType[ctrlerrors.BlockedError](err)
		Expect(ok).To(BeFalse(), "expected a non-blocked error, got %v", err)
		Expect(roverObj.Status.PermissionSets).To(BeNil())
	})
})
