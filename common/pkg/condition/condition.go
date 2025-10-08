// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ConditionTypeProcessing = "Processing"
	ConditionTypeReady      = "Ready"
)

var (
	ProcessingCondition = metav1.Condition{
		Type:   ConditionTypeProcessing,
		Status: metav1.ConditionTrue,
	}

	ReadyCondition = metav1.Condition{
		Type:   ConditionTypeReady,
		Status: metav1.ConditionTrue,
	}
)

func NewBlockedCondition(message string) metav1.Condition {
	condition := ProcessingCondition
	condition.Status = metav1.ConditionFalse
	condition.Message = message
	condition.Reason = "Blocked"
	return condition
}

func NewProcessingCondition(reason, message string) metav1.Condition {
	condition := ProcessingCondition
	condition.Message = message
	condition.Reason = reason
	return condition
}

// NewDoneProcessingCondition returns a Condition of type Processing with status False and reason "Done".
// If obj is provided, the ObservedGeneration is set to the generation of the object.
func NewDoneProcessingCondition(message string, obj ...metav1.Object) metav1.Condition {
	condition := ProcessingCondition
	condition.Status = metav1.ConditionFalse
	condition.Message = message
	condition.Reason = "Done"
	if obj != nil {
		condition.ObservedGeneration = obj[0].GetGeneration()
	}
	return condition
}

// NewReadyCondition returns a Condition of type Ready with status True and reason "Ready".
// If obj is provided, the ObservedGeneration is set to the generation of the object.
func NewReadyCondition(reason, message string, obj ...metav1.Object) metav1.Condition {
	condition := ReadyCondition
	condition.Message = message
	condition.Reason = reason
	if obj != nil {
		condition.ObservedGeneration = obj[0].GetGeneration()
	}
	return condition
}

func NewNotReadyCondition(reason, message string) metav1.Condition {
	condition := ReadyCondition
	condition.Status = metav1.ConditionFalse
	condition.Reason = reason
	condition.Message = message
	return condition
}

func SetToUnknown(condition metav1.Condition) metav1.Condition {
	condition.Status = metav1.ConditionUnknown
	condition.Reason = "Unknown"
	condition.Message = ""
	return condition
}
