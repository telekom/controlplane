// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/telekom/controlplane/common/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// LabelPredicate is a predicate that filters objects based on the presence of the label.
// It can be used to ensure that only objects relevant to the event controller are processed.
var LabelPredicate = predicate.NewPredicateFuncs(func(object client.Object) bool {
	labels := object.GetLabels()
	if labels == nil {
		return false
	}
	domainValue, ok := labels[config.DomainLabelKey]
	if !ok {
		return false
	}
	return domainValue == "event"
})
