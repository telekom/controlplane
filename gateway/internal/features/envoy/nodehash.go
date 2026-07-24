// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/go-logr/logr"
)

// nodeMetadataRoleKey is the node.metadata field that identifies which Gateway
// a connecting Envoy belongs to. All pods of one Gateway Deployment report the
// same role, so they share a single snapshot. Envoy's bootstrap must set this.
const nodeMetadataRoleKey = "role"

// fallbackNodeID keys the snapshot for nodes that do not report a role. They all
// share one entry rather than each spawning a distinct (empty) snapshot.
const fallbackNodeID = "unknown-role"

var _ cachev3.NodeHash = nodeHash{}

// nodeHash keys the snapshot cache on the Gateway identity (node.metadata.role),
// NOT the per-pod node.id. This makes all 100+ pods of a Gateway share one
// snapshot, so a change is computed and pushed once and fanned out to all
// connected streams.
type nodeHash struct{}

// ID implements [cachev3.NodeHash].
func (nodeHash) ID(node *corev3.Node) string {
	if node == nil {
		return fallbackNodeID
	}
	fields := node.GetMetadata().GetFields()
	if role, ok := fields[nodeMetadataRoleKey]; ok {
		if s := role.GetStringValue(); s != "" {
			return s
		}
	}
	return fallbackNodeID
}

// newCacheLogger adapts a logr.Logger to the go-control-plane cache logger.
func newCacheLogger(logger logr.Logger) cacheLogger {
	return cacheLogger{log: logger.WithName("envoy.cache")}
}

type cacheLogger struct {
	log logr.Logger
}

func (l cacheLogger) Debugf(format string, args ...any) {
	l.log.V(1).Info(fmt.Sprintf(format, args...))
}
func (l cacheLogger) Infof(format string, args ...any)  { l.log.V(1).Info(fmt.Sprintf(format, args...)) }
func (l cacheLogger) Warnf(format string, args ...any)  { l.log.Info(fmt.Sprintf(format, args...)) }
func (l cacheLogger) Errorf(format string, args ...any) { l.log.Info(fmt.Sprintf(format, args...)) }
