// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestKeepStatesWritesAnAnswerInOneCommitAndNoneOfARefusedAccount — #4461.
// An answer's accounts are written in one commit; one the check refuses, in
// the middle of the answer, is written not at all, and the others are written
// with the leaves the peer holds.
func TestKeepStatesWritesAnAnswerInOneCommitAndNoneOfARefusedAccount(t *testing.T) {
	peerDB := newObservedDB(t)
	var accounts []*url.URL
	b := peerDB.Begin(true)
	for i := 0; i < 3; i++ {
		u := protocol.AccountUrl(fmt.Sprintf("keep%d", i), "tokens")
		accounts = append(accounts, u)
		require.NoError(t, b.Account(u).Main().Put(&protocol.TokenAccount{Url: u, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(int64(i + 1))}))
		require.NoError(t, b.Account(u).MainChain().Inner().AddEntry([]byte{byte(i), 31: 1}, false))
	}
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())

	q := v3impl.NewQuerier(v3impl.QuerierParams{NodeState: nodestate.Always{}, Database: peerDB, Partition: "BVN0"})
	rec, err := api.Querier2{Querier: q}.QueryAccountStates(context.Background(), protocol.PartitionUrl("BVN0"), &api.AccountStatesQuery{Accounts: accounts})
	require.NoError(t, err)
	require.Len(t, rec.States, 3)

	// The middle account's open mark set does not rebuild its chain's head:
	// the check passes it (the leaf hashes the head, not the entries), and
	// the write refuses it after its body is already written.
	require.NotEmpty(t, rec.States[1].OpenEntries)
	rec.States[1].OpenEntries[0].Entries[0][0] ^= 1

	dst := newObservedDB(t)
	errs := KeepStates(dst, rec.States, nil)
	require.NoError(t, errs[0])
	require.Error(t, errs[1], "a state whose open mark set does not rebuild its head was kept")
	require.NoError(t, errs[2])

	rb, db := peerDB.Begin(false), dst.Begin(false)
	defer rb.Discard()
	defer db.Discard()
	for _, i := range []int{0, 2} {
		want, err := rb.Account(accounts[i]).Hash()
		require.NoError(t, err)
		got, err := db.BPT().Get(db.Account(accounts[i]).Key())
		require.NoError(t, err, "%v was not written", accounts[i])
		require.Equal(t, want[:], got)
	}
	_, err = db.Account(accounts[1]).Main().Get()
	require.Error(t, err, "the refused account's body was written")
	_, err = db.BPT().Get(db.Account(accounts[1]).Key())
	require.Error(t, err, "the refused account has a leaf")
}

// TestAnOlderStateIsNotWrittenOverANewerOne — #4518, executor spec "Sync" §3a,
// "Every account state the node holds is stamped" (Paul, 2026-10-05:
// "bookkeeping on account states as we load so we always can tell the most
// recently updated account states ... so we always know what we can
// overwrite"). A state kept as of block 10 is not overwritten by one served
// as of block 8, whoever serves it; one as of block 12 replaces it.
func TestAnOlderStateIsNotWrittenOverANewerOne(t *testing.T) {
	peerDB := newObservedDB(t)
	u := protocol.AccountUrl("stamped", "tokens")
	q := v3impl.NewQuerier(v3impl.QuerierParams{NodeState: nodestate.Always{}, Database: peerDB, Partition: "BVN0"})
	serve := func(balance int64) *api.AccountState {
		b := peerDB.Begin(true)
		require.NoError(t, b.Account(u).Main().Put(&protocol.TokenAccount{Url: u, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(balance)}))
		require.NoError(t, b.UpdateBPT())
		require.NoError(t, b.Commit())
		rec, err := api.Querier2{Querier: q}.QueryAccountStates(context.Background(), protocol.PartitionUrl("BVN0"), &api.AccountStatesQuery{Accounts: []*url.URL{u}})
		require.NoError(t, err)
		require.Len(t, rec.States, 1)
		return rec.States[0]
	}
	five, three := serve(5), serve(3)

	dst := newObservedDB(t)
	held := func() (int64, uint64) {
		b := dst.Begin(false)
		defer b.Discard()
		var acct *protocol.TokenAccount
		require.NoError(t, b.Account(u).Main().GetAs(&acct))
		at, err := b.Account(u).StateBlock().Get()
		require.NoError(t, err)
		return acct.Balance.Int64(), at
	}
	require.NoError(t, KeepStates(dst, []*api.AccountState{five}, []uint64{10})[0])
	bal, at := held()
	require.Equal(t, int64(5), bal)
	require.Equal(t, uint64(10), at, "the state was not stamped with the block it is of")

	// Older: dropped, and not an error -- the writer has nothing to do.
	require.NoError(t, KeepStates(dst, []*api.AccountState{three}, []uint64{8})[0])
	bal, at = held()
	require.Equal(t, int64(5), bal, "a state of block 8 overwrote the state of block 10")
	require.Equal(t, uint64(10), at)

	// Newer: written.
	require.NoError(t, KeepStates(dst, []*api.AccountState{three}, []uint64{12})[0])
	bal, at = held()
	require.Equal(t, int64(3), bal)
	require.Equal(t, uint64(12), at)

	// Of no block (a writer that does not know): written, unstamped checks skipped.
	require.NoError(t, KeepStates(dst, []*api.AccountState{five}, nil)[0])
	bal, _ = held()
	require.Equal(t, int64(5), bal)
}
