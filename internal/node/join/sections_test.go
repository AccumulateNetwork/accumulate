// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// forgedPage is a peer whose BPT pages carry one leaf other than its tree
// holds: the section it serves does not hash to the section's hash.
type forgedPage struct {
	pull.Source
	name string
}

func (f forgedPage) String() string { return f.name }

func (f forgedPage) QueryBptPage(ctx context.Context, scope *url.URL, q *api.BptPageQuery) (*api.BptPageRecord, error) {
	r, err := f.Source.QueryBptPage(ctx, scope, q)
	if err != nil {
		return nil, err
	}
	for _, e := range r.Entries {
		e.ValueHash[0] ^= 1
	}
	return r, nil
}

// forgedState is a peer that serves a body other than the account's, with a
// leaf hash it rebuilt to match: every part agrees with the leaf beside it,
// and the leaf is not the section's.
type forgedState struct {
	pull.Source
	name string
}

func (f forgedState) String() string { return f.name }

func (f forgedState) QueryAccountStates(ctx context.Context, scope *url.URL, q *api.AccountStatesQuery) (*api.AccountStatesRecord, error) {
	r, err := f.Source.QueryAccountStates(ctx, scope, q)
	if err != nil {
		return nil, err
	}
	for _, st := range r.States {
		if a, ok := st.Main.(*protocol.TokenAccount); ok {
			a = a.Copy()
			a.Balance = *big.NewInt(1000)
			st.Main = a
			h, err := pull.LeafHashOf(st)
			if err != nil {
				return nil, err
			}
			st.LeafHash = [32]byte(h)
		}
	}
	return r, nil
}

// sectionNotLoaded is a peer at block 10 holding alice, and a node holding
// the same tree with alice's section by its hash alone: its marker in place
// of alice's leaf. The node's root is the peer's.
func sectionNotLoaded(t *testing.T) (peer, node *database.Database, alice *url.URL) {
	here := protocol.PartitionUrl("BVN0")
	alice = protocol.AccountUrl("alice", "tokens")
	peer = peerStore(t)
	node = peerStore(t)
	for _, db := range []*database.Database{peer, node} {
		putLedger(t, db, here, 10)
		putTokens(t, db, alice, 5)
		putTokens(t, db, protocol.AccountUrl("bob", "tokens"), 6)
	}
	b := node.Begin(true)
	h, err := b.Account(alice).Hash()
	require.NoError(t, err)
	require.NoError(t, b.ForgetAccount(alice))
	require.NoError(t, b.BPT().Insert(database.SectionOfAccount(alice).MarkerKey(), h[:]))
	require.NoError(t, b.Commit())
	require.Equal(t, rootIn(t, peer), rootIn(t, node), "precondition: the marker does not stand for alice's section")
	return peer, node, alice
}

func rootIn(t *testing.T, db *database.Database) [32]byte {
	t.Helper()
	b := db.Ungated().Begin(false)
	defer b.Discard()
	r, err := b.GetBptRootHash()
	require.NoError(t, err)
	return r
}

// gateOver makes the node's sections the gate of its store, loading from
// srcs.
func gateOver(t *testing.T, node *database.Database, srcs ...pull.Source) *sections {
	x := newSections(protocol.PartitionUrl("BVN0"), "BVN0", node.Ungated(), slog.Default())
	x.peers = func(context.Context) []pull.Source { return srcs }
	x.timeout = 200 * time.Millisecond
	require.NoError(t, x.read())
	node.SetSectionGate(x)
	return x
}

// consistentForger is a peer whose own tree holds alice with another
// balance: its pages, its states and their leaves all agree, so every check of
// an account against its leaf passes, and only the section's hash -- against
// the marker the node holds from the signed top -- tells it from the truth
// (#4485 review, finding 2).
func consistentForger(t *testing.T, alice *url.URL) named {
	here := protocol.PartitionUrl("BVN0")
	liar := peerStore(t)
	putLedger(t, liar, here, 10)
	putTokens(t, liar, alice, 1000)
	putTokens(t, liar, protocol.AccountUrl("bob", "tokens"), 6)
	return named{Source: api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: liar, Partition: "BVN0"})}, name: "consistent forger"}
}

// TestAForgedSectionIsRefused — #4485, executor spec, "Sync", §3a: once the
// root has matched, a section is loaded as of the block the node last
// committed, its leaves must hash to the marker's value, and each account's
// state to its leaf, or the answer is refused and the next peer asked. A peer
// that serves another leaf, one that serves another body with a leaf rebuilt
// to match it, and one whose pages, states and leaves are all consistent with
// a tree that is not the network's are all refused; with no other peer the
// block is held, and nothing of what they served is written.
func TestAForgedSectionIsRefused(t *testing.T) {
	peer, node, alice := sectionNotLoaded(t)
	honest := named{Source: api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peer, Partition: "BVN0"})}, name: "honest"}
	pageForger := forgedPage{Source: honest.Source, name: "forged page"}
	stateForger := forgedState{Source: honest.Source, name: "forged state"}
	liar := consistentForger(t, alice)
	before := rootIn(t, node)

	for _, forger := range []pull.Source{pageForger, stateForger, liar} {
		x := gateOver(t, node, forger)
		require.True(t, x.NotLoaded(database.SectionOfAccount(alice)))

		b := node.BeginExecuting(true)
		_, err := b.Account(alice).Main().Get()
		require.Error(t, err, "%v: a forged section was loaded", forger)
		require.True(t, errors.Is(err, errors.NotReady), "%v: a section no peer served honestly read as %v", forger, err)
		require.NoError(t, b.UpdateBPT())
		require.Error(t, b.Commit(), "%v: a block that read a forged section committed", forger)
		b.Discard()
		require.True(t, x.NotLoaded(database.SectionOfAccount(alice)), "%v: the section counts as loaded", forger)
		require.Equal(t, before, rootIn(t, node))
		raw := node.Ungated().Begin(false)
		_, err = raw.Account(alice).Main().Get()
		raw.Discard()
		require.ErrorIs(t, err, errors.NotFound, "%v: what the forger served was written", forger)
	}

	// A forger, then an honest peer: the section is the honest peer's.
	x := gateOver(t, node, pageForger, stateForger, liar, honest)
	b := node.BeginExecuting(true)
	defer b.Discard()
	var acct *protocol.TokenAccount
	require.NoError(t, b.Account(alice).Main().GetAs(&acct))
	require.Equal(t, int64(5), acct.Balance.Int64(), "the section was taken from a forger")
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
	require.False(t, x.NotLoaded(database.SectionOfAccount(alice)))
	require.Equal(t, before, rootIn(t, node), "loading the section moved the root")
}
