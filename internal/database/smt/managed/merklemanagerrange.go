// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package managed

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// GetRange
// returns the list of hashes with indexes indicated by range: (begin,end)
// begin must be before or equal to end.  The hash with index begin upto
// but not including end are the hashes returned.  Indexes are zero based, so the
// first hash in the MerkleState is at 0
func (m *MerkleManager) GetRange(begin, end int64) ([][]byte, error) {
	head, err := m.Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load head: %w", err)
	}

	// Check bounds
	if begin < 0 {
		return nil, errors.BadRequest.WithFormat("begin is negative")
	}
	if end < begin {
		return nil, errors.BadRequest.WithFormat("begin is after end (%d > %d)", begin, end)
	}
	if begin >= head.Count {
		return nil, errors.BadRequest.WithFormat("begin is out of range (%d >= %d)", begin, head.Count)
	}

	// Don't return more entries than there are
	if end > head.Count {
		end = head.Count
	}

	// Nothing to return
	if begin == end {
		return nil, nil
	}

	var hashes [][]byte                         // Collect hashes from mark points
	beginMark := begin&^m.markMask + m.markFreq // Mark point after begin
	endMark := (end-1)&^m.markMask + m.markFreq // Mark point after end
	lastMark := head.Count &^ m.markMask        // Last mark point
	for i := beginMark; i <= endMark && i <= lastMark; i += m.markFreq {
		s, err := m.States(uint64(i - 1)).Get()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			return nil, errors.NotFound.WithCauseAndFormat(err, "markpoint %d not found", i)
		default:
			return nil, errors.UnknownError.WithFormat("load markpoint %d: %w", i, err)
		}
		if len(s.HashList) != int(m.markFreq) {
			return nil, errors.IncompleteChain.WithFormat("markpoint %d: expected %d entries, got %d", i, m.markFreq, len(s.HashList))
		}
		for _, h := range s.HashList {
			hashes = append(hashes, h)
		}
	}

	first := (begin) & m.markMask // Calculate the offset to the beginning of the range
	last := first + end - begin   // And to the end of the range
	if endMark <= lastMark {      // If end is before the last mark point, return the requested range
		return hashes[first:last], nil
	}

	expected := head.Count & m.markMask // Calculate the number of expected hashes in the current state
	if int64(len(head.HashList)) != expected {
		return nil, errors.IncompleteChain.WithFormat("head: expected %d entries, got %d", expected, len(head.HashList))
	}

	for _, h := range head.HashList {
		hashes = append(hashes, h) // Append the current hash list
	}
	return hashes[first:last], nil // Return the requested range
}
