// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import "context"

func populateRealmName(ctx context.Context, hc *HandlingContext) error {
	hc.Zone.Status.RealmName = hc.Environment.Spec.RealmName
	return nil
}
