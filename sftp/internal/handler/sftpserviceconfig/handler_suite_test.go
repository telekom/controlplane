// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package sftpserviceconfig

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSFTPServiceConfigHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SFTPServiceConfig Handler Suite")
}
