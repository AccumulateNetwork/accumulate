// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Received is derived from staging on read (#4189). Every operator surface asks
// an account how far a stream has been sighted — `debug sequence` for its
// unreceived and unprocessed counts, the soak dashboard for a channel's backlog
// as received minus delivered. That backlog is the quantity that pinned at
// exactly 4,096 while the network livelocked, so a zero here does not read as
// "no data", it reads as "nothing ever arrived" and paints a healthy stream as
// stalled.
func TestWithSighted_FillsReceivedFromStaging(t *testing.T) {
	synthetic := protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic)
	source := protocol.PartitionUrl("BVN1")
	id := execute.StreamID{Ledger: synthetic, Source: source}

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	ledger := new(protocol.SyntheticLedger)
	ledger.Url = synthetic
	ledger.Partition(source).Delivered = 5
	require.NoError(t, batch.Account(synthetic).Main().Put(ledger))

	// Sighted 9: 6 through 9 arrived, 5 have executed. The backlog is 4.
	for _, n := range []uint64{7, 9} {
		require.NoError(t, execute.Hold(batch, id, n, source.WithTxID([32]byte{byte(n)})))
	}

	stored, err := batch.Account(synthetic).Main().Get()
	require.NoError(t, err)
	got := withSighted(batch, synthetic, stored).(*protocol.SyntheticLedger)

	part := got.Partition(source)
	require.Equal(t, uint64(9), part.Received, "sighted through 9")
	require.Equal(t, uint64(5), part.Delivered)
	require.Equal(t, uint64(4), part.Received-part.Delivered,
		"received minus delivered is the channel backlog every dashboard reads")

	// The account on disk carries no trace of it: a value derived from staging
	// must never reach hashed state, or a staging discrepancy becomes a
	// divergent block hash instead of a wrong number on a dashboard.
	reread, err := batch.Account(synthetic).Main().Get()
	require.NoError(t, err)
	require.Equal(t, uint64(0), reread.(*protocol.SyntheticLedger).Partition(source).Received,
		"withSighted must copy, not edit the batch's memoized record")
}

// A stream that has never been behind reports Received == Delivered, not zero.
// Zero would read as "nothing ever arrived" for a perfectly healthy channel.
func TestWithSighted_HealthyStreamReportsDelivered(t *testing.T) {
	synthetic := protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic)
	source := protocol.PartitionUrl("BVN1")

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	ledger := new(protocol.SyntheticLedger)
	ledger.Url = synthetic
	ledger.Partition(source).Delivered = 42
	require.NoError(t, batch.Account(synthetic).Main().Put(ledger))

	stored, err := batch.Account(synthetic).Main().Get()
	require.NoError(t, err)
	got := withSighted(batch, synthetic, stored).(*protocol.SyntheticLedger)

	part := got.Partition(source)
	require.Equal(t, uint64(42), part.Received, "nothing staged means nothing outstanding")
	require.Equal(t, uint64(0), part.Received-part.Delivered, "a backlog of zero, not a backlog of 42")
}

// Anchors are a separate stream on a different account and must be filled too —
// the dashboard's anchor flow matrix reads the same field.
func TestWithSighted_AnchorLedger(t *testing.T) {
	anchors := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	source := protocol.DnUrl()
	id := execute.StreamID{Ledger: anchors, Source: source}

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	ledger := new(protocol.AnchorLedger)
	ledger.Url = anchors
	ledger.Partition(source).Delivered = 3
	require.NoError(t, batch.Account(anchors).Main().Put(ledger))
	require.NoError(t, execute.Hold(batch, id, 6, source.WithTxID([32]byte{6})))

	stored, err := batch.Account(anchors).Main().Get()
	require.NoError(t, err)
	got := withSighted(batch, anchors, stored).(*protocol.AnchorLedger)

	require.Equal(t, uint64(6), got.Partition(source).Received)
}
