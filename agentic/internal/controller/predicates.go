// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/telekom/controlplane/common/pkg/config"
)

const domainName = "agentic"

// LabelPredicate filters events for resources with domain=agentic label.
var LabelPredicate = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetLabels()[config.DomainLabelKey] == domainName
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetLabels()[config.DomainLabelKey] == domainName
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetLabels()[config.DomainLabelKey] == domainName
	},
	GenericFunc: func(e event.GenericEvent) bool {
		return e.Object.GetLabels()[config.DomainLabelKey] == domainName
	},
}
