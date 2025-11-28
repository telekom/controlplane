// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
)

type ApprovalResult string

var (
	DefaultStrategy v1.ApprovalStrategy = v1.ApprovalStrategySimple

	// ApprovalResultGranted is returned when the approval is granted
	ApprovalResultGranted ApprovalResult = "Granted"
	// ApprovalResultDenied is returned when the approval is denied
	ApprovalResultDenied ApprovalResult = "Denied"
	// ApprovalRequestDenied is returned when the approval request is denied
	ApprovalResultRequestDenied ApprovalResult = "RequestDenied"
	// ApprovalResultPending is returned when the approval request is waiting for approval
	ApprovalResultPending ApprovalResult = "Pending"
	// ApprovalResultNone is returned when the builder has already been run and nothing happened
	ApprovalResultNone ApprovalResult = "None"
)

type ApprovalBuilder interface {
	WithHashValue(hashValue any) ApprovalBuilder
	WithRequester(requester *v1.Requester) ApprovalBuilder
	WithStrategy(strategy v1.ApprovalStrategy) ApprovalBuilder
	WithDecider(decider *v1.Decider) ApprovalBuilder
	WithAction(action string) ApprovalBuilder
	WithTrustedRequesters(trustedRequesters []string) ApprovalBuilder
	Build(ctx context.Context) (ApprovalResult, error)

	GetApprovalRequest() *v1.ApprovalRequest
	GetApproval() *v1.Approval
	GetOwner() types.Object
}

var _ ApprovalBuilder = &approvalBuilder{}

type approvalBuilder struct {
	ran               atomic.Bool
	Client            cclient.JanitorClient
	Owner             types.Object
	Request           *v1.ApprovalRequest
	Approval          *v1.Approval
	TrustedRequesters []string

	hashValue any
}

func NewApprovalBuilder(client cclient.JanitorClient, owner types.Object) ApprovalBuilder {
	ownerKind := reflect.TypeOf(owner).Elem().Name()
	return &approvalBuilder{
		Client: client,
		Owner:  owner,
		Request: &v1.ApprovalRequest{
			Spec: v1.ApprovalRequestSpec{
				State:    v1.ApprovalStatePending,
				Strategy: DefaultStrategy,
				Decider:  v1.Decider{},
			},
		},
		Approval: &v1.Approval{
			ObjectMeta: metav1.ObjectMeta{
				Name:      v1.ApprovalName(ownerKind, owner.GetName()),
				Namespace: owner.GetNamespace(),
			},
		},
		hashValue: "unknown",
	}
}

// WithHashValue is used to make the approval request unique
// The value should be deterministic for each request
func (b *approvalBuilder) WithHashValue(hashValue any) ApprovalBuilder {
	b.hashValue = hashValue
	return b
}

func (b *approvalBuilder) setWithHash() {
	b.Request.Name = v1.ApprovalRequestName(b.Owner, b.hashValue)
	b.Request.Namespace = b.Owner.GetNamespace()
	b.Request.Spec.Target = *types.TypedObjectRefFromObject(b.Owner, b.Client.Scheme())
}

func (b *approvalBuilder) WithTrustedRequesters(trustedRequesters []string) ApprovalBuilder {
	b.TrustedRequesters = trustedRequesters
	return b
}

func (b *approvalBuilder) WithRequester(requester *v1.Requester) ApprovalBuilder {
	b.Request.Spec.Requester = *requester
	return b
}

func (b *approvalBuilder) requireRequester() error {
	if b.Request.Spec.Requester.TeamName == "" {
		return errors.New("missing required value: Requester")
	}
	return nil
}

func (b *approvalBuilder) WithStrategy(strategy v1.ApprovalStrategy) ApprovalBuilder {
	b.Request.Spec.Strategy = strategy
	return b
}

func (b *approvalBuilder) WithDecider(decider *v1.Decider) ApprovalBuilder {
	b.Request.Spec.Decider = *decider
	return b
}

func (b *approvalBuilder) WithAction(action string) ApprovalBuilder {
	b.Request.Spec.Action = action
	return b
}

