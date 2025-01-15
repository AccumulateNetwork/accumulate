// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"golang.org/x/exp/slices"
)

type RangeSet struct {
	values.Value[*rangeSet]
}

func (i *IndexDBAccountChain) DidLoad() RangeSet {
	return RangeSet{i.getDidLoad()}
}

func (r RangeSet) Add(v uint64) (bool, error) {
	s, err := r.Get()
	if err != nil {
		return false, err
	}
	added := s.Add(v)
	return added, r.Put(s)
}

func (r RangeSet) Contains(v uint64) (bool, error) {
	s, err := r.Get()
	if err != nil {
		return false, err
	}
	return s.Contains(v), nil
}

type rangeSet []indexRange
type indexRange struct {
	start, end uint64
}

// Add adds the given value to the range set. Add will insert a new (v, v) range
// or expand an existing range as necessary.
func (r *rangeSet) Add(v uint64) bool {
	// If the set is empty, add (v, v)
	s := *r
	if len(s) == 0 {
		*r = append(s, indexRange{start: v, end: v})
		return true
	}

	// If v exists within an existing range, do nothing
	i, found := s.search(v)
	if found {
		return false
	}

	// Add v to the previous range?
	addPrev := i > 0 && v == s[i-1].end+1
	if addPrev {
		s[i-1].end = v
	}

	// Add v to the next range?
	addNext := i < len(s) && v == s[i].start-1
	if addNext {
		s[i].start = v
	}

	switch {
	case addPrev && addNext:
		// Merge the previous and next ranges
		*r = slices.Replace(s, i-1, i+1, indexRange{
			start: s[i-1].start,
			end:   s[i].end,
		})

	case !addPrev && !addNext:
		// Insert (v, v) at I
		*r = slices.Insert(s, i, indexRange{start: v, end: v})
	}
	return true
}

func (r rangeSet) Contains(v uint64) bool {
	_, ok := r.search(v)
	return ok
}

// search returns the index of the range containing the value or the position
// where such a range would exist.
//
//   - If the value is contained within a range, search returns the index of that
//     element.
//   - If the value is less than the start of the first range, search returns 0.
//   - If the value is greater than the end of the last range, search returns
//     len(r).
//   - If the value is between two neighboring ranges A and B, search returns the
//     index of B.
func (r rangeSet) search(v uint64) (int, bool) {
	return slices.BinarySearchFunc(r, v, func(r indexRange, v uint64) int {
		switch {
		case r.end < v:
			return -1
		case r.start > v:
			return +1
		default:
			return 0
		}
	})
}

func (r rangeSet) Copy() rangeSet {
	s := make(rangeSet, len(r))
	copy(s, r)
	return s
}

func (r rangeSet) CopyAsInterface() any {
	return r.Copy()
}

func (r rangeSet) MarshalBinary() ([]byte, error) {
	var b []byte
	for _, r := range r {
		b = binary.AppendUvarint(b, r.start)
		b = binary.AppendUvarint(b, r.end)
	}
	return b, nil
}

func (r *rangeSet) UnmarshalBinary(b []byte) error {
	rd := bytes.NewReader(b)
	return r.UnmarshalBinaryFrom(rd)
}

func (r *rangeSet) UnmarshalBinaryFrom(rd io.Reader) error {
	b := bufio.NewReader(rd)
	for {
		start, err := binary.ReadUvarint(b)
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, io.EOF):
			return nil
		default:
			return err
		}

		end, err := binary.ReadUvarint(b)
		if err != nil {
			return err
		}
		*r = append(*r, indexRange{start: start, end: end})
	}
}
