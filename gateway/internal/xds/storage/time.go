// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func timestampFromUnixNano(value int64) *timestamppb.Timestamp {
	return timestamppb.New(time.Unix(0, value).UTC())
}
