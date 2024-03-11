// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package sortutil

import (
	"sort"
)

// Search uses a binary search to find and return the smallest index i in [0,
// len(l)) at which cmp(l[i]) ≥ 0, assuming that on the range [0, len(l)),
// cmp(l[i]) ≥ 0 implies cmp(l[i+1]) ≥ 0. That is, Search requires that cmp ≥ 0
// for some (possibly empty) prefix of the input range [0, len(l)) and then < 0
// for the (possibly empty) remainder; Search returns the first ≥ 0 index and
// whether cmp == 0 for that index. If there is no such index, Search returns
// len(l), false.
func Search[T any, S ~[]T](l S, cmp func(entry T) int) (index int, found bool) {
	// TODO Use sort.Find?
	i := sort.Search(len(l), func(i int) bool {
		return cmp(l[i]) >= 0
	})
	if i >= len(l) {
		return i, false
	}

	return i, cmp(l[i]) == 0
}

// BinaryInsert uses Search to find the smallest index i in [0, len(l)) at which
// cmp(l[i]) ≥ 0. If i ≥ len(l), an empty entry is appended. Otherwise an empty
// entry is inserted at i unless cmp == 0. BinaryInsert returns l, the address
// of l[i], and whether a new entry was added.
func BinaryInsert[T any, S ~[]T](l *S, cmp func(entry T) int) (entry *T, added bool) {
	i, found := Search(*l, cmp)
	if found {
		// Entry found
		return &(*l)[i], false
	}

	var zero T
	*l = append(*l, zero)
	if i >= len(*l) {
		// Append
		return &(*l)[i], true
	}

	// Insert
	copy((*l)[i+1:], (*l)[i:])
	(*l)[i] = zero
	return &(*l)[i], true
}

// RemoveAt removes the specified element.
func RemoveAt[T any](l *[]T, i int) {
	copy((*l)[i:], (*l)[i+1:])
	*l = (*l)[:len(*l)-1]
}

// Remove removes the specified element.
func Remove[T any](l *[]T, cmp func(entry T) int) bool {
	i, found := Search(*l, cmp)
	if !found {
		return false
	}
	RemoveAt(l, i)
	return true
}
