// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package hash

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"

	"k8s.io/apimachinery/pkg/util/dump"
	"k8s.io/apimachinery/pkg/util/rand"
)

// ComputeHash computes a hash for the given content.
//
// See https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/controller_utils.go#L1229
func ComputeHash(content any, collisionCount *uint32) string {
	hasher := fnv.New32a()
	hasher.Reset()
	_, err := fmt.Fprintf(hasher, "%v", dump.ForHash(content))
	if err != nil {
		panic(err)
	}

	if collisionCount != nil {
		collisionCountBytes := make([]byte, 8)
		binary.LittleEndian.PutUint32(collisionCountBytes, *collisionCount)
		hasher.Write(collisionCountBytes)
	}

	return rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
}
