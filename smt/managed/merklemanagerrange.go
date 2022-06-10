package managed

import (
	"errors"
	"fmt"
)

// GetRange
// returns the list of hashes with indexes indicated by range: (begin,end)
// begin must be before or equal to end.  The hash with index begin upto
// but not including end are the hashes returned.  Indexes are zero based, so the
// first hash in the MerkleState is at 0
func (m *MerkleManager) GetRange(begin, end int64) (hashes []Hash, err error) {
	ec := m.GetElementCount()

	// end++  Increment to include end in results, comment out to leave it out.

	if end < begin || begin >= ec || begin < 0 {
		return nil, fmt.Errorf("impossible range %d,%d for chain length %d",
			begin, end, m.GetElementCount()) // Return zero begin and/or end are impossible
	}
	if end > ec { // Don't try and return more elements than are in the chain
		end = ec
	}
	if end == begin { // We will return an empty string if begin == end
		return hashes, nil
	}

	markPoint := begin & ^m.MarkMask // Get the mark point just past begin

	var s *MerkleState
	var hl []Hash // Collect all the hashes of the mark points covering the range of begin-end
	marks := (end-(begin&^m.MarkMask))/m.MarkFreq + 1
	for i := int64(0); i < marks; i++ {
		markPoint += m.MarkFreq
		if s = m.GetState(markPoint - 1); s != nil {
			hl = append(hl, s.HashList...)
		} else {
			s, err = m.GetChainState()
			if err != nil {
				return nil, errors.New("a chain should always have a chain state")
			}
			hl = append(hl, s.HashList...)
			break
		}
	}

	first := (begin) & m.MarkMask // Calculate the offset to the beginning of the range
	last := first + end - begin   // and to the end of the range

	// FIXME Is this supposed to be an error?
	// if int(last) > len(hl) {
	// 	fmt.Println("begin end", begin, " ", end)
	// }
	return hl[first:last], nil // Return this slice.

}
