// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
)

// Step is a single unit of zone bootstrapping work.
// Each step reads its dependencies from hc and writes its results back.
type Step func(ctx context.Context, hc *HandlingContext) error

// HandlingContext carries all state through the zone bootstrapping pipeline.
// Steps read their dependencies and write their results into this struct.
type HandlingContext struct {
	// Input (set during construction)
	Zone        *adminv1.Zone
	Environment *adminv1.Environment
	Namespace   *corev1.Namespace

	// Shared configuration derived once at initialization
	DefaultClaims []identityapi.ClaimConfig

	// Intermediate resources — populated by steps as they execute
	IdentityProvider      *identityapi.IdentityProvider
	DefaultIdentityRealm  *identityapi.Realm
	InternalIdentityRealm *identityapi.Realm
	GatewayAdminClient    *identityapi.Client
	GatewayClient         *identityapi.Client
	Gateway               *gatewayapi.Gateway
	GatewayConsumer       *gatewayapi.Consumer
	TeamApiIdentityRealm  *identityapi.Realm
	AiGateway             *gatewayapi.Gateway
}

// newHandlingContext fetches the Environment, creates/updates the zone Namespace,
// and builds the shared claims configuration.
func newHandlingContext(ctx context.Context, obj *adminv1.Zone) (*HandlingContext, error) {
	envName := contextutil.EnvFromContextOrDie(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	environment := &adminv1.Environment{}
	err := c.Get(ctx, client.ObjectKey{Name: envName, Namespace: envName}, environment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("environment %s not found", envName)
		}
		return nil, ctrlerrors.RetryableErrorf("failed to get environment %s: %s", envName, err)
	}

	// Namespace
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: strings.ToLower(fmt.Sprintf("%s--%s", environment.Name, obj.Name)),
		},
	}

	mutator := func() error {
		if namespace.Labels == nil {
			namespace.Labels = make(map[string]string)
		}
		namespace.Labels[cconfig.EnvironmentLabelKey] = environment.Name
		namespace.Labels[cconfig.BuildLabelKey(zoneLabelName)] = obj.Name
		return nil
	}
	_, err = c.CreateOrUpdate(ctx, namespace, mutator)
	if err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to create or update namespace %s: %s", namespace.Name, err)
	}

	obj.Status.Namespace = namespace.Name

	defaultPreset, err := obj.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("failed to get default gateway preset for zone %s: %s", obj.Name, err)
	}

	defaultClaims := []identityapi.ClaimConfig{
		{
			Name:  claimOriginZone,
			Value: obj.Name,
			Type:  identityapi.ClaimTypeHardcodedClaim,
		},
		{
			Name:  claimOriginStargate,
			Value: defaultPreset.GetDefaultUrl(),
			Type:  identityapi.ClaimTypeHardcodedClaim,
		},
		{
			Name: claimClientId,
			Type: identityapi.ClaimTypeSessionNote,
		},
	}

	return &HandlingContext{
		Zone:          obj,
		Environment:   environment,
		Namespace:     namespace,
		DefaultClaims: defaultClaims,
	}, nil
}
