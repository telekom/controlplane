// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	"github.com/telekom/controlplane/common/pkg/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("Index", func() {

	Context("OwnerIndex", func() {

		It("should enable use to query using the owner-reference", func() {

			ownerObj := test.NewObject("owner", "default")
			Expect(k8sClient.Create(ctx, ownerObj)).To(Succeed())

			ownedObj := test.NewObject("owned", "default")
			Expect(controllerutil.SetControllerReference(ownerObj, ownedObj, k8sClient.Scheme())).To(Succeed())
			Expect(k8sClient.Create(ctx, ownedObj)).To(Succeed())

			objList := test.NewObjectList()
			Eventually(func(g Gomega) {
				err := k8sManager.GetClient().List(ctx, objList, client.MatchingFields{index.ControllerIndexKey: string(ownerObj.GetUID())})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(objList.Items).To(HaveLen(1))

			}, 2*time.Second, 250*time.Millisecond).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, ownedObj)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ownerObj)).To(Succeed())
		})

	})
})
