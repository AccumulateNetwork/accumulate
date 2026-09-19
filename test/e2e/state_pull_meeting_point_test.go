// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// peerServing is a peer as the network sees one: the production querier of
// internal/api/v3 over that peer's store, behind the production Querier2
// client the pull uses. Nothing here stands in for a step production takes —
// the chain records, the entry pages and the account bodies are the ones a
// real peer would serve from the same store.
func peerServing(db database.Viewer) pull.Source {
	return api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{
		Database:  db,
		Partition: Directory,
	})}
}

// pullSpineFrom is what join.pullSpine does, against one source.
func pullSpineFrom(t *testing.T, src pull.Source, into *database.Database) error {
	t.Helper()
	batch := into.Begin(true)
	defer batch.Discard()
	for _, u := range pull.SpineAccounts(DnUrl()) {
		p, err := pull.Fetch(context.Background(), src, batch, u,
			pull.Options{Mode: pull.ModeFullSpine, Partition: DnUrl()}, false)
		if err != nil {
			return err
		}
		if err := p.Keep(); err != nil {
			return err
		}
	}
	if err := batch.UpdateBPT(); err != nil {
		return err
	}
	return batch.Commit()
}

// spineHeights reads the height of every chain of every spine account.
func spineHeights(t *testing.T, db *database.Database) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	View(t, db, func(b *database.Batch) {
		for _, u := range pull.SpineAccounts(DnUrl()) {
			chains, err := b.Account(u).Chains().Get()
			require.NoError(t, err)
			for _, cm := range chains {
				c, err := b.Account(u).ChainByName(cm.Name)
				require.NoError(t, err)
				head, err := c.Head().Get()
				require.NoError(t, err)
				out[u.String()+"#"+cm.Name] = head.Count
			}
		}
	})
	return out
}

// forkAt appends one entry that nothing in the network holds to the account's
// main chain, giving a peer whose history differs from everyone else's at the
// position that entry lands in.
func forkAt(t *testing.T, db *database.Database, u *url.URL) {
	t.Helper()
	b := db.Begin(true)
	defer b.Discard()
	junk := make([]byte, 32)
	junk[0] = 0xde
	junk[31] = 0xad
	require.NoError(t, b.Account(u).MainChain().Inner().AddEntry(junk, false))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
}

// TestPullSpine_MeetsAPeerThatIsBehind is the restart, through the wiring a
// restart uses: a node that executed to block Q asks a peer that is only at
// block P < Q for the spine.
//
// Syncing and bootstrapping are one walk, and the meeting point is where they
// differ: a bootstrapping node never reaches it and collects everything, a
// restarted node reaches it at once. The pull refused at exactly that point —
// "the local chain is at 1303 and the peer served 631; it cannot be re-pulled"
// — so every bootstrap test passed and a twelve-node restart could not pull a
// single spine account.
func TestPullSpine_MeetsAPeerThatIsBehind(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// A peer that stopped early, holding the Directory's real spine as of
	// the block it stopped at.
	sim.StepN(10)
	behind := emptyDb()
	require.NoError(t, pullSpineFrom(t, peerServing(sim.S.Database(Directory)), behind))
	behindAt := spineHeights(t, behind)

	// This node ran on, and holds the same chains with more on the end of
	// them. This is the state a restart comes back with.
	sim.StepN(25)
	local := emptyDb()
	require.NoError(t, pullSpineFrom(t, peerServing(sim.S.Database(Directory)), local))
	localAt := spineHeights(t, local)

	var ahead int
	for k, v := range localAt {
		require.GreaterOrEqual(t, v, behindAt[k], "%s went backwards", k)
		if v > behindAt[k] {
			ahead++
		}
	}
	require.NotZero(t, ahead, "no spine chain grew; the test proves nothing")
	t.Logf("%d of %d spine chains are ahead of the peer's", ahead, len(localAt))

	// The meeting point is the account's, not one chain's: the node's own
	// ledger body says which block it executed to, and a peer that is behind
	// must not rewind it.
	ledgerIndex := func(db *database.Database) uint64 {
		var n uint64
		View(t, db, func(b *database.Batch) {
			var l *SystemLedger
			require.NoError(t, b.Account(DnUrl().JoinPath(Ledger)).Main().GetAs(&l))
			n = l.Index
		})
		return n
	}
	mine, theirs := ledgerIndex(local), ledgerIndex(behind)
	require.Greater(t, mine, theirs, "the peer is not behind on the ledger; the test proves nothing")

	// The restart: ask the peer that is behind. Everything it can give for
	// these chains, this node already has.
	require.NoError(t, pullSpineFrom(t, peerServing(behind), local),
		"a peer that is simply behind was refused")

	require.Equal(t, localAt, spineHeights(t, local),
		"meeting a peer that is behind moved the node's own chains")
	require.Equal(t, mine, ledgerIndex(local),
		"meeting a peer that is behind rewound the node's own ledger to the peer's block")
}

