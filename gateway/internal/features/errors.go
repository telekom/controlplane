// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package features

import (
	"fmt"
)

var ErrNoRoute = fmt.Errorf("no route found in builder context")
var ErrNoConsumer = fmt.Errorf("no consumer found in builder context")
