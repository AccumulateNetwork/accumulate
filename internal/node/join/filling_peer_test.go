// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// namedPeer is a peer reached by name: the production querier over its
// store, its account answers stamped with its block.
type namedPeer struct {
	servedAt
	name string
}

func (p namedPeer) String() string { return p.name }

// somePeers is the partition's peers, in the order given.
type somePeers struct {
	noValidators
	partition *url.URL
	srcs      []pull.Source
}

func (p *somePeers) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return p.srcs, p.partition, nil
}

func (p *somePeers) Querier(*url.URL) api.Querier { return nil }

// markAccountSection makes db hold the section u is in by its hash alone, as
// a node that joined and has not loaded it does: the section's leaves
// forgotten and its marker in their place. The root does not move.
func markAccountSection(t *testing.T, db *database.Database, u *url.URL) {
	t.Helper()
	sec := database.SectionOfAccount(u)
	before := rootIn(t, db)
	b := db.Ungated().Begin(true)
	defer b.Discard()
	leaves, err := leavesOf(b.BPT(), sec)
	require.NoError(t, err)
	var list []bpt.SectionLeaf
	for k, l := range leaves {
		list = append(list, bpt.SectionLeaf{KeyHash: k, Hash: l.hash})
		require.NotNil(t, l.url)
		require.NoError(t, b.ForgetAccount(l.url))
	}
	h, ok := bpt.SectionHash(list)
	require.True(t, ok)
	require.NoError(t, b.BPT().Insert(sec.MarkerKey(), h[:]))
	require.NoError(t, b.Commit())
	require.Equal(t, before, rootIn(t, db), "precondition: the marker moved the root")
}

// putRecord writes block n's block-ledger record, naming u, on the peer.
func putRecord(t *testing.T, db *database.Database, partition *url.URL, n uint64, u *url.URL) {
	t.Helper()
	b := db.Begin(true)
	defer b.Discard()
	ledger := b.Account(partition.JoinPath(protocol.Ledger))
	require.NoError(t, ledger.BlockLedger(n).Put(&database.BlockLedger{Index: n, Entries: []*protocol.BlockEntry{{Account: u, Chain: "main", Index: n}}}))
	require.NoError(t, b.Commit())
}

// holds reports whether db holds a leaf for u.
func holds(t *testing.T, db *database.Database, u *url.URL) bool {
	t.Helper()
	b := db.Ungated().Begin(false)
	defer b.Discard()
	_, err := b.BPT().Get(b.Account(u).Key())
	if errors.Is(err, errors.NotFound) {
		return false
	}
	require.NoError(t, err)
	return true
}

