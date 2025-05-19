// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"fmt"
	"hash/fnv"

	"github.com/telekom/controlplane/secret-manager/pkg/backend"
)

var _ Cache[backend.SecretId, backend.Secret[backend.SecretId]] = (*ShardedCache[backend.SecretId, backend.Secret[backend.SecretId]])(nil)

type ShardedCache[T backend.SecretId, S backend.Secret[T]] struct {
	shards     []Cache[T, S]
	shardCount uint8
}

func NewShardedCache[T backend.SecretId, S backend.Secret[T]](shardCount uint8) *ShardedCache[T, S] {
	shards := make([]Cache[T, S], shardCount)
	if shardCount == 0 {
		panic("shardCount must be greater than 0")
	}
	for i := uint8(0); i < shardCount; i++ {
		shards[i] = NewSimpleCache[T, S]()
	}
	return &ShardedCache[T, S]{shards: shards, shardCount: shardCount}
}

func (sc *ShardedCache[T, S]) getShard(key string) Cache[T, S] {
	hash := fnv.New32a()
	_, err := hash.Write([]byte(key))
	if err != nil {
		fmt.Printf("⚠️ Error hashing key: %v\n", err)
		return sc.shards[0]
	}
	return sc.shards[hash.Sum32()%uint32(sc.shardCount)]
}

func (sc *ShardedCache[T, S]) Get(id string) (CacheItem[T, S], bool) {
	return sc.getShard(id).Get(id)
}

func (sc *ShardedCache[T, S]) Set(id string, item CacheItem[T, S]) {
	sc.getShard(id).Set(id, item)
}

func (sc *ShardedCache[T, S]) Delete(id string) {
	sc.getShard(id).Delete(id)
}
