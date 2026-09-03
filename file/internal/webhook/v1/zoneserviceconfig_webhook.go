// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/file/internal/handler/util"
)

var zoneserviceconfiglog = logf.Log.WithName("zoneserviceconfig-resource")

// SetupZoneServiceConfigWebhookWithManager registers the webhook for ZoneServiceConfig in the manager.
func SetupZoneServiceConfigWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &filev1.ZoneServiceConfig{}).
		WithValidator(&ZoneServiceConfigValidator{
			client: mgr.GetClient(),
		}).
		Complete()
}

// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch
// +kubebuilder:webhook:path=/validate-file-cp-ei-telekom-de-v1-zoneserviceconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=file.cp.ei.telekom.de,resources=zoneserviceconfigs,verbs=create;update,versions=v1,name=vzoneserviceconfig-v1.kb.io,admissionReviewVersions=v1

// ZoneServiceConfigValidator struct is responsible for validating the ZoneServiceConfig resource.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
//
// +kubebuilder:object:generate=false
type ZoneServiceConfigValidator struct {
	client client.Client
}

var _ admission.Validator[*filev1.ZoneServiceConfig] = &ZoneServiceConfigValidator{}

func (v *ZoneServiceConfigValidator) ValidateCreate(ctx context.Context, obj *filev1.ZoneServiceConfig) (admission.Warnings, error) {
	zoneserviceconfiglog.V(1).Info("validate create", "name", obj.GetName(), "namespace", obj.GetNamespace())
	return v.ValidateCreateOrUpdate(ctx, obj)
}

func (v *ZoneServiceConfigValidator) ValidateUpdate(ctx context.Context, _, obj *filev1.ZoneServiceConfig) (admission.Warnings, error) {
	zoneserviceconfiglog.V(1).Info("validate update", "name", obj.GetName(), "namespace", obj.GetNamespace())
	return v.ValidateCreateOrUpdate(ctx, obj)
}

func (v *ZoneServiceConfigValidator) ValidateDelete(ctx context.Context, obj *filev1.ZoneServiceConfig) (admission.Warnings, error) {
	zoneserviceconfiglog.V(1).Info("validate delete", "name", obj.GetName(), "namespace", obj.GetNamespace())
	return nil, nil
}

func (v *ZoneServiceConfigValidator) ValidateCreateOrUpdate(ctx context.Context, obj *filev1.ZoneServiceConfig) (admission.Warnings, error) {
	var allErrs field.ErrorList
	var warnings admission.Warnings

	namespace := util.GetZoneNamespace(obj.Spec.Zone)
	if obj.Name != obj.Spec.Zone.Name || obj.Namespace != namespace {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("metadata").Child("name"),
			obj.GetName(),
			fmt.Sprintf("ZoneServiceConfig name must match the Zone name and it must be in the namespace of the Zone it configures. Expected name: %q, namespace: %q", obj.Spec.Zone.Name, namespace),
		))
	}

	// Validate that a Zone with the same name and namespace exists
	zone := &adminv1.Zone{}
	if err := v.client.Get(ctx, obj.Spec.Zone.K8s(), zone); err != nil {
		if apierrors.IsNotFound(err) {
			allErrs = append(allErrs, field.Required(
				field.NewPath("metadata").Child("name"),
				fmt.Sprintf("Zone with name %q not found in namespace %q", obj.GetName(), obj.GetNamespace()),
			))
		} else {
			return nil, apierrors.NewInternalError(fmt.Errorf("failed to get Zone: %w", err))
		}
	}

	if len(allErrs) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(filev1.GroupVersion.WithKind("ZoneServiceConfig").GroupKind(), obj.Name, allErrs)
}
