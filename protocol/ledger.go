package protocol

var Received uint64
var Delivered uint64

// Add records a received or delivered synthetic transaction.
func (s *SubnetSyntheticLedger) Add(delivered bool, sequenceNumber uint64, hash [32]byte) (dirty bool) {
	// Update received
	if sequenceNumber > s.Received {
		s.Received, dirty = sequenceNumber, true
	}

	if delivered {
		// Update delivered and truncate pending
		if sequenceNumber > s.Delivered {
			s.Delivered, dirty = sequenceNumber, true
		}
		if len(s.Pending) > 0 {
			s.Pending, dirty = s.Pending[1:], true
		}
		return
	}

	// Grow pending if necessary
	if n := s.Received - s.Delivered - uint64(len(s.Pending)); n > 0 {
		s.Pending, dirty = append(s.Pending, make([][32]byte, n)...), true
	}

	// Insert the hash
	i := sequenceNumber - s.Delivered - 1
	if s.Pending[i] != hash {
		s.Pending[i], dirty = hash, true
	}

	return dirty
}

// Get gets the hash for a synthetic transaction.
func (s *SubnetSyntheticLedger) Get(sequenceNumber uint64) ([32]byte, bool) {
	if sequenceNumber <= s.Delivered || sequenceNumber > s.Received {
		return [32]byte{}, false
	}

	hash := s.Pending[sequenceNumber-s.Delivered-1]
	return hash, hash != [32]byte{}
}
