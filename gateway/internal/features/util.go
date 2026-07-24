// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package features

import "sort"

// SortFeatures based on their priority
// the higher the priority, the later the feature is applied
// this is important because some features might depend on other features
func SortFeatures[T FeatureBuilder](featureList []Feature[T]) []Feature[T] {
	sort.Slice(featureList, func(i, j int) bool {
		return featureList[i].Priority() < featureList[j].Priority()
	})
	return featureList
}

func ToSlice[K comparable, T any](m map[K]T) []T {
	s := make([]T, 0, len(m))
	for _, v := range m {
		s = append(s, v)
	}
	return s
}
