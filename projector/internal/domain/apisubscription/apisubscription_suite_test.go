// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApiSubscription(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApiSubscription Suite")
}
