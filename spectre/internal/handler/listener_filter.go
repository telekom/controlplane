// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

// buildBridgeTrigger merges user-defined filter criteria with the four fixed
// bridge attributes (issue, consumer, provider, kind) into a pubsub Trigger.
//
// Fixed attributes are written last so they always win on collision.
//
// User trigger values are copied as-is (no "payload." prefix — Galaxy compares
// SelectionFilter values as literal strings). User payload paths get a
// "payload." prefix so Galaxy's dot-to-slash JSON Pointer conversion navigates
// into SpectreData.payload.
func buildBridgeTrigger(
	filter *spectrev1.ListenerFilter,
	apiBasePath, consumerId, providerId, kindValue string,
) *pubsubv1.Trigger {
	// Start with the four mandatory selection attributes.
	attrs := map[string]string{
		"issue":    apiBasePath,
		"consumer": consumerId,
		"provider": providerId,
		"kind":     kindValue,
	}

	// Merge user trigger entries underneath (fixed keys written above win).
	if filter != nil {
		for k, v := range filter.Trigger {
			if _, fixed := attrs[k]; !fixed {
				attrs[k] = v
			}
		}
	}

	trigger := &pubsubv1.Trigger{
		SelectionFilter: &pubsubv1.SelectionFilter{
			Attributes: attrs,
		},
	}

	// Build ResponseFilter from user payload paths.
	if filter != nil && len(filter.Payload) > 0 {
		paths := make([]string, len(filter.Payload))
		for i, p := range filter.Payload {
			paths[i] = "payload." + p
		}
		trigger.ResponseFilter = &pubsubv1.ResponseFilter{
			Mode:  pubsubv1.ResponseFilterModeInclude,
			Paths: paths,
		}
	}

	return trigger
}
