// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import "slices"

// Add adds the given value to the range set. Add will insert a new (v, v) range
// or expand an existing range as necessary.
func (r *RangeSet) Add(v uint64) bool {
	// If the set is empty, add (v, v)
	s := *r
	if len(s) == 0 {
		*r = append(s, Range{Start: v, End: v})
		return true
	}

	// If v exists within an existing range, do nothing
	i, found := s.search(v)
	if found {
		return false
	}

	// Add v to the previous range?
	addPrev := i > 0 && v == s[i-1].End+1
	if addPrev {
		s[i-1].End = v
	}

	// Add v to the next range?
	addNext := i < len(s) && v == s[i].Start-1
	if addNext {
		s[i].Start = v
	}

	switch {
	case addPrev && addNext:
		// Merge the previous and next ranges
		*r = slices.Replace(s, i-1, i+1, Range{
			Start: s[i-1].Start,
			End:   s[i].End,
		})

	case !addPrev && !addNext:
		// Insert (v, v) at I
		*r = slices.Insert(s, i, Range{Start: v, End: v})
	}
	return true
}

func (r RangeSet) Contains(v uint64) bool {
	_, ok := r.search(v)
	return ok
}

func (r RangeSet) Continuous() bool {
	return len(r) >= 1
}

func (r RangeSet) Size() int {
	size := 0
	for _, r := range r {
		size += int(r.End - r.Start + 1)
	}
	return size
}

// search returns the index of the range containing the value or the position
// where such a range would exist.
//
//   - If the value is contained within a range, search returns the index of that
//     element.
//   - If the value is less than the Start of the first range, search returns 0.
//   - If the value is greater than the End of the last range, search returns
//     len(r).
//   - If the value is between two neighboring ranges A and B, search returns the
//     index of B.
func (r RangeSet) search(v uint64) (int, bool) {
	return slices.BinarySearchFunc(r, v, func(r Range, v uint64) int {
		switch {
		case r.End < v:
			return -1
		case r.Start > v:
			return +1
		default:
			return 0
		}
	})
}
