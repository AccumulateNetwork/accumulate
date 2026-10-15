// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A BVN that learns of several major blocks in one block — catching up after
// an outage — opens the newest, whatever order the notices merged in, and
// from v2-canaveral records every one of them, in order, on its major block
// chain, never one at or below the last it recorded (executor spec, "Major
// blocks on a BVN").
func TestMajorBlockNotices(t *testing.T) {
	at := func(i uint64) time.Time { return time.Unix(int64(1000*i), 0).UTC() }
	notice := func(i uint64) *chain.ProcessTransactionState {
		return &chain.ProcessTransactionState{MakeMajorBlock: i, MakeMajorBlockTime: at(i)}
	}

	newBlock := func(t *testing.T, version protocol.ExecutorVersion, recorded ...uint64) *Block {
		t.Helper()
		globals := new(Globals)
		globals.Active.ExecutorVersion = version
		x := new(Executor)
		x.globalsPtr.Store(globals)
		x.Describe.NetworkType = protocol.PartitionTypeBlockValidator
		x.Describe.PartitionId = "BVN0"

		batch := database.OpenInMemory(nil).Begin(true)
		t.Cleanup(batch.Discard)
		pool := batch.Account(x.Describe.AnchorPool())
		main, err := pool.MainChain().Get()
		require.NoError(t, err)
		require.NoError(t, main.AddEntry(make([]byte, 32), false))
		for _, i := range recorded {
			_, err := addIndexChainEntry(pool.MajorBlockChain(), &protocol.IndexEntry{BlockIndex: i, BlockTime: ptr(at(i))})
			require.NoError(t, err)
		}
		return &Block{Executor: x, Batch: batch}
	}

	recorded := func(t *testing.T, b *Block) []uint64 {
		t.Helper()
		c, err := b.Batch.Account(b.Executor.Describe.AnchorPool()).MajorBlockChain().Get()
		require.NoError(t, err)
		var got []uint64
		for i := int64(0); i < c.Height(); i++ {
			entry := new(protocol.IndexEntry)
			require.NoError(t, c.EntryAs(i, entry))
			got = append(got, entry.BlockIndex)
		}
		return got
	}

	t.Run("before v2-canaveral", func(t *testing.T) {
		b := newBlock(t, protocol.ExecutorVersionV2Kourou, 5)
		for _, i := range []uint64{7, 6} {
			b.mergeTransactionState(notice(i))
		}
		require.Equal(t, uint64(6), b.State.MakeMajorBlock, "the last notice merged wins")
		require.NoError(t, b.Executor.recordMajorBlock(b, 1))
		require.Equal(t, []uint64{5, 6}, recorded(t, b))
	})

	t.Run("several notices", func(t *testing.T) {
		b := newBlock(t, protocol.ExecutorVersionV2Canaveral, 5)
		for _, i := range []uint64{7, 6} {
			b.mergeTransactionState(notice(i))
		}
		require.Equal(t, uint64(7), b.State.MakeMajorBlock)
		require.Equal(t, at(7), b.State.MakeMajorBlockTime)
		require.NoError(t, b.Executor.recordMajorBlock(b, 1))
		require.Equal(t, []uint64{5, 6, 7}, recorded(t, b))
	})

	t.Run("a notice at or below the last recorded", func(t *testing.T) {
		b := newBlock(t, protocol.ExecutorVersionV2Canaveral, 5)
		for _, i := range []uint64{4, 5} {
			b.mergeTransactionState(notice(i))
		}
		require.NoError(t, b.Executor.recordMajorBlock(b, 1))
		require.Equal(t, []uint64{5}, recorded(t, b))
	})
}

func ptr[T any](v T) *T { return &v }
