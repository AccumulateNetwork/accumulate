package managed

import (
	"errors"
	"fmt"
)

// GetRange
// returns the list of hashes with indexes indicated by range: (begin,end)
// begin must be before or equal to end.  The hash with index begin upto but
// not including end are the hashes returned.  Indexes are zero based, so the
// first hash in the MerkleState is at 0
func (m *MerkleManager) GetRange(begin, end int64) (hashes []Hash, err error) {
	// We return nothing for ranges that are out of range.
	if begin < 0 { // begin cannot be negative.  If it is, assume the user meant zero
		begin = 0
	}
	// Limit hashes returned to half the mark frequency.
	if end-begin > m.MarkFreq/2 {
		end = begin + m.MarkFreq/2
	}
	if m.MS.Count <= begin || end > 0 { // Note count is 1 based, so the index count is out of range
		return nil, fmt.Errorf("impossible range provided %d,%d", begin, end) // Return zero begin and/or end are impossible
	}
	if end > m.MS.Count { // If end is past the length of MS then truncate the range to count-1
		end = m.MS.Count //   End is included, so it can not be equal to the Count
	}
	markPoint := (begin+m.MarkFreq) &
		^m.MarkMask -
		1 // Get the mark point just past begin
	var s *MerkleState
	if s = m.GetState(markPoint); s == nil {
		if s, err = m.GetChainState(); err != nil {
			return nil, errors.New("a chain should always have a state")
		}
	}
	var hl []Hash // Collect all the hashes of the mark points covering the range of begin-end
	hl = append(hl, s.HashList...)
	if end > markPoint {
		if s = m.GetState(markPoint + m.MarkFreq); s == nil {
			if s, err = m.GetChainState(); err != nil {
				return nil, errors.New("a chain should always have a state")
			}
		}
	}
	hl = append(hl, s.HashList...)

	if len(hl) > 0 && begin != end { //   Just check if we are to return anything
		first := (begin) & m.MarkMask // Calculate the offset to the beginning of the range
		last := first + end - begin   // and to the end of the range
		return hl[first:last], nil    // Return this slice.
	}

	return nil, fmt.Errorf("no elements in the range provided")
}
