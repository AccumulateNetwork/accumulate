// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import "gitlab.com/accumulatenetwork/accumulate/pkg/url"

// Add records a received or delivered synthetic transaction.
func (s *PartitionSyntheticLedger) Add(delivered bool, sequenceNumber uint64, txid *url.TxID) (dirty bool) {
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
		return dirty
	}

	// Grow pending if necessary
	if n := s.Received - s.Delivered - uint64(len(s.Pending)); n > 0 {
		s.Pending, dirty = append(s.Pending, make([]*url.TxID, n)...), true
	}

	if sequenceNumber <= s.Delivered {
		panic("already delivered")
	}

	// Insert the hash
	i := sequenceNumber - s.Delivered - 1
	if s.Pending[i] == nil {
		s.Pending[i], dirty = txid, true
	}

	return dirty
}

// Get gets the hash for a synthetic transaction.
func (s *PartitionSyntheticLedger) Get(sequenceNumber uint64) (*url.TxID, bool) {
	if sequenceNumber <= s.Delivered || sequenceNumber > s.Received {
		return nil, false
	}

	txid := s.Pending[sequenceNumber-s.Delivered-1]
	return txid, txid != nil
}
