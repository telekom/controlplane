// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ predicate.Predicate = CustomPredicate{}

// CustomPredicate is a predicate that filters events based on the Ready condition and generation.
type CustomPredicate struct{}

func convertObject(o client.Object) types.Object {
	obj, ok := o.(types.Object)
	if !ok {
		panic("object does not implement types.Object")
	}
	return obj
}

// Create implements predicate.TypedPredicate.
func (c CustomPredicate) Create(e event.TypedCreateEvent[client.Object]) bool {
	if e.IsInInitialList {
		return true
	}
	obj := convertObject(e.Object)
	readyCondition := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
	if readyCondition == nil || readyCondition.Status != metav1.ConditionTrue {
		return true
	}
	if readyCondition.ObservedGeneration != obj.GetGeneration() {
		return true
	}
	return false
}

// Delete implements predicate.TypedPredicate.
func (c CustomPredicate) Delete(e event.TypedDeleteEvent[client.Object]) bool {
	return true
}

// Generic implements predicate.TypedPredicate.
func (c CustomPredicate) Generic(e event.TypedGenericEvent[client.Object]) bool {
	return true
}

// Update implements predicate.TypedPredicate.
func (c CustomPredicate) Update(e event.TypedUpdateEvent[client.Object]) bool {
	oldObj := convertObject(e.ObjectOld)
	newObj := convertObject(e.ObjectNew)

	if oldObj.GetGeneration() != newObj.GetGeneration() {
		return true
	}

	readyCondition := meta.FindStatusCondition(newObj.GetConditions(), condition.ConditionTypeReady)
	if readyCondition == nil || readyCondition.Status != metav1.ConditionTrue {
		return true
	}
	if readyCondition.ObservedGeneration != newObj.GetGeneration() {
		return true
	}

	return false
}
