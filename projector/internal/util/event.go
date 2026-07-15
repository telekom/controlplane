// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
)

func MapEventTrigger(obj eventv1.EventTrigger) model.EventTrigger {
	trigger := model.EventTrigger{}

	if obj.ResponseFilter != nil {
		trigger.ResponseFilter = &model.ResponseFilter{
			Paths: obj.ResponseFilter.Paths,
			Mode:  obj.ResponseFilter.Mode.String(),
		}
	}

	if obj.SelectionFilter != nil {
		sf := &model.SelectionFilter{
			Attributes: obj.SelectionFilter.Attributes,
		}
		if obj.SelectionFilter.Expression != nil {
			sf.Expression = string(obj.SelectionFilter.Expression.Raw)
		}
		trigger.SelectionFilter = sf
	}

	return trigger
}
