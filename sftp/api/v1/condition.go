// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	ConditionTypePublicKeysUpdatedInService = "PublicKeysUpdatedInService"
	ConditionReadyReasonSSHPublicKeyProvided   = "SSHPublicKeyProvided"
)

func NewPublicKeysUpdatedInServiceCondition() metav1.Condition {
	return metav1.Condition{
		Type:    ConditionTypePublicKeysUpdatedInService,
		Status:  metav1.ConditionTrue,
		Reason:  "Updated",
		Message: "SFTP public keys have been updated in service",
	}
}

func NewPublicKeysNotUpdatedInServiceCondition(message string) metav1.Condition {
	return metav1.Condition{
		Type:    ConditionTypePublicKeysUpdatedInService,
		Status:  metav1.ConditionFalse,
		Reason:  "UpdateFailed",
		Message: message,
	}
}
