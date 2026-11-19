// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package listener

import (
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
)

type Key struct {
	Namespace string
	Name      string
}

type Data struct {
	Meta           shared.Metadata
	StatusPhase    string
	StatusMessage  string
	BasePath       string
	RequestFilter  *model.ListenerFilter
	ResponseFilter *model.ListenerFilter
	Observer       Key
	Consumer       Key
	Provider       Key
}
