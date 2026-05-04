// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mocks

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/mock"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/store"
	csmocks "github.com/telekom/controlplane/common-server/test/mocks"
)

func NewApprovalStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*approvalv1.Approval] {
	mockStore := csmocks.NewMockObjectStore[*approvalv1.Approval](testing)
	ConfigureApprovalStoreMock(testing, mockStore)
	return mockStore
}

func ConfigureApprovalStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *csmocks.MockObjectStore[*approvalv1.Approval]) {
	approval := GetApproval(testing, approvalFileName)

	mockedStore.EXPECT().Get(
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.Anything,
	).Return(approval, nil).Maybe()
}
