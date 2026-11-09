// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package applicationinfo

import (
	"encoding/json"

	"github.com/stretchr/testify/mock"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subscription endpoint metadata", func() {
	DescribeTable("maps available file exposure endpoints",
		func(internalURL, externalURL string) {
			exposure := &filev1.FileExposure{
				Spec: filev1.FileExposureSpec{FileType: "reports", SFTP: &filev1.FileSFTP{}},
				Status: filev1.FileExposureStatus{
					ServiceURL: internalURL, ServiceExternalURL: externalURL,
				},
			}
			mapped := mapFileExposureInfo(exposure)
			Expect(mapped.ServiceUrl).To(Equal(internalURL))
			Expect(mapped.ServiceExternalUrl).To(Equal(externalURL))
			raw, err := json.Marshal(mapped)
			Expect(err).NotTo(HaveOccurred())
			var fields map[string]json.RawMessage
			Expect(json.Unmarshal(raw, &fields)).To(Succeed())
			if internalURL == "" {
				Expect(fields).NotTo(HaveKey("serviceUrl"))
			} else {
				Expect(string(fields["serviceUrl"])).To(MatchJSON(`"` + internalURL + `"`))
			}
			if externalURL == "" {
				Expect(fields).NotTo(HaveKey("serviceExternalUrl"))
			} else {
				Expect(string(fields["serviceExternalUrl"])).To(MatchJSON(`"` + externalURL + `"`))
			}
		},
		Entry("both endpoints", "sftp.internal:22", "sftp.example.com:2222"),
		Entry("internal endpoint only", "sftp.internal:22", ""),
		Entry("external endpoint only", "", "sftp.example.com:2222"),
		Entry("unavailable endpoints", "", ""),
	)

	It("includes pending API and file endpoints without hiding approval errors", func() {
		apiSub := &apiv1.ApiSubscription{
			Spec:   apiv1.ApiSubscriptionSpec{ApiBasePath: "/checkout/v1"},
			Status: apiv1.ApiSubscriptionStatus{GatewayUrl: "https://gateway.example.com/checkout/v1"},
		}
		apiSub.SetCondition(condition.NewNotReadyCondition("ApprovalPending", "Waiting for approval"))
		fileSub := &filev1.FileSubscription{
			Spec:   filev1.FileSubscriptionSpec{FileType: "reports", SFTP: &filev1.FileSFTP{}},
			Status: filev1.FileSubscriptionStatus{ServiceURL: "sftp.internal:22", ServiceExternalURL: "sftp.example.com:2222"},
		}
		fileSub.SetCondition(condition.NewNotReadyCondition("ApprovalPending", "Waiting for approval"))
		apiStore := mocks.NewMockObjectStore[*apiv1.ApiSubscription](GinkgoT())
		apiStore.EXPECT().Get(mock.Anything, "test", "api").Return(apiSub, nil).Once()
		fileStore := mocks.NewMockObjectStore[*filev1.FileSubscription](GinkgoT())
		fileStore.EXPECT().Get(mock.Anything, "test", "file").Return(fileSub, nil).Once()
		resource := &roverv1.Rover{Status: roverv1.RoverStatus{
			ApiSubscriptions:  []types.ObjectRef{{Namespace: "test", Name: "api"}},
			FileSubscriptions: []types.ObjectRef{{Namespace: "test", Name: "file"}},
		}}
		info := &api.ApplicationInfo{}
		Expect(FillSubscriptionInfo(ctx, resource, info, &store.Stores{
			APISubscriptionStore: apiStore, FileSubscriptionStore: fileStore,
		})).To(Succeed())
		Expect(info.Variables).To(Equal([]api.Data{{
			Name: "tardis.stargate.url.api.checkout.v1", Value: "https://gateway.example.com/checkout/v1",
		}}))
		Expect(info.Errors).To(HaveLen(2))
		Expect(info.Errors[0].Cause).To(Equal("Waiting for approval"))
		Expect(info.Errors[1].Cause).To(Equal("Waiting for approval"))
		Expect(info.Status).NotTo(Equal(api.OverallStatusComplete))
		raw, err := json.Marshal(info.Subscriptions)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).To(MatchJSON(`[
			{"type":"api","basePath":"/checkout/v1"},
			{"type":"file","fileType":"reports","publicKeys":[],"serviceUrl":"sftp.internal:22","serviceExternalUrl":"sftp.example.com:2222"}
		]`))
	})

	DescribeTable("omits unavailable file endpoints and preserves granted mapping",
		func(internalURL, externalURL string, ready bool, expected string) {
			sub := &filev1.FileSubscription{Spec: filev1.FileSubscriptionSpec{
				FileType: "reports", SFTP: &filev1.FileSFTP{},
			}, Status: filev1.FileSubscriptionStatus{ServiceURL: internalURL, ServiceExternalURL: externalURL}}
			if ready {
				sub.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Ready"))
			} else {
				sub.SetCondition(condition.NewNotReadyCondition("ApprovalPending", "Waiting for approval"))
			}
			mapped := mapFileSubscriptionInfo(sub)
			raw, err := json.Marshal(mapped)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(raw)).To(MatchJSON(expected))
		},
		Entry("pending without endpoints", "", "", false, `{"type":"file","fileType":"reports","publicKeys":[]}`),
		Entry("internal endpoint only", "sftp.internal:22", "", false, `{"type":"file","fileType":"reports","publicKeys":[],"serviceUrl":"sftp.internal:22"}`),
		Entry("external endpoint only", "", "sftp.example.com:22", false, `{"type":"file","fileType":"reports","publicKeys":[],"serviceExternalUrl":"sftp.example.com:22"}`),
		Entry("granted", "sftp.internal:22", "sftp.example.com:22", true, `{"type":"file","fileType":"reports","publicKeys":[],"serviceUrl":"sftp.internal:22","serviceExternalUrl":"sftp.example.com:22"}`),
	)
})
