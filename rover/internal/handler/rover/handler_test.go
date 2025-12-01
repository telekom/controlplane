// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rover_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/test/testutil"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"github.com/telekom/controlplane/rover/internal/handler/rover"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	secretsapifake "github.com/telekom/controlplane/secret-manager/api/fake"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

			testutil.ExpectConditionToBeFalse(NewGomegaWithT(GinkgoT()), meta.FindStatusCondition(roverObj.GetConditions(), condition.ConditionTypeReady), "DeletionFailed")
		})
	})
})
