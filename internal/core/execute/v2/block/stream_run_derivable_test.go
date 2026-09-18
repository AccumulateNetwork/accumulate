// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// These tests establish what a reconstructed stage has to get EXACTLY right,
// rather than approximately right: the block a node executes is a function of
// its staging, so two nodes whose stages differ execute different blocks even
// when their chains are identical.
//
// This is the question Paul's proposal turns on. Rebuilding staging by
// pulling everything a source will serve gives a SUPERSET of what the peers
// hold, and a superset is as divergent as a subset.

// collectedPos is runPos with the entries held as COLLECTED — the state a
// synthetic is in when it arrived without a proof of its own — and with a
// validated hash recorded for each number in `proven`.
func collectedPos(t *testing.T, delivered uint64, hold []uint64, proven []uint64) *streamPosition {
	t.Helper()
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	s := stream{kind: streamSynthetic, ledger: protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic), source: protocol.PartitionUrl("BVN0")}
	st := execute.NewStaging().Begin()
	hashOf := func(n uint64) [32]byte { return [32]byte{byte(n), 0xAA} }
	for _, n := range hold {
		st.Hold(s.id(), n, &execute.Held{
			ID:        protocol.PartitionUrl("BVN0").WithTxID([32]byte{byte(n)}),
			Collected: true,
			Hash:      hashOf(n),
		})
	}
	if len(proven) > 0 {
		// A collection proof over the source's chain from its very
		// beginning: element i stands at number i+1 (Staging.Prove).
		list := &merkle.ReceiptList{MerkleState: &merkle.State{Count: 0}}
		for _, n := range proven {
			h := hashOf(n)
			list.Elements = append(list.Elements, h[:])
		}
		require.NoError(t, st.Prove(s.id(), list))
	}
	return &streamPosition{stream: s, delivered: delivered, batch: batch, staging: st}
}

// With nothing arriving and identical chain state, the run is exactly what
// staging holds. A node that reconstructed one entry too many runs one
// transaction its peers do not.
func TestBuildRun_TheRunIsAFunctionOfStagingAlone(t *testing.T) {
	lean, _ := buildRun(runPos(t, 0, 1, 2, 3), arr(), noLimit)
	fat, _ := buildRun(runPos(t, 0, 1, 2, 3, 4, 5), arr(), noLimit)

	assert.Equal(t, []uint64{1, 2, 3}, runNumbers(lean))
	assert.Equal(t, []uint64{1, 2, 3, 4, 5}, runNumbers(fat),
		"the same ledger, the same block, two stages: two different blocks execute")
	assert.NotEqual(t, runNumbers(lean), runNumbers(fat))
}

// And the validated list moves the run on its own. A collected entry is not
// runnable until a hash validated at its number matches it, so a node that
// pulled a fresh collection proof covering more numbers than its peers have
// validated executes further than they do — from identical chains.
func TestBuildRun_TheValidatedListMovesTheRun(t *testing.T) {
	hold := []uint64{1, 2, 3, 4, 5}

	none, _ := buildRun(collectedPos(t, 0, hold, nil), arr(), noLimit)
	assert.Empty(t, runNumbers(none), "collected and unvalidated: nothing runs")

	some, _ := buildRun(collectedPos(t, 0, hold, []uint64{1, 2, 3}), arr(), noLimit)
	assert.Equal(t, []uint64{1, 2, 3}, runNumbers(some))

	all, _ := buildRun(collectedPos(t, 0, hold, hold), arr(), noLimit)
	assert.Equal(t, []uint64{1, 2, 3, 4, 5}, runNumbers(all),
		"a proof covering more numbers runs more numbers — with no change on any chain")
}
