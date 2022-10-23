// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package managed

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

// GetRange
// returns the list of hashes with indexes indicated by range: (begin,end)
// begin must be before or equal to end.  The hash with index begin upto
// but not including end are the hashes returned.  Indexes are zero based, so the
// first hash in the MerkleState is at 0
func (m *MerkleManager) GetRange(begin, end int64) ([]Hash, error) {
	head, err := m.Head().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load head: %w", err)
	}

	// Check bounds
	if begin < 0 {
		return nil, errors.Format(errors.StatusBadRequest, "begin is negative")
	}
	if end < begin {
		return nil, errors.Format(errors.StatusBadRequest, "begin is after end (%d > %d)", begin, end)
	}
	if begin >= head.Count {
		return nil, errors.Format(errors.StatusBadRequest, "begin is out of range (%d >= %d)", begin, head.Count)
	}

	// Don't return more entries than there are
	if end > head.Count {
		end = head.Count
	}

	// Nothing to return
	if begin == end {
		return nil, nil
	}

	var hashes []Hash                           // Collect hashes from mark points
	beginMark := begin&^m.markMask + m.markFreq // Mark point after begin
	endMark := (end-1)&^m.markMask + m.markFreq // Mark point after end
	lastMark := head.Count &^ m.markMask        // Last mark point
	for i := beginMark; i <= endMark && i <= lastMark; i += m.markFreq {
		s, err := m.States(uint64(i - 1)).Get()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.StatusNotFound):
			return nil, errors.FormatWithCause(errors.StatusNotFound, err, "markpoint %d not found", i)
		default:
			return nil, errors.Format(errors.StatusUnknownError, "load markpoint %d: %w", i, err)
		}
		if len(s.HashList) != int(m.markFreq) {
			return nil, errors.Format(errors.StatusIncompleteChain, "markpoint %d: expected %d entries, got %d", i, m.markFreq, len(s.HashList))
		}
		hashes = append(hashes, s.HashList...)
	}

	first := (begin) & m.markMask // Calculate the offset to the beginning of the range
	last := first + end - begin   // And to the end of the range
	if endMark <= lastMark {      // If end is before the last mark point, return the requested range
		return hashes[first:last], nil
	}

	expected := head.Count & m.markMask // Calculate the number of expected hashes in the current state
	if int64(len(head.HashList)) != expected {
		return nil, errors.Format(errors.StatusIncompleteChain, "head: expected %d entries, got %d", expected, len(head.HashList))
	}

	hashes = append(hashes, head.HashList...) // Append the current hash list
	return hashes[first:last], nil            // Return the requested range
}
