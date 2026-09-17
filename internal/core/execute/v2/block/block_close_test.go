// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A block that fails to close releases its batch. The caller holds an
// execute.Block, which has no Discard, so nothing else can: on run
// 20260915T042428Z a Directory block whose receipt failed to build left its
// batch open on every node for the life of the process, pinning a version of
// the store and staging every later commit's pre-images behind it (#4279).
func TestClose_ReleasesTheBatchWhenItFails(t *testing.T) {
	x := new(Executor)
	x.Describe = execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}
	x.globalsPtr.Store(&Globals{Active: core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionLatest, Network: &protocol.NetworkDefinition{Version: 1}}})
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	b := &Block{positions: new(positionCache), Executor: x, Batch: batch, staging: x.staging().Begin()}
	b.fatal = errors.New("a shard commit failed")

	_, err := b.Close()
	require.Error(t, err)

	// A discarded batch refuses further use; an open one would commit.
	require.Panics(t, func() { _ = batch.Commit() }, "the batch must be discarded when the block fails to close")
}

// A block whose COMMIT fails releases the batch too. Close was fixed first;
// commit has two error returns of its own, and the caller holds an
// execute.BlockState, which offers no Discard either. The publish below runs
// before the batch is touched at all, so nothing else would ever release it
// (#4279 review).
func TestCommit_ReleasesTheBatchWhenThePublishFails(t *testing.T) {
	x := new(Executor)
	x.Describe = execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}
	x.globalsPtr.Store(&Globals{Active: core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionLatest, Network: &protocol.NetworkDefinition{Version: 1}}})
	x.EventBus = events.NewBus(logging.OptionalLogger{})
	refused := errors.New("a subscriber refused the block")
	events.SubscribeSync(x.EventBus, func(execute.WillCommitBlock) error { return refused })

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	b := &Block{positions: new(positionCache), Executor: x, Batch: batch,
		staging: x.staging().Begin(), cache: x.synthCache().Begin(1)}
	// Not empty: an empty block discards and returns before publishing.
	b.State.Delivered = 1

	cb := &closedBlock{Block: *b}
	// Wrapped by the project's error type, so match the message.
	require.ErrorContains(t, cb.Commit(), refused.Error())
	require.Panics(t, func() { _ = batch.Commit() },
		"the batch must be discarded when the commit fails")
}
