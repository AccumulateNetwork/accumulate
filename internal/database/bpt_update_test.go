// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestCommitWithoutUpdateBPT_LeavesTheRootWhereItWas pins the omission that
// made a join impossible (#4305).
//
// Batch.Commit commits the BPT *store* -- it never calls Account.putBpt -- so
// writing an account and committing leaves the state root exactly where it
// was. A node pulling state to match an anchored root can pull every account
// the network has and never move its own root by one bit.
//
// Nothing in the Batch API says so, and nothing stopped the pull from
// forgetting it: `PulledState.Pull` and `pullSpine` each ended with
// `batch.Commit()` and no `batch.UpdateBPT()`, while the e2e test that was
// meant to prove the pull called `UpdateBPT` by hand. This test is here so the
// next writer of a pulled account finds the rule stated rather than inferring
// it from a soak run.
func TestCommitWithoutUpdateBPT_LeavesTheRootWhereItWas(t *testing.T) {
	db := OpenInMemory(nil)
	db.SetObserver(NewDatabaseObserver())

	root := func() [32]byte {
		b := db.Begin(false)
		defer b.Discard()
		h, err := b.GetBptRootHash()
		require.NoError(t, err)
		return h
	}

	before := root()

	// Write an account and commit, WITHOUT UpdateBPT.
	batch := db.Begin(true)
	u := protocol.AccountUrl("alice", "tokens")
	require.NoError(t, batch.Account(u).Main().Put(&protocol.TokenAccount{
		Url:      u,
		TokenUrl: protocol.AcmeUrl(),
	}))
	require.NoError(t, batch.Commit())

	require.Equal(t, before, root(),
		"committing a written account without UpdateBPT moved the state root; "+
			"if this ever becomes true, the pull's UpdateBPT calls are no longer load-bearing")

	// The same write with UpdateBPT does move it. This half is what makes the
	// first half a statement about UpdateBPT and not about the write.
	batch = db.Begin(true)
	require.NoError(t, batch.Account(u).Main().Put(&protocol.TokenAccount{
		Url:      u,
		TokenUrl: protocol.AcmeUrl(),
	}))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	require.NotEqual(t, before, root(), "UpdateBPT then Commit did not move the state root")
}
