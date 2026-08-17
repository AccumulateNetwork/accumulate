// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestHealNeeded verifies the gate that keeps the healing scan from launching
// on a healthy node. For synthetic messages it fires only when a pending window
// has a nil (unknown) entry — known synthetic messages deliver on their own
// once their predecessors arrive. For anchors ANY pending entry fires it: a
// known anchor can be quorum-stuck forever after validator churn, and a
// proof-authorized resubmission recovers it immediately (#4056).
func TestHealNeeded(t *testing.T) {
	txid := protocol.PartitionUrl("BVN1").JoinPath(protocol.Synthetic).WithTxID([32]byte{1})

	synth := func(pending ...*url.TxID) *protocol.SyntheticLedger {
		return &protocol.SyntheticLedger{Sequence: []*protocol.PartitionSyntheticLedger{
			{Url: protocol.PartitionUrl("BVN1"), Pending: pending},
		}}
	}
	anchors := func(pending ...*url.TxID) *protocol.AnchorLedger {
		return &protocol.AnchorLedger{Sequence: []*protocol.PartitionSyntheticLedger{
			{Url: protocol.PartitionUrl("BVN1"), Pending: pending},
		}}
	}
	empty := &protocol.SyntheticLedger{}
	emptyAnchors := &protocol.AnchorLedger{}

	// Healthy: no pending at all.
	require.False(t, healNeeded(empty, emptyAnchors), "no pending must not need healing")

	// Healthy: known pending synthetic messages deliver on their own.
	require.False(t, healNeeded(synth(txid, txid), emptyAnchors), "all-known synthetic pending must not need healing")

	// A known pending anchor may be quorum-stuck — it needs the healing scan.
	require.True(t, healNeeded(empty, anchors(txid)), "a known pending anchor must need healing")

	// Gap: a nil entry means a later message arrived but this one never did.
	require.True(t, healNeeded(synth(nil), emptyAnchors), "a missing synthetic message must need healing")
	require.True(t, healNeeded(synth(nil, txid), emptyAnchors), "a missing message before a known one must need healing")
	require.True(t, healNeeded(empty, anchors(txid, nil)), "a missing anchor must need healing")
}
