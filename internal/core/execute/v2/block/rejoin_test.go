// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A rejoining node holds the anchors its peers hold: one entry per number
// above the anchor ledger's Delivered, the sequenced message itself, so the
// drain runs it once the signatures recorded in the store make its quorum
// (#4290, run 20260918T015356Z: the restarted node's first Directory block
// executed no anchor where its peers executed three).
func TestCollect_HoldsAnchorsAboveDelivered(t *testing.T) {
	f := newStagingFixture(t, 0)
	dn := protocol.DnUrl()
	anchor := func(n uint64) *messaging.BlockAnchor {
		txn := new(protocol.Transaction)
		txn.Header.Principal = protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
		txn.Body = &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: dn, MinorBlockIndex: n}}
		seq := &messaging.SequencedMessage{Message: &messaging.TransactionMessage{Transaction: txn}, Source: dn, Destination: protocol.PartitionUrl("BVN0"), Number: n}
		return &messaging.BlockAnchor{Anchor: seq}
	}

	// Delivered 2: #2 is tossed, #3 and #4 are held, a second copy of #3 is
	// one entry
	var ledger protocol.AnchorLedger
	ledger.Url = protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	ledger.Partition(dn).Delivered = 2
	require.NoError(t, f.batch.Account(ledger.Url).Main().Put(&ledger))

	held, err := f.x.Collect(f.batch, []*messaging.Envelope{{Messages: []messaging.Message{anchor(2), anchor(3), anchor(4), anchor(3)}}})
	require.NoError(t, err)
	require.Equal(t, 2, held)

	tx := f.x.staging().Begin()
	defer tx.Discard()
	id := execute.StreamID{Ledger: ledger.Url, Source: dn}
	_, ok := tx.IDOf(id, 2)
	require.False(t, ok, "at or below Delivered is not held")
	h, ok := tx.IDOf(id, 3)
	require.True(t, ok)
	require.True(t, h.Collected)
	_, ok = tx.IDOf(id, 4)
	require.True(t, ok)
}
