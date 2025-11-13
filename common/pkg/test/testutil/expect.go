// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"fmt"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ExpectConditionToMatch(g gomega.Gomega, condition *metav1.Condition, reason string, toBeTrue bool) {
	g.Expect(condition).ToNot(gomega.BeNil(), "Condition is nil")

	expectedStatus := metav1.ConditionFalse
	if toBeTrue {
		expectedStatus = metav1.ConditionTrue
	}
	errMsg := fmt.Sprintf("Expected condition to be %v but got %v. Condition: %v", expectedStatus, condition.Status, condition.String())
	g.Expect(condition.Status).To(gomega.Equal(expectedStatus), errMsg)
	errMsg = fmt.Sprintf("Expected condition reason to be %q but got %q. Condition: %v", reason, condition.Reason, condition.String())
	g.Expect(condition.Reason).To(gomega.Equal(reason), errMsg)
}

func ExpectConditionToBeFalse(g gomega.Gomega, condition *metav1.Condition, reason string) {
	ExpectConditionToMatch(g, condition, reason, false)
}

func ExpectConditionToBeTrue(g gomega.Gomega, condition *metav1.Condition, reason string) {
	ExpectConditionToMatch(g, condition, reason, true)
}
