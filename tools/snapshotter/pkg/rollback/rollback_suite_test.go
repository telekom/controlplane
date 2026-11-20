// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rollback_test

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
)

func TestRollback(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rollback Suite")
}

const desiredJSON = `{
  "environment": "prod",
  "zone": "aws",
  "id": "my-route",
  "raw": true,
  "state": {
    "service": {"id": "svc-1", "name": "my-route", "host": "my-route", "port": 8080, "protocol": "http", "created_at": 1, "updated_at": 2},
    "route": {"id": "route-1", "name": "my-route", "paths": ["/b", "/a"], "service": {"id": "svc-1"}, "created_at": 1, "updated_at": 2},
    "plugins": [
      {"id": "plugin-2", "name": "rate-limiting", "route": {"id": "route-1"}, "config": {"minute": 10, "policy": null, "unknown_field": {"x": [3, 1]}}, "enabled": true, "created_at": 1},
      {"id": "plugin-1", "name": "acl", "route": {"id": "route-1"}, "config": {"allow": ["b", "a"]}, "enabled": true, "created_at": 1}
    ],
    "upstream": {"id": "up-1", "name": "my-route", "algorithm": "round-robin", "created_at": 1},
    "targets": [
      {"id": "target-1", "target": "10.0.0.1:80", "weight": 100, "upstream": {"id": "up-1"}, "created_at": 1.5}
    ]
  }
}`

// desiredSnapshot returns a fresh raw snapshot fixture.
func desiredSnapshot() *snapshot.Snapshot {
	snap := &snapshot.Snapshot{}
	Expect(json.Unmarshal([]byte(desiredJSON), snap)).To(Succeed())
	return snap
}
