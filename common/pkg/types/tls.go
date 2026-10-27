// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

// ConfigMapKeyRef references a key in the data of a ConfigMap
// in the namespace of the referencing resource.
type ConfigMapKeyRef struct {
	// Name of the ConfigMap.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name"`
	// Key in the ConfigMap data containing PEM encoded CA certificates.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[-._a-zA-Z0-9]+$`
	Key string `json:"key"`
}

// TLSConfig configures additional trusted CA certificates.
// The referenced certificates are trusted in addition to the system roots.
type TLSConfig struct {
	// ConfigMapRef references the PEM encoded CA certificates.
	// +kubebuilder:validation:Required
	ConfigMapRef ConfigMapKeyRef `json:"configMapRef"`
}

func (in *ConfigMapKeyRef) DeepCopyInto(out *ConfigMapKeyRef) {
	*out = *in
}

func (in *ConfigMapKeyRef) DeepCopy() *ConfigMapKeyRef {
	if in == nil {
		return nil
	}
	out := new(ConfigMapKeyRef)
	in.DeepCopyInto(out)
	return out
}

func (in *TLSConfig) DeepCopyInto(out *TLSConfig) {
	*out = *in
}

func (in *TLSConfig) DeepCopy() *TLSConfig {
	if in == nil {
		return nil
	}
	out := new(TLSConfig)
	in.DeepCopyInto(out)
	return out
}
