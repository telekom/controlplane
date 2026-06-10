// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
)

var environmentlog = logf.Log.WithName("environment-resource")

// SetupEnvironmentWebhookWithManager registers the webhook for Environment in the manager.
func SetupEnvironmentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &adminv1.Environment{}).
		WithValidator(&EnvironmentCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-admin-cp-ei-telekom-de-v1-environment,mutating=false,failurePolicy=fail,sideEffects=None,groups=admin.cp.ei.telekom.de,resources=environments,verbs=create;update,versions=v1,name=venvironment-v1.kb.io,admissionReviewVersions=v1

// EnvironmentCustomValidator struct is responsible for validating the Environment resource
// when it is created, updated, or deleted.
type EnvironmentCustomValidator struct {
	Client client.Client
}

var _ admission.Validator[*adminv1.Environment] = &EnvironmentCustomValidator{}

// ValidateCreate implements admission.CustomValidator so a webhook will be registered for the type Environment.
func (v *EnvironmentCustomValidator) ValidateCreate(ctx context.Context, environment *adminv1.Environment) (admission.Warnings, error) {
	environmentlog.Info("Validation for Environment upon creation", "name", environment.GetName())

	if err := v.validateRealmNameUniqueness(ctx, environment); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements admission.CustomValidator so a webhook will be registered for the type Environment.
func (v *EnvironmentCustomValidator) ValidateUpdate(ctx context.Context, oldEnvironment, newEnvironment *adminv1.Environment) (admission.Warnings, error) {
	environmentlog.Info("Validation for Environment upon update", "name", newEnvironment.GetName())

	// RealmName is immutable once set — changing it after resources have been
	// provisioned in the realm (gateway routes, identity clients, etc.) would
	// leave orphaned objects in the old realm that controllers cannot clean up.
	if oldEnvironment.Spec.RealmName != "" && oldEnvironment.Spec.RealmName != newEnvironment.Spec.RealmName {
		return nil, fmt.Errorf("spec.realmName is immutable once set (was %q)", oldEnvironment.Spec.RealmName)
	}

	if err := v.validateRealmNameUniqueness(ctx, newEnvironment); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements admission.CustomValidator so a webhook will be registered for the type Environment.
func (v *EnvironmentCustomValidator) ValidateDelete(_ context.Context, _ *adminv1.Environment) (admission.Warnings, error) {
	return nil, nil
}

// validateRealmNameUniqueness ensures no other Environment across the cluster uses the same effective realm name.
func (v *EnvironmentCustomValidator) validateRealmNameUniqueness(ctx context.Context, environment *adminv1.Environment) error {
	realmName := environment.Spec.RealmName

	var envList adminv1.EnvironmentList
	if err := v.Client.List(ctx, &envList,
		client.MatchingFields{IndexFieldRealmName: realmName},
	); err != nil {
		return fmt.Errorf("listing environments: %w", err)
	}

	for i := range envList.Items {
		other := &envList.Items[i]
		if other.UID == environment.UID {
			continue
		}
		return fmt.Errorf("realm name %q is already used by Environment %s/%s", realmName, other.Namespace, other.Name)
	}

	return nil
}
