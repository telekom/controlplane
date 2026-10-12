// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	// ProviderCALabelKey marks ConfigMaps that are provider CA sources or durable copies.
	ProviderCALabelKey = "cp.ei.telekom.de/provider-ca"
	// ProviderCALabelValue is the only accepted value of ProviderCALabelKey.
	ProviderCALabelValue = "true"
)

// CABundleConfigMapRef selects a key of a ConfigMap in the namespace of the referencing object.
type CABundleConfigMapRef struct {
	// Name of the ConfigMap in the referencing object's namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name"`
	// Key of the PEM CA bundle in the ConfigMap's data.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[-._a-zA-Z0-9]+$`
	Key string `json:"key"`
}

// ProviderTLS configures additional trust anchors for a provider connection.
// Omitting it, or its reference, means system trust.
type ProviderTLS struct {
	// CABundleConfigMapRef selects the PEM CA bundle that is added to the system roots.
	// +optional
	CABundleConfigMapRef *CABundleConfigMapRef `json:"caBundleConfigMapRef,omitempty"`
}

// Validate checks the Kubernetes syntax of name and key.
func (r *CABundleConfigMapRef) Validate() error {
	if r == nil {
		return nil
	}
	if errs := validation.IsDNS1123Subdomain(r.Name); len(errs) > 0 {
		return fmt.Errorf("invalid caBundleConfigMapRef name %q: %s", r.Name, strings.Join(errs, "; "))
	}
	if errs := validation.IsConfigMapKey(r.Key); len(errs) > 0 {
		return fmt.Errorf("invalid caBundleConfigMapRef key %q: %s", r.Key, strings.Join(errs, "; "))
	}
	return nil
}

// Ref returns the CA reference or nil when system trust is selected.
func (t *ProviderTLS) Ref() *CABundleConfigMapRef {
	if t == nil {
		return nil
	}
	return t.CABundleConfigMapRef
}

// Validate validates the optional reference.
func (t *ProviderTLS) Validate() error {
	return t.Ref().Validate()
}

// HasProviderCALabel reports whether labels carry the exact required provider-ca label.
func HasProviderCALabel(labels map[string]string) bool {
	return labels[ProviderCALabelKey] == ProviderCALabelValue
}

func (r *CABundleConfigMapRef) DeepCopyInto(out *CABundleConfigMapRef) {
	*out = *r
}

func (r *CABundleConfigMapRef) DeepCopy() *CABundleConfigMapRef {
	if r == nil {
		return nil
	}
	out := new(CABundleConfigMapRef)
	r.DeepCopyInto(out)
	return out
}

func (t *ProviderTLS) DeepCopyInto(out *ProviderTLS) {
	*out = *t
	out.CABundleConfigMapRef = t.CABundleConfigMapRef.DeepCopy()
}

func (t *ProviderTLS) DeepCopy() *ProviderTLS {
	if t == nil {
		return nil
	}
	out := new(ProviderTLS)
	t.DeepCopyInto(out)
	return out
}