// Build will trigger the approval process and return the result
// Prioritization of results:
// 1. If the Approval is rejected or suspended --> Denied (must delete all child resources)
// 2. If the ApprovalRequest is rejected --> RequestDenied (child resources must remain untouched)
// 3. If the ApprovalRequest is pending --> Pending (must not provision child resources)
// 4. If the Approval is granted --> Granted (can provision child resources)
func (b *approvalBuilder) Build(ctx context.Context) (finalResult ApprovalResult, err error) {
	if b.ran.Load() {
		return ApprovalResultNone, errors.New("builder has already been run")
	}
	b.ran.Store(true)
	log := log.FromContext(ctx).WithValues("approval.name", b.Approval.Name, "approval.namespace", b.Approval.Namespace)

	b.setWithHash()
	if err := b.requireRequester(); err != nil {
		return ApprovalResultNone, err
	}

	approvalReq := b.Request.DeepCopy()
	mutate := func() error {
		if approvalReq.Spec.State != "" && approvalReq.Spec.State != v1.ApprovalStatePending {
			return nil // no need to create approval request as it already exists
		}

		if err := controllerutil.SetControllerReference(b.Owner, approvalReq, b.Client.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		if b.isRequesterFromTrustedRequesters() {
			approvalReq.Spec.Strategy = v1.ApprovalStrategyAuto
		}

		if approvalReq.Spec.Strategy == v1.ApprovalStrategyAuto {
			approvalReq.Spec.State = v1.ApprovalStateGranted
		}
		return nil
	}

	res, err := b.Client.CreateOrUpdate(ctx, approvalReq, mutate)
	if err != nil {
		return ApprovalResultNone, errors.Wrap(err, "failed to create approval-request")
	}
	b.Request = approvalReq

	if res != controllerutil.OperationResultNone {
		log.V(2).Info("ApprovalRequest reconciled", "operation", res)
		b.Owner.SetCondition(newApprovalGrantedCondition(v1.ApprovalStatePending, "ApprovalRequest has been created or updated"))
	}

	_, err = b.Client.Cleanup(ctx, &v1.ApprovalRequestList{}, cclient.OwnedBy(b.Owner))
	if err != nil {
		return ApprovalResultPending, errors.Wrap(err, "failed to cleanup approval-requests")
	}

	// Priority 1: Check Approval state FIRST (highest priority - overrides everything)
	err = b.Client.Get(ctx, client.ObjectKeyFromObject(b.Approval), b.Approval)
	if err != nil && !apierrors.IsNotFound(err) {
		return ApprovalResultNone, errors.Wrap(err, "failed to get approval")
	}
	approvalExists := err == nil

	// Approval was found
	if approvalExists {
		log.V(2).Info("Approval exists")
		isDenied := b.Approval.Spec.State == v1.ApprovalStateRejected || b.Approval.Spec.State == v1.ApprovalStateSuspended

		if isDenied {
			log.V(1).Info("Approval is rejected or suspended and must not be provisioned")
			b.Owner.SetCondition(newApprovalGrantedCondition(b.Approval.Spec.State, "Approval has been rejected or suspended"))
			return ApprovalResultDenied, nil
		}
	}

	// Priority 2: Check if ApprovalRequest is rejected (only if Approval didn't deny)
	if approvalReq.Spec.State == v1.ApprovalStateRejected {
		log.V(1).Info("ApprovalRequest is rejected")
		b.Owner.SetCondition(newApprovalGrantedCondition(v1.ApprovalStateRejected, "ApprovalRequest has been rejected"))
		return ApprovalResultRequestDenied, nil
	}

	// Priority 3: If Approval doesn't exist --> Pending
	if !approvalExists {
		log.Info("Approval does not exist")
		b.Owner.SetCondition(newApprovalGrantedCondition(v1.ApprovalStatePending, "Approval does not exist yet"))
		return ApprovalResultPending, nil
	}

	// Check if the Approval is for the current ApprovalRequest
	if b.Approval.Spec.ApprovedRequest != nil && !b.Approval.Spec.ApprovedRequest.Equals(approvalReq) {
		log.V(1).Info("Approval is not for this request. Returning early")
		b.Owner.SetCondition(newApprovalGrantedCondition(v1.ApprovalStatePending, "Approval is not for the current ApprovalRequest"))
		return ApprovalResultPending, nil
	}

	// Priority 4: Approval is granted
	if b.Approval.Spec.State == v1.ApprovalStateGranted {
		log.V(2).Info("Approval is granted")
		b.Owner.SetCondition(newApprovalGrantedCondition(v1.ApprovalStateGranted, "Approval has been granted"))
		return ApprovalResultGranted, nil
	}
	// Fallback, but should not be reached
	return ApprovalResultNone, nil
}

func (b *approvalBuilder) GetApprovalRequest() *v1.ApprovalRequest {
	return b.Request
}

func (b *approvalBuilder) GetApproval() *v1.Approval {
	return b.Approval
}

func (b *approvalBuilder) GetOwner() types.Object {
	return b.Owner
}

func (b *approvalBuilder) isRequesterFromTrustedRequesters() bool {
	requesterTeamName := b.Request.Spec.Requester.TeamName
	return slices.ContainsFunc(b.TrustedRequesters, func(name string) bool {
		return strings.EqualFold(name, requesterTeamName)
	})
}
