// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// capture records the `Stream position` lines a block writes.
type capture struct{ lines []map[string]any }

func (c *capture) Debug(string, ...interface{})       {}
func (c *capture) Error(string, ...interface{})       {}
func (c *capture) With(...interface{}) logging.Logger { return c }
func (c *capture) Info(msg string, kv ...interface{}) {
	if msg != "Stream position" {
		return
	}
	f := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			f[k] = kv[i+1]
		}
	}
	c.lines = append(c.lines, f)
}

func streamLogFixture(t *testing.T) (*Executor, *capture, execute.StreamID) {
	t.Helper()
	x := new(Executor)
	x.Describe = execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}
	x.globalsPtr.Store(&Globals{Active: core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionLatest, Network: &protocol.NetworkDefinition{Version: 1}}})
	c := new(capture)
	x.logger.Set(c)
	return x, c, x.synthStream(protocol.PartitionUrl("BVN1"))
}

func heldAt(n uint64) *execute.Held {
	seq := &messaging.SequencedMessage{Number: n, Source: protocol.PartitionUrl("BVN1"), Destination: protocol.PartitionUrl("BVN0")}
	return &execute.Held{ID: seq.ID(), Message: seq, Hash: seq.Hash()}
}

// blockAt runs one block's stream logging and returns how many lines it wrote.
func blockAt(x *Executor, index uint64, fn func(tx *execute.StagingTxn)) int {
	b := &Block{positions: new(positionCache), Executor: x, staging: x.staging().Begin()}
	b.Index = index
	if fn != nil {
		fn(b.staging)
		b.staging.Commit()
	}
	before := len(x.logger.L.(*capture).lines)
	b.logStreams()
	return len(x.logger.L.(*capture).lines) - before
}

// A stream in trouble logs whenever anything about it changes; a stream that
// is frozen logs on the cadence and no more. Without the cadence a stall
// wrote the same line every block for as long as it lasted (#4182, #4279).
func TestLogStreams_AStalledStreamLogsOnTheCadence(t *testing.T) {
	x, c, id := streamLogFixture(t)

	// Entries 1-3 arrive with a hole at 1: the stream is behind and waiting.
	require.Equal(t, 1, blockAt(x, 1, func(tx *execute.StagingTxn) {
		tx.Hold(id, 2, heldAt(2))
		tx.Hold(id, 3, heldAt(3))
	}), "a stream that just went behind is logged")
	require.EqualValues(t, 1, c.lines[0]["waiting"], "the hole below the held entries")
	require.EqualValues(t, 3, c.lines[0]["sighted"])
	require.EqualValues(t, 0, c.lines[0]["delivered"])

	// Nothing changes: silent until the cadence elapses.
	for i := uint64(2); i < StreamLogEvery; i++ {
		require.Zero(t, blockAt(x, i, nil), "block %d: nothing changed", i)
	}
	require.Equal(t, 1, blockAt(x, StreamLogEvery+1, nil), "the cadence still proves it is stalled")

	// A change is logged the block it happens, cadence or not.
	require.Equal(t, 1, blockAt(x, StreamLogEvery+2, func(tx *execute.StagingTxn) {
		tx.Hold(id, 4, heldAt(4))
	}), "sighted moved")
	require.EqualValues(t, 4, c.lines[len(c.lines)-1]["sighted"])
}

// A caught-up stream is not worth a line a block: it logs when it catches up
// and then only on the cadence.
func TestLogStreams_ACaughtUpStreamIsQuiet(t *testing.T) {
	x, c, id := streamLogFixture(t)

	blockAt(x, 1, func(tx *execute.StagingTxn) { tx.Hold(id, 1, heldAt(1)) })
	require.Len(t, c.lines, 1, "behind")

	// Delivering everything sighted catches the stream up — that is a change
	// and the block that does it is logged.
	require.Equal(t, 1, blockAt(x, 2, func(tx *execute.StagingTxn) { tx.Release(id, 1) }))
	last := c.lines[len(c.lines)-1]
	require.EqualValues(t, 1, last["delivered"])
	require.EqualValues(t, 0, last["waiting"], "no hole: caught up")

	// Ticking along, caught up: quiet until the cadence.
	for i := uint64(3); i < StreamLogEvery; i++ {
		require.Zero(t, blockAt(x, i, nil), "block %d", i)
	}
	require.Equal(t, 1, blockAt(x, StreamLogEvery+2, nil), "the cadence")
}

// A stream nothing has ever touched says nothing.
func TestLogStreams_SilentUntilThereIsSomethingToSay(t *testing.T) {
	x, _, _ := streamLogFixture(t)
	for i := uint64(1); i < StreamLogEvery+5; i++ {
		require.Zero(t, blockAt(x, i, nil), "block %d", i)
	}
}

// #4412 review F5: the line carries the ledger's Received beside staging's
// sighted. On a node that rejoined behind a hole its peers hold entries
// behind, the two disagree -- the ledger, pulled from the peers, says 12
// arrived; this node's staging has sighted nothing -- and that disagreement
// is the whole diagnosis. Such a stream is logged even though staging
// thinks it is caught up.
func TestLogStreams_ShowsTheLedgersReceivedBesideSighted(t *testing.T) {
	x, c, id := streamLogFixture(t)
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = id.Ledger
	ledger.Partition(id.Source).Delivered = 5
	ledger.Partition(id.Source).Received = 12
	require.NoError(t, batch.Account(id.Ledger).Main().Put(ledger))

	b := &Block{positions: new(positionCache), Executor: x, Batch: batch, staging: x.staging().Begin()}
	b.Index = 1
	b.staging.Release(id, 5)
	b.staging.Commit()
	b.logStreams()

	require.Len(t, c.lines, 1, "a stream whose ledger says more arrived than this node sighted is logged")
	require.EqualValues(t, 12, c.lines[0]["received"])
	require.EqualValues(t, 0, c.lines[0]["sighted"])
	require.EqualValues(t, 5, c.lines[0]["delivered"])
}

// #4412 review F7: logging reads the ledger and must not write it. A stream
// that exists only in this node's staging -- a joiner's collected blocks
// after the state it pulled, a source first seen since -- has no entry on the
// ledger, and a find-or-create read on the batch's memoized record would
// insert one into hashed state on this node alone, committed with the next
// write of that ledger.
func TestLogStreams_DoesNotWriteTheLedger(t *testing.T) {
	x, _, id := streamLogFixture(t)
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	other := protocol.PartitionUrl("BVN2")
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = id.Ledger
	ledger.Partition(other).Delivered = 5
	require.NoError(t, batch.Account(id.Ledger).Main().Put(ledger)) // dirty, as after flushStreams

	b := &Block{positions: new(positionCache), Executor: x, Batch: batch, staging: x.staging().Begin()}
	b.Index = 1
	b.staging.Hold(id, 3, heldAt(3)) // BVN1: staging only
	b.staging.Commit()
	b.logStreams()
	require.NoError(t, batch.Commit())

	batch = db.Begin(false)
	defer batch.Discard()
	var got *protocol.SyntheticLedger
	require.NoError(t, batch.Account(id.Ledger).Main().GetAs(&got))
	require.Len(t, got.Sequence, 1, "logging inserted a ledger entry for a stream only staging knows")
}
