package managed

import "fmt"

// GetRange
// returns the list of hashes with indexes indicated by range: (begin,end)
// begin must be before or equal to end.  The hash with index begin and end
// are included in the hashes returned.  Indexes are zero based, so the
// first hash in the MerkleState is at 0
func (m *MerkleManager) GetRange(ChainID []byte, begin, end int64) (hashes []Hash, err error) {
	m.SetChainID(ChainID)
	// We return nothing for ranges that are out of range.
	if begin < 0 { // begin cannot be negative.  If it is, assume the user meant zero
		begin = 0
	}
	// Limit hashes returned to half the mark frequency.
	if end-begin > m.MarkFreq/2 {
		end = begin + m.MarkFreq/2
	}
	if m.MS.Count <= begin || 0 > end { // Note count is 1 based, so the index count is out of range
		return nil, fmt.Errorf("impossible range provided %d,%d", begin, end) // Return zero begin and/or end are impossible
	}
	if end >= m.MS.Count { // If end is past the length of MS then truncate the range to count-1
		end = m.MS.Count - 1 // Remember that end is zero based, count is 1 based
	}
	markPoint := m.MarkFreq - 1 // markPoint has the list of hashes just prior to the first mark.
	if begin > m.MarkFreq-1 {   // If begin is past the first mark, calculate it
		markPoint = (begin+1)&(^m.MarkMask) - 1
		markPoint += m.MarkFreq
	}
	s := m.GetState(markPoint) // Get the state of the mark right after the begin index
	var hl []Hash              // Collect all the hashes of the mark points covering the range of begin-end
	if s == nil {              // If no state found, then get the highest state of the chain
		head, err := m.ReadChainHead(ChainID) //
		if err != nil {                       // If we have an error, we will just ignore it.
			return nil, fmt.Errorf("No State found %x:  %v:err", ChainID, err)
		}
		hl = append(hl, head.HashList...)
	} else { // If a mark follows begin, then
		hl = append(hl, s.HashList...)
		s = m.GetState(markPoint + m.MarkFreq) // Get the next state
		if s != nil {
			hl = append(hl, s.HashList...)
		} else {
			head, err := m.ReadChainHead(ChainID)
			if err != nil { // If we have an error, we will just ignore it.
				return nil, fmt.Errorf("No State found %x:  %v:err", ChainID, err)
			}
			hl = append(hl, head.HashList...)
		}
	}
	if len(hl) > 0 && begin != end {
		first := (begin + 1) & m.MarkMask
		last := first + end - begin
		hashes = append(hashes, hl[first:last]...) // Cut out the hashes requested
	}
	return hashes, nil
}
