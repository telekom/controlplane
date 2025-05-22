// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
)

func GetRealmByRef(ctx context.Context, ref types.ObjectRef) (bool, *v1.Realm, error) {
	client := client.ClientFromContextOrDie(ctx)

	realm := &v1.Realm{}
	err := client.Get(ctx, ref.K8s(), realm)
	if err != nil {
		return false, nil, errors.Wrapf(err, "failed to get realm %s", ref.String())
	}

	if !meta.IsStatusConditionTrue(realm.GetConditions(), condition.ConditionTypeReady) {
		return false, realm, nil
	}
	return true, realm, nil
}
