// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Entries
// returns the list of hashes with indexes indicated by range: (begin,end)
// begin must be before or equal to end.  The hash with index begin upto
// but not including end are the hashes returned.  Indexes are zero based, so the
// first hash in the State is at 0
func (m *Chain) Entries(begin, end int64) ([][]byte, error) {
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
	firstAvailableMark := int64(-1)             // Track the first available mark point
	for i := beginMark; i <= endMark && i <= lastMark; i += m.markFreq {
		s, err := m.States(uint64(i - 1)).Get()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			// Skip missing mark points in a partial/truncated chain
			continue
		default:
			return nil, errors.UnknownError.WithFormat("load markpoint %d: %w", i, err)
		}
		if len(s.HashList) != int(m.markFreq) {
			return nil, errors.IncompleteChain.WithFormat("markpoint %d: expected %d entries, got %d", i, m.markFreq, len(s.HashList))
		}
		if firstAvailableMark == -1 {
			firstAvailableMark = i
		}
		hashes = append(hashes, s.HashList...)
	}

	// Calculate offsets based on the first available mark point
	var first int64
	if firstAvailableMark == -1 {
		// No mark points found in the requested range
		// Check if the data might be in the head
		if beginMark > lastMark {
			// All requested data should be in head, will calculate offset after appending head
			first = -1 // Marker to recalculate after appending head
		} else {
			// Requested range is entirely before available data
			return nil, errors.NotFound.WithFormat("no data available in range [%d:%d]", begin, end)
		}
	} else if firstAvailableMark <= beginMark {
		// Normal case: data starts at or before our begin mark
		first = (begin) & m.markMask
	} else {
		// Truncated case: first available data is after our begin mark
		// Check if the requested range starts before available data
		firstAvailableEntry := firstAvailableMark - m.markFreq
		if begin < firstAvailableEntry {
			return nil, errors.NotFound.WithFormat("entry %d not available (earliest available: %d)", begin, firstAvailableEntry)
		}
		// Adjust offset for the truncated beginning
		first = begin - firstAvailableEntry
	}

	if endMark <= lastMark { // If end is before the last mark point, return the requested range
		if len(hashes) == 0 {
			// No mark points were loaded, data must be unavailable
			return nil, errors.NotFound.WithFormat("no data available in range [%d:%d]", begin, end)
		}
		last := first + end - begin
		return hashes[first:last], nil
	}

	// End extends into head, append head.HashList

	expected := head.Count & m.markMask // Calculate the number of expected hashes in the current state
	if int64(len(head.HashList)) != expected {
		return nil, errors.IncompleteChain.WithFormat("head: expected %d entries, got %d", expected, len(head.HashList))
	}

	hashes = append(hashes, head.HashList...) // Append the current hash list

	// If first=-1, the range is entirely in head, recalculate the offset
	if first == -1 {
		first = begin - lastMark
	}

	last := first + end - begin
	return hashes[first:last], nil // Return the requested range
}