// TestPullSpine_RefusesAPeerThatDisagrees — ahead is not by itself safe. A
// peer holding an entry the node does not hold, at a position the node also
// holds, is two nodes with different history, and that stays loud however far
// ahead the node is.
func TestPullSpine_RefusesAPeerThatDisagrees(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	sim.StepN(10)
	forked := emptyDb()
	require.NoError(t, pullSpineFrom(t, peerServing(sim.S.Database(Directory)), forked))
	// One entry nobody else has, at a position this node will hold too.
	forkAt(t, forked, DnUrl().JoinPath(AnchorPool))

	sim.StepN(25)
	local := emptyDb()
	require.NoError(t, pullSpineFrom(t, peerServing(sim.S.Database(Directory)), local))
	before := spineHeights(t, local)

	err := pullSpineFrom(t, peerServing(forked), local)
	require.Error(t, err, "a peer whose history differs from the node's was accepted")
	require.Contains(t, err.Error(), "disagree", "it was refused for the wrong reason: %v", err)
	require.Contains(t, err.Error(), "/anchors/main",
		"it was refused over some chain other than the forked one: %v", err)
	t.Logf("refused, as it must be: %v", err)
	require.Equal(t, before, spineHeights(t, local), "a refused pull left something behind")
}

// TestPullSpine_ThreeSourcesTwoOfThemForked is the shape the live network
// produced: three peers, two disagreeing with the node and one simply behind
// it. FetchFrom must find the one it can be reconciled with.
func TestPullSpine_ThreeSourcesTwoOfThemForked(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	sim.StepN(10)
	behind := emptyDb()
	require.NoError(t, pullSpineFrom(t, peerServing(sim.S.Database(Directory)), behind))
	forked1 := emptyDb()
	require.NoError(t, pullSpineFrom(t, peerServing(sim.S.Database(Directory)), forked1))
	forkAt(t, forked1, DnUrl().JoinPath(AnchorPool))
	forked2 := emptyDb()
	require.NoError(t, pullSpineFrom(t, peerServing(sim.S.Database(Directory)), forked2))
	forkAt(t, forked2, DnUrl().JoinPath(Ledger))

	sim.StepN(25)
	local := emptyDb()
	require.NoError(t, pullSpineFrom(t, peerServing(sim.S.Database(Directory)), local))
	before := spineHeights(t, local)

	srcs := []pull.Source{peerServing(forked1), peerServing(behind), peerServing(forked2)}

	batch := local.Begin(true)
	defer batch.Discard()
	for _, u := range pull.SpineAccounts(DnUrl()) {
		p, i, err := pull.FetchFrom(context.Background(), srcs, batch, u,
			pull.Options{Mode: pull.ModeFullSpine, Partition: DnUrl()})
		require.NoError(t, err, "no source served the spine account %v", u)
		require.NoError(t, p.Keep())
		t.Logf("%v came from source %d", u, i)
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	require.Equal(t, before, spineHeights(t, local),
		"the pull moved the node's own chains")
}
