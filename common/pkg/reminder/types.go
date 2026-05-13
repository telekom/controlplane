// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package reminder

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
)

// Threshold defines when a reminder should fire relative to a deadline.
type Threshold struct {
	// Before is how long before the deadline this reminder fires.
	// For example, "720h" means 30 days before the deadline.
	// +kubebuilder:validation:Required
	Before metav1.Duration `json:"before"`

	// Repeat optionally re-sends the reminder at this interval once the
	// "Before" window is entered. For example, Before=168h + Repeat=24h
	// sends a reminder at 7d, 6d, 5d, … , 1d before the deadline.
	// If omitted, the reminder fires exactly once for this threshold.
	// +optional
	Repeat *metav1.Duration `json:"repeat,omitempty"`
}

// DeepCopyInto copies the receiver into out.
func (in *Threshold) DeepCopyInto(out *Threshold) {
	*out = *in
	if in.Repeat != nil {
		out.Repeat = new(metav1.Duration)
		*out.Repeat = *in.Repeat
	}
}

// DeepCopy returns a deep copy of the Threshold.
func (in *Threshold) DeepCopy() *Threshold {
	if in == nil {
		return nil
	}
	out := new(Threshold)
	in.DeepCopyInto(out)
	return out
}

// SentReminder tracks a reminder that was sent for a specific threshold.
type SentReminder struct {
	// Threshold is the key identifying which threshold triggered this reminder.
	// It is the string representation of the Threshold.Before duration (e.g. "720h0m0s").
	Threshold string `json:"threshold"`

	// Ref is the reference to the created notification or other resource.
	Ref types.ObjectRef `json:"ref"`

	// SentAt is when this reminder was last sent for this threshold.
	SentAt metav1.Time `json:"sentAt"`
}

// DeepCopyInto copies the receiver into out.
func (in *SentReminder) DeepCopyInto(out *SentReminder) {
	*out = *in
	in.Ref.DeepCopyInto(&out.Ref)
	in.SentAt.DeepCopyInto(&out.SentAt)
}

// DeepCopy returns a deep copy of the SentReminder.
func (in *SentReminder) DeepCopy() *SentReminder {
	if in == nil {
		return nil
	}
	out := new(SentReminder)
	in.DeepCopyInto(out)
	return out
}
