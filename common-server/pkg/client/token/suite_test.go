// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package accesstoken

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAccessToken(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AccessToken Suite")
}
