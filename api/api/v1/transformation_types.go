package v1

// Transformation defines request/response transformations for an API
// This is shared object for both subscriptions and exposures
type Transformation struct {
	// Request defines transformations applied to incoming API requests
	// +kubebuilder:validation:Optional
	Request RequestResponseTransformation `json:"request"`
}

// RequestResponseTransformation defines transformations applied to API requests and responses
type RequestResponseTransformation struct {
	// Headers defines HTTP header modifications for requests
	// +kubebuilder:validation:Optional
	Headers HeaderTransformation `json:"headers"`
}

// HeaderTransformation defines HTTP header modifications
type HeaderTransformation struct {
	// Remove is a list of HTTP header names to remove
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=5
	Remove []string `json:"remove,omitempty"`
	// Add is a list of HTTP headers to add to the request/response
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=5
	Add []string `json:"add,omitempty"`
}
