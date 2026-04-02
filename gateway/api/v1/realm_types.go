// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"errors"
	"net/url"
	"path"

	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RealmSpec defines the desired state of Realm
type RealmSpec struct {

	// Gateway is the Gateway that is associated with the Realm
	// If empty, the realm considered a virtual realm
	Gateway *types.ObjectRef `json:"gateway,omitempty"`
	// Urls is the list of Gateway URLs that this realm should accept traffic from
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Format=uri
	Urls []string `json:"urls"`
	// IssuerUrls is the list of token issuer URLs that are trusted for this realm
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Format=uri
	IssuerUrls []string `json:"issuerUrls"`
	// DefaultConsumers is a list of consumers that are allowed to access the Realm for all Routes
	// +listType=set
	// +kubebuilder:default={}
	DefaultConsumers []string `json:"defaultConsumers"`
}

// RealmStatus defines the observed state of Realm
type RealmStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Virtual indicates that this is a virtual realm and has no Gateway associated with it
	Virtual bool `json:"virtual"`

	IssuerRoute    *types.ObjectRef `json:"issuerRoute,omitempty"`
	IssuerUrl      string           `json:"issuerUrl,omitempty"`
	CertsRoute     *types.ObjectRef `json:"certsRoute,omitempty"`
	CertsUrl       string           `json:"certsUrl,omitempty"`
	DiscoveryRoute *types.ObjectRef `json:"discoveryRoute,omitempty"`
	DiscoveryUrl   string           `json:"discoveryUrl,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Realm is the Schema for the realms API
type Realm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RealmSpec   `json:"spec,omitempty"`
	Status RealmStatus `json:"status,omitempty"`
}

var _ types.Object = &Realm{}

func (r *Realm) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Realm) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

func (r *Realm) AsUpstream(apiBasePath string) (ups Upstream, err error) {
	// Use the first URL as the upstream URL
	if len(r.Spec.Urls) == 0 {
		return ups, errors.New("no upstreams found")
	}
	url, err := url.Parse(r.Spec.Urls[0])
	if err != nil {
		return ups, err
	}
	ups = Upstream{
		Scheme: url.Scheme,
		Host:   url.Hostname(),
		Port:   GetPortOrDefaultFromScheme(url),
		Path:   path.Join(url.Path, apiBasePath),
	}
	return
}

func (r *Realm) AsDownstream(apiBasePath string) (dws Downstream, err error) {
	// Use the first URL as the downstream URL
	if len(r.Spec.Urls) == 0 {
		return dws, errors.New("no downstreams found")
	}
	url, err := url.Parse(r.Spec.Urls[0])
	if err != nil {
		return dws, err
	}
	// Use the first issuer URL
	issuerUrl := ""
	if len(r.Spec.IssuerUrls) > 0 {
		issuerUrl = r.Spec.IssuerUrls[0]
	}
	dws = Downstream{
		Host:      url.Hostname(),
		Port:      GetPortOrDefaultFromScheme(url),
		Path:      path.Join(url.Path, apiBasePath),
		IssuerUrl: issuerUrl,
	}
	return
}

// +kubebuilder:object:root=true

// RealmList contains a list of Realm
type RealmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Realm `json:"items"`
}

var _ types.ObjectList = &RealmList{}

func (l *RealmList) GetItems() []types.Object {
	items := make([]types.Object, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Realm{}, &RealmList{})
}
