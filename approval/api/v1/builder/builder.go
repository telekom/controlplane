// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"reflect"
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
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
)

type ApprovalResult string

var (
	DefaultStrategy v1.ApprovalStrategy = v1.ApprovalStrategySimple

	// ApprovalResultGranted is returned when the approval is granted
	ApprovalResultGranted ApprovalResult = "Granted"
	// ApprovalResultDenied is returned when the approval is denied
	ApprovalResultDenied ApprovalResult = "Denied"
	// ApprovalResultPending is returned when the approval request is waiting for approval
	ApprovalResultPending ApprovalResult = "Pending"
	// ApprovalResultNone is returned when the builder has already been run and nothing happened
	ApprovalResultNone ApprovalResult = "None"
)

type ApprovalBuilder interface {
	WithHashValue(hashValue any) ApprovalBuilder
	WithRequester(requester *v1.Requester) ApprovalBuilder
	WithStrategy(strategy v1.ApprovalStrategy) ApprovalBuilder
	WithDecider(decider v1.Decider) ApprovalBuilder
	WithAction(action string) ApprovalBuilder
	WithTrustedTeams(trustedTeams []string) ApprovalBuilder
	Build(ctx context.Context) (ApprovalResult, error)

	GetApprovalRequest() *v1.ApprovalRequest
	GetApproval() *v1.Approval
	GetOwner() types.Object
}

var _ ApprovalBuilder = &approvalBuilder{}

type approvalBuilder struct {
	ran          atomic.Bool
	Client       cclient.JanitorClient
	Owner        types.Object
	Request      *v1.ApprovalRequest
	Approval     *v1.Approval
	TrustedTeams []string

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
	b.Request.Spec.Resource = *types.TypedObjectRefFromObject(b.Owner, b.Client.Scheme())
}

func (b *approvalBuilder) WithTrustedTeams(trustedTeams []string) ApprovalBuilder {
	b.TrustedTeams = trustedTeams
	return b
}

func (b *approvalBuilder) WithRequester(requester *v1.Requester) ApprovalBuilder {
	b.Request.Spec.Requester = *requester
	return b
}

func (b *approvalBuilder) requireRequester() error {
	if b.Request.Spec.Requester.Name == "" {
		return errors.New("missing required value: Requester")
	}
	return nil
}

func (b *approvalBuilder) WithStrategy(strategy v1.ApprovalStrategy) ApprovalBuilder {
	b.Request.Spec.Strategy = strategy
	return b
}

func (b *approvalBuilder) WithDecider(Decider v1.Decider) ApprovalBuilder {
	b.Request.Spec.Decider = Decider
	return b
}

func (b *approvalBuilder) WithAction(action string) ApprovalBuilder {
	b.Request.Spec.Action = action
	return b
}

func (b *approvalBuilder) Build(ctx context.Context) (ApprovalResult, error) {
	if b.ran.Load() {
		return ApprovalResultNone, errors.New("builder has already been run")
	}
	b.ran.Store(true)

	b.setWithHash()
	if err := b.requireRequester(); err != nil {
		return ApprovalResultNone, err
	}

	log := log.FromContext(ctx)

	approvalReq := b.Request.DeepCopy()
	mutate := func() error {
		if approvalReq.Spec.State != "" && approvalReq.Spec.State != v1.ApprovalStatePending {
			return nil // no need to create approval request as it already exists
		}

		if err := controllerutil.SetControllerReference(b.Owner, approvalReq, b.Client.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		approvalReq.Spec = b.Request.Spec

		if b.isRequesterFromTrustedTeam() {
			approvalReq.Spec.Strategy = v1.ApprovalStrategyAuto
		}

		if approvalReq.Spec.Strategy == v1.ApprovalStrategyAuto {
			approvalReq.Spec.State = v1.ApprovalStateGranted
		}
		return nil
	}

	_, err := b.Client.CreateOrUpdate(ctx, approvalReq, mutate)
	if err != nil {
		return ApprovalResultNone, errors.Wrap(err, "failed to create approval-request")
	}
	b.Request = approvalReq
	b.Owner.SetCondition(condition.NewProcessingCondition("ApprovalPending", "Approval is pending"))
	b.Owner.SetCondition(condition.NewNotReadyCondition("ApprovalPending", "Approval is pending"))

	_, err = b.Client.Cleanup(ctx, &v1.ApprovalRequestList{}, cclient.OwnedBy(b.Owner))
	if err != nil {
		return ApprovalResultPending, errors.Wrap(err, "failed to cleanup approval-requests")
	}

	err = b.Client.Get(ctx, client.ObjectKeyFromObject(b.Approval), b.Approval)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ApprovalResultPending, errors.Wrap(err, "failed to get approval")
		}

		log.V(5).Info("Approval does not exist")
		return ApprovalResultPending, nil

	} else {
		log.V(5).Info("Approval exists")

		if b.Approval.Spec.State != v1.ApprovalStateGranted {
			log.V(5).Info("Approval is not granted and must not be provisioned")
			b.Owner.SetCondition(condition.NewNotReadyCondition("NoApproval", "Approval is either rejected or suspended"))
			b.Owner.SetCondition(condition.NewBlockedCondition("Approval is either rejected or suspended"))

			// cleanup
			return ApprovalResultDenied, nil
		}

		if b.Approval.Spec.ApprovedRequest != nil && b.Approval.Spec.ApprovedRequest.Name != approvalReq.Name {
			log.V(5).Info("Approval is not for this ApiSub. Returning early")
			b.Owner.SetCondition(condition.NewNotReadyCondition("ApprovalPending", "Approval is pending"))
			b.Owner.SetCondition(condition.NewBlockedCondition("Approval is pending"))
			return ApprovalResultPending, nil
		}
	}

	if b.Approval.Spec.State == v1.ApprovalStateGranted {
		b.Owner.SetCondition(condition.NewReadyCondition("ApprovalGranted", "Approval is granted"))
		b.Owner.SetCondition(condition.NewProcessingCondition("ApprovalGranted", "Approval is granted"))
		return ApprovalResultGranted, nil
	}

	return ApprovalResultDenied, nil
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

func (b *approvalBuilder) isRequesterFromTrustedTeam() bool {
	requesterTeamName := b.Request.Spec.Requester.Name

	for i := range b.TrustedTeams {
		if strings.EqualFold(b.TrustedTeams[i], requesterTeamName) {
			return true
		}
	}
	return false
}
