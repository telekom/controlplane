// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	common "github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

// GetRealm reads the Realm without checking readiness. Its persisted connection snapshot
// may be used for cleanup while the Realm is not Ready.
func GetRealm(ctx context.Context, realmRef *common.ObjectRef) (*identityv1.Realm, error) {
	realm := &identityv1.Realm{}
	if err := client.ClientFromContextOrDie(ctx).Get(ctx, realmRef.K8s(), realm); err != nil {
		return nil, errors.Wrapf(err, "failed to get realm %s", realmRef.String())
	}
	return realm, nil
}

// GetRealmByName reads the Realm and requires it to be Ready.
func GetRealmByName(ctx context.Context, realmRef *common.ObjectRef) (*identityv1.Realm, error) {
	realm, err := GetRealm(ctx, realmRef)
	if err != nil {
		return nil, err
	}
	if readyErr := condition.EnsureReady(realm); readyErr != nil {
		return nil, ctrlerrors.BlockedErrorf("realm %q is not ready", realmRef.String())
	}
	return realm, nil
}

func ValidateRealmStatus(realmStatus *identityv1.RealmStatus) error {
	if realmStatus == nil {
		return fmt.Errorf("realmStatus is nil")
	}
	if realmStatus.IssuerUrl == "" {
		return fmt.Errorf("realmStatus.IssuerUrl is empty")
	}
	if realmStatus.AdminClientId == "" {
		return fmt.Errorf("realmStatus.AdminClientId is empty")
	}
	if realmStatus.AdminUserName == "" {
		return fmt.Errorf("realmStatus.AdminUserName is empty")
	}
	if realmStatus.AdminPassword == "" {
		return fmt.Errorf("realmStatus.AdminPassword is empty")
	}
	if realmStatus.AdminUrl == "" {
		return fmt.Errorf("realmStatus.AdminUrl is empty")
	}
	if realmStatus.AdminTokenUrl == "" {
		return fmt.Errorf("realmStatus.AdminTokenUrl is empty")
	}
	return nil
}
