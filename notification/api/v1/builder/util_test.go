// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/notification/api/v1/builder"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("NotificationBuilder Util", func() {
	It("ExtractApplicationInformation with successful target", func() {

		target := types.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ApiSubscription",
				APIVersion: "api.cp.ei.telekom.de/v1",
			},
			ObjectRef: types.ObjectRef{
				Name:      "my-app-for-permissions--foo-bar-permissions-v1",
				Namespace: "env--group--team",
				UID:       "6e8f4e02-c91c-465f-b22d-7f102fca381b",
			},
		}

		kind, application, basepath, group, team := builder.ExtractApplicationInformation(target)
		Expect(kind).To(Equal("ApiSubscription"))
		Expect(application).To(Equal("my-app-for-permissions"))
		Expect(basepath).To(Equal("foo-bar-permissions-v1"))
		Expect(group).To(Equal("group"))
		Expect(team).To(Equal("team"))

	})

	It("ExtractApplicationInformation with empty target", func() {

		target := types.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ApiSubscription",
				APIVersion: "api.cp.ei.telekom.de/v1",
			},
			ObjectRef: types.ObjectRef{
				Name:      "my-app-for-permissions--foo-bar-permissions-v1",
				Namespace: "env--group--team",
				UID:       "6e8f4e02-c91c-465f-b22d-7f102fca381b",
			},
		}

		kind, application, basepath, group, team := builder.ExtractApplicationInformation(target)
		Expect(kind).To(Equal(""))
		Expect(application).To(Equal(""))
		Expect(basepath).To(Equal(""))
		Expect(group).To(Equal(""))
		Expect(team).To(Equal(""))

	})

	It("ExtractApplicationInformation with partial target", func() {

		target := types.TypedObjectRef{}

		kind, application, basepath, group, team := builder.ExtractApplicationInformation(target)
		Expect(kind).To(Equal(""))
		Expect(application).To(Equal(""))
		Expect(basepath).To(Equal(""))
		Expect(group).To(Equal(""))
		Expect(team).To(Equal(""))

	})

})
