// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"errors"
	"testing"

	k8stypes "k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
)

func TestUtil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Agentic Handler Util Suite")
}

// unwrapAll follows the pkg/errors Cause chain to the root error.
func unwrapAll(err error) error {
	for {
		cause, ok := err.(interface{ Cause() error })
		if !ok {
			return err
		}
		err = cause.Cause()
	}
}

// isBlockedError checks if the error implements the BlockedError interface.
func isBlockedError(err error) bool {
	var be ctrlerrors.BlockedError
	ok := errors.As(err, &be)
	return ok && be.IsBlocked()
}

// setReady stamps the Ready=True condition on the given object.
func setReady(conditions *[]metav1.Condition) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
}

func makeReadyZone(name string) *adminv1.Zone {
	z := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Status:     adminv1.ZoneStatus{Namespace: "zone-ns"},
	}
	setReady(&z.Status.Conditions)
	return z
}

func makeReadyApplication(name, clientId string) *applicationapi.Application {
	a := &applicationapi.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Status:     applicationapi.ApplicationStatus{ClientId: clientId},
	}
	setReady(&a.Status.Conditions)
	return a
}

func makeReadyMcpServer(basePath string) agenticv1.McpServer {
	s := agenticv1.McpServer{
		ObjectMeta: metav1.ObjectMeta{Name: "mcp-server", Namespace: "default"},
		Spec:       agenticv1.McpServerSpec{BasePath: basePath, Version: "1.0.0", Name: "Test"},
		Status:     agenticv1.McpServerStatus{Active: true},
	}
	setReady(&s.Status.Conditions)
	return s
}

//nolint:unparam // test helper designed for reuse with different basePaths
func makeActiveMcpExposure(basePath, zoneName, uid string) agenticv1.McpExposure {
	e := agenticv1.McpExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "exposure-" + zoneName,
			Namespace: "default",
			UID:       k8stypes.UID(uid),
		},
		Spec: agenticv1.McpExposureSpec{
			BasePath: basePath,
			Zone:     ctypes.ObjectRef{Name: zoneName, Namespace: "default"},
			Provider: ctypes.ObjectRef{Name: "app", Namespace: "default"},
		},
		Status: agenticv1.McpExposureStatus{Active: true},
	}
	return e
}