// TestAFillingPeersAnswerIsNoVerdict — #4496, executor spec, "Sync", §3a:
// "an account absent from a section not loaded is not yet known ... Only
// execution decides that an account does not exist", and "an AccountStates
// answer serves it with no body, so the asker asks the next peer". A peer
// still filling holds alice's section by its hash alone. The join's real
// rounds (PulledState.Pull) ask it, through the production querier, for the
// records that name alice (ChangedAccountStates) and then for alice herself
// (AccountStates): it must not be taken as saying alice does not exist.
//
// Before the fix the querier read the tree first, met the marker, and served
// NoLeaf; takeStates forgot alice on that one answer, and the loaded peer
// behind it was never asked.
func TestAFillingPeersAnswerIsNoVerdict(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")

	// Two peers holding the same state at block 10, the joiner taking the
	// top of the tree from them, and then block 11, naming alice, which
	// the joiner takes: it holds alice at 6.
	setup := func(t *testing.T) (loaded, filling *database.Database, counting *countingQuerier, joiner *database.Database, s *PulledState, peers *somePeers) {
		loaded, filling = peerStore(t), peerStore(t)
		for _, db := range []*database.Database{loaded, filling} {
			putLedger(t, db, here, 10)
			putTokens(t, db, alice, 5)
			putTokens(t, db, protocol.AccountUrl("bob", "tokens"), 9)
		}
		counting = &countingQuerier{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: filling, Partition: "BVN0"}), calls: map[string]int{}}
		peers = &somePeers{partition: here, srcs: []pull.Source{
			namedPeer{servedAt{Source: api.Querier2{Querier: counting}, block: 10}, "filling"},
			namedPeer{servedAt{Source: api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: loaded, Partition: "BVN0"})}, block: 10}, "loaded"},
		}}
		joiner = database.OpenInMemory(nil)
		joiner.SetObserver(database.NewDatabaseObserver())
		t.Cleanup(func() { _ = joiner.Close() })
		values, _ := genesisValues(t, 4)
		putNetwork(t, joiner, here, values)
		var err error
		s, err = NewState(StateOptions{Partition: here, Database: joiner, Sources: peers, ExecutedBlock: 1})
		require.NoError(t, err)
		require.NoError(t, s.Pull(context.Background()))

		for _, db := range []*database.Database{loaded, filling} {
			putTokens(t, db, alice, 6)
			putRecord(t, db, here, 11, alice)
			putLedger(t, db, here, 11)
		}
		require.NoError(t, s.Pull(context.Background()))
		require.Equal(t, int64(6), balanceIn(t, joiner, alice), "precondition: the joiner took alice from block 11's record")

		// Block 12 changes alice; then the filling peer holds her section
		// by its hash alone.
		for _, db := range []*database.Database{loaded, filling} {
			putTokens(t, db, alice, 7)
			putRecord(t, db, here, 12, alice)
			putLedger(t, db, here, 12)
		}
		markAccountSection(t, filling, alice)
		counting.calls = map[string]int{}
		return
	}

	t.Run("the next peer is asked", func(t *testing.T) {
		_, _, counting, joiner, s, _ := setup(t)
		require.NoError(t, s.Pull(context.Background()))
		require.Positive(t, counting.calls["*api.ChangedAccountStatesQuery"], "precondition: the filling peer was asked for the records")
		require.True(t, holds(t, joiner, alice), "a filling peer's answer for a section it has not loaded deleted alice")
		require.Equal(t, int64(7), balanceIn(t, joiner, alice), "alice was not taken from the loaded peer")
	})

	t.Run("no peer but the filling one: nothing is deleted, alice is owed", func(t *testing.T) {
		_, _, counting, joiner, s, peers := setup(t)
		peers.srcs = peers.srcs[:1]
		require.NoError(t, s.Pull(context.Background()))
		require.Positive(t, counting.calls["*api.ChangedAccountStatesQuery"], "precondition: the filling peer was asked for the records")
		require.True(t, holds(t, joiner, alice), "a filling peer's answer for a section it has not loaded deleted alice")
		require.Equal(t, int64(6), balanceIn(t, joiner, alice))
		require.Positive(t, counting.calls["*api.AccountStatesQuery"], "precondition: the filling peer was asked for alice by the batch call too")
		require.Contains(t, s.sync.retry, accountKey(alice), "alice is owed, to be asked again")
	})

	// The rule the fix must not break: a loaded peer whose tree holds no
	// leaf says the account does not exist at its block, and the joiner
	// forgets the copy it holds.
	t.Run("a loaded peer with no leaf still forgets", func(t *testing.T) {
		loaded, filling, _, joiner, s, peers := setup(t)
		_ = filling
		peers.srcs = peers.srcs[1:]
		gone := protocol.AccountUrl("gone", "tokens")
		putTokens(t, joiner, gone, 1)
		require.True(t, holds(t, joiner, gone))
		putRecord(t, loaded, here, 13, gone)
		putLedger(t, loaded, here, 13)
		require.NoError(t, s.Pull(context.Background()))
		require.False(t, holds(t, joiner, gone), "a loaded peer's no-leaf answer did not forget the account")
		require.Equal(t, int64(7), balanceIn(t, joiner, alice))
	})
}
