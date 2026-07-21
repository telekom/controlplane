// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package sftpserviceconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/condition"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	"github.com/telekom/controlplane/sftp/internal/service"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SFTPServiceConfigHandler", func() {
	const (
		testName      = "test-sftpServiceConfig"
		testNamespace = "test"
	)

	var (
		ctx context.Context
		obj *sftpv1.SFTPServiceConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		obj = &sftpv1.SFTPServiceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:       testName,
				Namespace:  testNamespace,
				Generation: 2,
			},
		}
	})

	It("requires a client manager", func() {
		handler, err := New(nil)

		Expect(err).To(MatchError("client manager is required"))
		Expect(handler).To(BeNil())
	})

	It("recreates the service when the cache is empty", func() {
		manager := &recordingClientManager{serviceCached: false}
		handler := newTestHandler(manager)

		Expect(handler.CreateOrUpdate(ctx, obj)).To(Succeed())

		Expect(manager.cacheChecks).To(Equal([]client.ObjectKey{{Namespace: testNamespace, Name: testName}}))
		Expect(manager.createOrUpdateCalls).To(Equal(1))
		Expect(meta.IsStatusConditionTrue(obj.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		Expect(meta.IsStatusConditionFalse(obj.Status.Conditions, condition.ConditionTypeProcessing)).To(BeTrue())
	})

	It("recreates the service when the Ready condition observed generation is stale", func() {
		manager := &recordingClientManager{serviceCached: true}
		handler := newTestHandler(manager)
		ready := condition.NewReadyCondition("SFTPServiceConfigProvided", "SFTPServiceConfig has been provided")
		ready.ObservedGeneration = obj.Generation - 1
		obj.SetCondition(ready)

		Expect(handler.CreateOrUpdate(ctx, obj)).To(Succeed())

		Expect(manager.cacheChecks).To(Equal([]client.ObjectKey{{Namespace: testNamespace, Name: testName}}))
		Expect(manager.createOrUpdateCalls).To(Equal(1))
	})

	It("skips reconciliation when the Ready condition observed generation is current and the service is cached", func() {
		manager := &recordingClientManager{serviceCached: true}
		handler := newTestHandler(manager)
		ready := condition.NewReadyCondition("SFTPServiceConfigProvided", "SFTPServiceConfig has been provided")
		ready.ObservedGeneration = obj.Generation
		obj.SetCondition(ready)

		Expect(handler.CreateOrUpdate(ctx, obj)).To(Succeed())

		Expect(manager.cacheChecks).To(Equal([]client.ObjectKey{{Namespace: testNamespace, Name: testName}}))
		Expect(manager.createOrUpdateCalls).To(Equal(0))
	})
})

func newTestHandler(manager service.ClientManager) *SFTPServiceConfigHandler {
	handler, err := New(manager)
	Expect(err).NotTo(HaveOccurred())
	return handler
}

type recordingClientManager struct {
	serviceCached       bool
	cacheChecks         []client.ObjectKey
	createOrUpdateCalls int
}

func (m *recordingClientManager) ServiceFor(context.Context, client.ObjectKey) (service.Service, error) {
	return service.NopService{}, nil
}

func (m *recordingClientManager) ExistClient(key client.ObjectKey) bool {
	m.cacheChecks = append(m.cacheChecks, key)
	return m.serviceCached
}

func (m *recordingClientManager) CreateOrUpdate(context.Context, *sftpv1.SFTPServiceConfig) error {
	m.createOrUpdateCalls++
	return nil
}

func (m *recordingClientManager) Delete(*sftpv1.SFTPServiceConfig) {}
