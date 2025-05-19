// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur

import (
	"errors"

	"github.com/cyberark/conjur-api-go/conjurapi/response"
)

func AsError(err error) (*response.ConjurError, bool) {
	var cErr *response.ConjurError
	if errors.As(err, &cErr) {
		return cErr, true
	}
	return nil, false
}
