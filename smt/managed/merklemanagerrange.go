package managed

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

// GetRange
// returns the list of hashes with indexes indicated by range: (begin,end)
// begin must be before or equal to end.  The hash with index begin upto
// but not including end are the hashes returned.  Indexes are zero based, so the
// first hash in the MerkleState is at 0
func (m *MerkleManager) GetRange(begin, end int64) (hashes []Hash, err error) {
	head, err := m.Head().Get() // Get the current state
	if err != nil {
		return nil, err
	}
	ec := head.Count

	// end++  Increment to include end in results, comment out to leave it out.

	if end < begin || begin >= ec || begin < 0 {
		return nil, fmt.Errorf("impossible range %d,%d for chain length %d",
			begin, end, head.Count) // Return zero begin and/or end are impossible
	}
	if end > ec { // Don't try and return more elements than are in the chain
		end = ec
	}
	if end == begin { // We will return an empty string if begin == end
		return hashes, nil
	}

	markPoint := begin & ^m.markMask // Get the mark point just past begin

	var s *MerkleState
	var hl []Hash // Collect all the hashes of the mark points covering the range of begin-end
	marks := (end-(begin&^m.markMask))/m.markFreq + 1
	for i := int64(0); i < marks; i++ {
		last := markPoint
		markPoint += m.markFreq
		if s = m.GetState(markPoint - 1); s != nil {
			if len(s.HashList) != int(m.markFreq) {
				return nil, errors.Format(errors.StatusIncompleteChain, "markpoint %d: expected %d entries, got %d", i, m.markFreq, len(s.HashList))
			}
			hl = append(hl, s.HashList...)
		} else {
			s, err = m.Head().Get()
			if err != nil {
				return nil, errors.New(errors.StatusInternalError, "a chain should always have a chain state")
			}
			c := s.Count - last
			if c%m.markFreq == 0 {
				c ^= m.markFreq //Swaps c==0 with c==m.markFreq
			}
			if len(s.HashList) != int(c) {
				return nil, errors.Format(errors.StatusIncompleteChain, "head: expected %d entries, got %d", s.Count-last, len(s.HashList))
			}

			hl = append(hl, s.HashList...)
			break
		}
	}

	first := (begin) & m.markMask // Calculate the offset to the beginning of the range
	last := first + end - begin   // and to the end of the range

	// FIXME Is this supposed to be an error?
	// if int(last) > len(hl) {
	// 	fmt.Println("begin end", begin, " ", end)
	// }
	return hl[first:last], nil // Return this slice.

}
