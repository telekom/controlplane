// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approval

import (
	"encoding/json"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
)

type ApprovalProperties struct {
	// Scopes is the list of access-scopes requested.
	Scopes []string
}

// rawProperties mirrors the JSON shape of Requester.Properties.
type rawProperties struct {
	Scopes []string `json:"scopes,omitempty"`

	// ... other fields ...
}

func FromProperties(requester approvalv1.Requester) (props ApprovalProperties, err error) {
	if requester.Properties.Size() == 0 {
		return props, nil
	}

	var raw rawProperties
	if err := json.Unmarshal(requester.Properties.Raw, &raw); err != nil {
		return props, err
	}

	props.Scopes = raw.Scopes

	// ... extract other fields ...

	return props, nil
}
