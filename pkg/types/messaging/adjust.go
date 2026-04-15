// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// AdjustStatusIDs corrects for the fact that the block anchor's ID method was
// bad and has been changed.
func AdjustStatusIDs(messages []Message, st []*protocol.TransactionStatus) {
	adjustedIDs := map[[32]byte]*url.TxID{}
	for _, msg := range messages {
		switch msg := msg.(type) {
		case *BlockAnchor:
			adjustedIDs[msg.Hash()] = msg.OldID()
		}
	}
	for _, st := range st {
		if st.TxID == nil {
			continue
		}
		id, ok := adjustedIDs[st.TxID.Hash()]
		if ok {
			st.TxID = id
		}
	}
}
