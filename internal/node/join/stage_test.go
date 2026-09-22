// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The gap is read against the Delivered the PULLED state holds, not the
// one staging remembers: B+1 carries #105 of a stream whose pulled Delivered
// is 103, and nothing is held at 104 — an entry from before the node was
// listening, which its peers hold. Once the pulled state says 104 was
// delivered, the stream runs contiguously and there is no gap (executor
// spec, "Sync", step 4).
func TestExecutorStage_AGapIsReadAgainstThePulledDelivered(t *testing.T) {
	source := protocol.PartitionUrl("BVN0")
	ledgerUrl := protocol.PartitionUrl("BVN1").JoinPath(protocol.Synthetic)
	stream := execute.StreamID{Ledger: ledgerUrl, Source: source}

	staging := execute.NewStaging()
	tx := staging.Begin()
	seq := &messaging.SequencedMessage{Number: 105, Source: source, Destination: protocol.PartitionUrl("BVN1")}
	tx.Hold(stream, 105, &execute.Held{ID: seq.ID(), Message: seq, Hash: seq.Hash()})
	tx.Commit()

	db := database.OpenInMemory(nil)
	deliver := func(n uint64) {
		t.Helper()
		batch := db.Begin(true)
		defer batch.Discard()
		ledger := new(protocol.SyntheticLedger)
		ledger.Url = ledgerUrl
		ledger.Partition(source).Delivered = n
		require.NoError(t, batch.Account(ledgerUrl).Main().Put(ledger))
		require.NoError(t, batch.Commit())
	}

	stage := &ExecutorStage{Staging: staging, Database: db}

	deliver(103)
	gap, err := stage.HasGap(21)
	require.NoError(t, err)
	require.True(t, gap, "104 is missing between the pulled Delivered and what is held")

	deliver(104)
	gap, err = stage.HasGap(22)
	require.NoError(t, err)
	require.False(t, gap, "the pulled state says 104 executed, so 105 is next")
}
