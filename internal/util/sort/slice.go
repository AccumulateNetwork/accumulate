// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package sortutil

// FilterInPlace removes elements of L that fail the predicate without
// allocating with O(N) runtime. FilterInPlace may reorder L.
//
// This doesn't really fit in the sortutil package but 🤷 making a new package
// for just this function would be excessive.
func FilterInPlace[V any](l []V, predicate func(V) bool) []V {
	// I = 0..N-1
	for i, n := 0, len(l); i < n; {
		// Get the element I from the end
		j := n - i - 1
		v := l[j]

		// If the element passes the predicate, keep it and move on to the next
		if predicate(v) {
			i++
			continue
		}

		// If V is not the last element, swap it for the last
		if i > 0 {
			l[j] = l[n-1]
		}

		// Truncate
		l = l[:n-1]
		n--
	}

	return l
}
