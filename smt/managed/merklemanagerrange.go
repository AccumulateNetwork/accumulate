package managed

// GetRange
// returns the list of hashes with indexes indicated by range: (begin,end)
// begin must be before or equal to end.  The hash with index begin and end
// are included in the hashes returned.  Indexes are zero based, so the
// first hash in the MerkleState is at 0
func (m *MerkleManager) GetRange(ChainID []byte, begin, end int64) (hashes []Hash) {
	// We return nothing for ranges that are out of range.
	if begin < 0 { // begin cannot be negative.  If it is, assume the user meant zero
		begin = 0
	}
	// Limit hashes returned to half the mark frequency.
	if end-begin > m.MarkFreq/2 {
		end = begin + m.MarkFreq/2
	}
	if m.MS.Count <= begin || begin > end { // Note count is 1 based, so the index count is out of range
		return nil // Return zero if begin and end do not intersect the indexes of the merkle state
	}
	if end >= m.MS.Count { // If end is past the length of MS then truncate the range to count-1
		end = m.MS.Count - 1 // Remember that end is zero based, count is 1 based
	}
	markPoint := m.MarkFreq - 1
	if begin > m.MarkFreq-1 {
		markPoint = begin&(^m.MarkMask) + m.MarkFreq - 1
	}
	s := m.GetState(markPoint)
	var hl []Hash
	if s == nil {
		hl = append(hl, m.ReadChainHead(ChainID).HashList...)
	} else {
		hl = append(hl, s.HashList...)
	}
	if len(hl) < int(end-begin) {
		hl = append(hl, *m.GetNext(markPoint))
	}
	if end > markPoint+1 {
		s = m.GetState(markPoint + m.MarkFreq)
		if s == nil {
			hl = append(hl, m.ReadChainHead(ChainID).HashList...)
		}
	}
	copy(hashes, hl[begin&m.MarkMask:begin&m.MarkMask+end-begin])

	return hashes
}
