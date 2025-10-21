// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// RemoteClusterClient handles communication with the legacy cluster via Kubernetes API
type RemoteClusterClient struct {
	client ctrlclient.Client
}

type RemoteClusterConfig struct {
	APIServer  string
	Token      string
	CAData     []byte
	Kubeconfig []byte // Optional: full kubeconfig
}

// NewRemoteClusterClient creates a new client for accessing the remote cluster
func NewRemoteClusterClient(config *RemoteClusterConfig) (*RemoteClusterClient, error) {
	var restConfig *rest.Config
	var err error

	// Try to use kubeconfig if provided
	if len(config.Kubeconfig) > 0 {
		restConfig, err = clientcmd.RESTConfigFromKubeConfig(config.Kubeconfig)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create config from kubeconfig")
		}
	} else {
		// Build config from individual components
		restConfig = &rest.Config{
			Host:        config.APIServer,
			BearerToken: config.Token,
		}

		if len(config.CAData) > 0 {
			restConfig.TLSClientConfig = rest.TLSClientConfig{
				CAData: config.CAData,
			}
		}
	}

	// Create scheme with Approval CRD
	scheme := runtime.NewScheme()
	if err := approvalv1.AddToScheme(scheme); err != nil {
		return nil, errors.Wrap(err, "failed to add approval scheme")
	}

	// Create Kubernetes client
	client, err := ctrlclient.New(restConfig, ctrlclient.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create kubernetes client")
	}

	return &RemoteClusterClient{
		client: client,
	}, nil
}

// NewRemoteClusterClientWithClient creates a client with an existing Kubernetes client (for testing)
func NewRemoteClusterClientWithClient(client ctrlclient.Client) *RemoteClusterClient {
	return &RemoteClusterClient{
		client: client,
	}
}

// GetApproval fetches an Approval resource from the legacy cluster (acp.ei.telekom.de/v1)
// and converts it to the current approval format
func (c *RemoteClusterClient) GetApproval(ctx context.Context, namespace, name string) (*approvalv1.Approval, error) {
	// Fetch as unstructured from legacy API group
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "acp.ei.telekom.de",
		Version: "v1",
		Kind:    "Approval",
	})

	err := c.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, u)
	if err != nil {
		return nil, err
	}

	// Convert to current Approval type
	approval, err := c.convertLegacyApproval(u)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert legacy approval")
	}

	return approval, nil
}

// ListApprovals lists all Approval resources in a namespace from the legacy cluster
func (c *RemoteClusterClient) ListApprovals(ctx context.Context, namespace string) (*approvalv1.ApprovalList, error) {
	// Fetch as unstructured list from legacy API group
	uList := &unstructured.UnstructuredList{}
	uList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "acp.ei.telekom.de",
		Version: "v1",
		Kind:    "ApprovalList",
	})

	err := c.client.List(ctx, uList, ctrlclient.InNamespace(namespace))
	if err != nil {
		return nil, err
	}

	// Convert each item
	approvalList := &approvalv1.ApprovalList{}
	for _, item := range uList.Items {
		approval, err := c.convertLegacyApproval(&item)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert legacy approval in list")
		}
		approvalList.Items = append(approvalList.Items, *approval)
	}

	return approvalList, nil
}

// convertLegacyApproval converts a legacy Approval (acp.ei.telekom.de/v1) to current format
func (c *RemoteClusterClient) convertLegacyApproval(u *unstructured.Unstructured) (*approvalv1.Approval, error) {
	approval := &approvalv1.Approval{}

	// Copy metadata
	approval.Name = u.GetName()
	approval.Namespace = u.GetNamespace()
	approval.UID = u.GetUID()
	approval.ResourceVersion = u.GetResourceVersion()
	approval.Generation = u.GetGeneration()
	approval.CreationTimestamp = u.GetCreationTimestamp()
	approval.Labels = u.GetLabels()
	approval.Annotations = u.GetAnnotations()

	// Extract spec fields from unstructured
	spec, found, err := unstructured.NestedMap(u.Object, "spec")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get spec")
	}
	if !found {
		return nil, errors.New("spec not found in legacy approval")
	}

	// Convert state (GRANTED -> Granted, SUSPENDED -> Suspended, etc.)
	if stateRaw, ok := spec["state"].(string); ok {
		approval.Spec.State = c.convertState(stateRaw)
	}

	// Convert strategy (AUTO -> Auto, SIMPLE -> Simple, etc.)
	if strategyRaw, ok := spec["strategy"].(string); ok {
		approval.Spec.Strategy = c.convertStrategy(strategyRaw)
	}

	return approval, nil
}

// convertState converts legacy state values (GRANTED, SUSPENDED) to current format (Granted, Suspended)
func (c *RemoteClusterClient) convertState(legacyState string) approvalv1.ApprovalState {
	// Legacy uses uppercase (GRANTED, REJECTED, SUSPENDED, PENDING, SEMIGRANTED)
	// Current uses PascalCase (Granted, Rejected, Suspended, Pending, Semigranted)
	switch strings.ToUpper(legacyState) {
	case "GRANTED":
		return approvalv1.ApprovalStateGranted
	case "REJECTED":
		return approvalv1.ApprovalStateRejected
	case "SUSPENDED":
		return approvalv1.ApprovalStateSuspended
	case "PENDING":
		return approvalv1.ApprovalStatePending
	case "SEMIGRANTED":
		return approvalv1.ApprovalStateSemigranted
	default:
		// Fallback to Pending if unknown
		return approvalv1.ApprovalStatePending
	}
}

// convertStrategy converts legacy strategy values (AUTO, SIMPLE) to current format (Auto, Simple)
func (c *RemoteClusterClient) convertStrategy(legacyStrategy string) approvalv1.ApprovalStrategy {
	// Legacy uses uppercase (AUTO, SIMPLE, FOUREYES)
	// Current uses PascalCase (Auto, Simple, FourEyes)
	switch strings.ToUpper(legacyStrategy) {
	case "AUTO":
		return approvalv1.ApprovalStrategyAuto
	case "SIMPLE":
		return approvalv1.ApprovalStrategySimple
	case "FOUREYES", "FOUR_EYES":
		return approvalv1.ApprovalStrategyFourEyes
	default:
		// Fallback to Auto if unknown
		return approvalv1.ApprovalStrategyAuto
	}
}
