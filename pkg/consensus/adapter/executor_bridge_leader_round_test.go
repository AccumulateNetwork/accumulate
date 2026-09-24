// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// The leader round a block is produced with reaches the system ledger through
// the bridge, which is how a DAG-BFT node executes (#4362): the executor
// recording a round it is handed proves nothing if the bridge never hands it.
func TestBridge_TheLedgerRecordsTheLeaderRoundItWasProducedAt(t *testing.T) {
	liteKey := acctesting.GenerateKey("bridge", "leader-round")
	r := newRealBridge(t, 0, liteKey)

	r.index++
	r.time = r.time.Add(1e9)
	b := types.NewBatch([][]byte{marshalEnv(t, transfer(t, liteKey, 1, 1))})
	_, err := r.bridge.ProduceBlock(context.Background(), BlockParams{
		Index:       r.index,
		Time:        r.time,
		IsLeader:    true,
		LeaderRound: 91,
		Batches:     []*types.Batch{b},
	})
	require.NoError(t, err)

	var ledger *protocol.SystemLedger
	require.NoError(t, r.db.View(func(batch *database.Batch) error {
		return batch.Account(protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	}))
	require.Equal(t, r.index, ledger.Index, "the ledger is the block just produced")
	require.Equal(t, uint64(91), ledger.LeaderRound, "and records the round it was committed at")
}
