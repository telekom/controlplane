// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/telekom/controlplane/common/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// LabelPredicate filters events for resources with domain=aurora label.
var LabelPredicate = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetLabels()[config.DomainLabelKey] == "aurora"
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetLabels()[config.DomainLabelKey] == "aurora"
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetLabels()[config.DomainLabelKey] == "aurora"
	},
	GenericFunc: func(e event.GenericEvent) bool {
		return e.Object.GetLabels()[config.DomainLabelKey] == "aurora"
	},
}
