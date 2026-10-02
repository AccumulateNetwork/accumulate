// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A section not loaded is refused or loaded, never not found (database.md;
// executor.md, "Sync", §3a; #4485). These tests drive the gate with one that
// loads from a second store, standing in for a peer.

// peerGate is a gate whose sections not loaded are loaded from a peer's
// store: the accounts written beneath the batch, the marker swapped for the
// leaves.
type peerGate struct {
	t         *testing.T
	peer      *Database
	notLoaded map[bpt.Section][32]byte
	loads     int
	committed []bpt.Section
	fail      bool
}

func (g *peerGate) NotLoaded(s bpt.Section) bool { _, ok := g.notLoaded[s]; return ok }
func (g *peerGate) AnyNotLoaded() bool           { return len(g.notLoaded) > 0 }

func (g *peerGate) Load(batch *Batch, s bpt.Section, _ bool) error {
	g.loads++
	if g.fail {
		return errors.NotFound.With("no peer answers")
	}
	src := g.peer.Begin(false)
	defer src.Discard()
	var urls []*url.URL
	err := src.ForEachAccount(func(a *Account, _ [32]byte) error {
		if SectionOfAccount(a.Url()) == s {
			urls = append(urls, a.Url())
		}
		return nil
	})
	require.NoError(g.t, err)
	err = batch.WriteBeneath(func(c *Batch) error {
		for _, u := range urls {
			v, err := src.Account(u).Main().Get()
			if err != nil {
				return err
			}
			if err := c.Account(u).Main().Put(v); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(g.t, err)
	require.NoError(g.t, batch.BPT().Delete(s.MarkerKey()))
	for _, u := range urls {
		h, err := src.Account(u).Hash()
		require.NoError(g.t, err)
		require.NoError(g.t, batch.BPT().Insert(src.Account(u).Key(), h[:]))
	}
	return nil
}

func (g *peerGate) Committed(s []bpt.Section) {
	g.committed = append(g.committed, s...)
	for _, s := range s {
		delete(g.notLoaded, s)
	}
}

func (g *peerGate) Opened(*Batch)           {}
func (g *peerGate) Discarded([]bpt.Section) {}

func putToken(t *testing.T, db *Database, u *url.URL, balance int64) {
	t.Helper()
	b := db.Begin(true)
	defer b.Discard()
	require.NoError(t, b.Account(u).Main().Put(&protocol.TokenAccount{Url: u, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(balance)}))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
}

// sectionsFixture is a peer holding alice and bob, and a node holding the
// peer's tree with alice's section replaced by its marker. bob is in another
// section, which the node holds loaded.
func sectionsFixture(t *testing.T) (node *Database, gate *peerGate, alice, bob, absent *url.URL) {
	alice = protocol.AccountUrl("alice", "tokens")
	bob = protocol.AccountUrl("bob", "tokens")
	require.NotEqual(t, SectionOfAccount(alice), SectionOfAccount(bob))

	// An account no store holds, in alice's section, and one in bob's.
	for i := 0; absent == nil; i++ {
		u := protocol.AccountUrl("nobody", "x"+big.NewInt(int64(i)).String())
		if SectionOfAccount(u) == SectionOfAccount(alice) {
			absent = u
		}
	}

	peer := OpenInMemory(nil)
	peer.SetObserver(NewDatabaseObserver())
	putToken(t, peer, alice, 5)
	putToken(t, peer, bob, 7)

	node = OpenInMemory(nil)
	node.SetObserver(NewDatabaseObserver())
	putToken(t, node, bob, 7)
	src := peer.Begin(false)
	h, err := src.Account(alice).Hash()
	src.Discard()
	require.NoError(t, err)
	b := node.Begin(true)
	require.NoError(t, b.BPT().Insert(SectionOfAccount(alice).MarkerKey(), h[:]))
	require.NoError(t, b.Commit())

	// The node's root is the peer's: the marker holds alice's section.
	pr, nr := rootOf(t, peer), rootOf(t, node)
	require.Equal(t, pr, nr, "precondition: the marker does not stand for alice's section")

	gate = &peerGate{t: t, peer: peer, notLoaded: map[bpt.Section][32]byte{SectionOfAccount(alice): h}}
	node.SetSectionGate(gate)
	return node, gate, alice, bob, absent
}

func rootOf(t *testing.T, db *Database) [32]byte {
	t.Helper()
	b := db.Ungated().Begin(false)
	defer b.Discard()
	r, err := b.GetBptRootHash()
	require.NoError(t, err)
	return r
}

func TestAReadOfASectionNotLoadedIsRefusedNotNotFound(t *testing.T) {
	node, gate, alice, bob, absent := sectionsFixture(t)

	b := node.Begin(false)
	defer b.Discard()
	_, err := b.Account(alice).Main().Get()
	require.Error(t, err)
	require.False(t, errors.Is(err, errors.NotFound), "an account in a section not loaded read as not found: %v", err)
	require.True(t, errors.Is(err, errors.NotReady), "an account in a section not loaded was not refused as not ready: %v", err)

	// Absent from a section not loaded is not yet known either.
	_, err = b.Account(absent).Main().Get()
	require.True(t, errors.Is(err, errors.NotReady), "an account absent from a section not loaded read as %v", err)

	// A loaded section answers, and absent from it is absent.
	_, err = b.Account(bob).Main().Get()
	require.NoError(t, err)
	_, err = b.Account(protocol.AccountUrl("bob", "nothing")).Main().Get()
	if SectionOfAccount(protocol.AccountUrl("bob", "nothing")) != SectionOfAccount(alice) {
		require.ErrorIs(t, err, errors.NotFound)
	}
	require.Zero(t, gate.loads, "a batch that does not execute loaded a section")

	// The join's own view passes.
	raw := node.Ungated().Begin(false)
	defer raw.Discard()
	_, err = raw.Account(alice).Main().Get()
	require.ErrorIs(t, err, errors.NotFound, "the join's view went through the gate")
}

func TestABlockLoadsTheSectionItReadsAndNamesNothingItLoaded(t *testing.T) {
	node, gate, alice, _, absent := sectionsFixture(t)
	before := rootOf(t, node)

	b := node.BeginExecuting(true)
	defer b.Discard()

	// Absent from a section not loaded: the section is loaded, and the
	// account looked for again, and is absent.
	_, err := b.Account(absent).Main().Get()
	require.ErrorIs(t, err, errors.NotFound, "an account absent from the section, read once it was loaded")
	require.Equal(t, 1, gate.loads)

	// What the section holds is there, and loading it once is enough.
	var acct *protocol.TokenAccount
	require.NoError(t, b.Account(alice).Main().GetAs(&acct))
	require.Equal(t, int64(5), acct.Balance.Int64())
	require.Equal(t, 1, gate.loads, "the section was loaded twice in one batch")

	// Nothing loaded is an account the block changed.
	for _, a := range b.UpdatedAccounts() {
		require.NotEqual(t, alice.String(), a.Url().String(), "a loaded account is among the accounts the block changed")
	}

	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
	require.Equal(t, []bpt.Section{SectionOfAccount(alice)}, gate.committed)
	require.Equal(t, before, rootOf(t, node), "loading a section moved the root")

	// The marker is gone and the account is the store's.
	r := node.Ungated().Begin(false)
	defer r.Discard()
	_, err = r.BPT().Get(SectionOfAccount(alice).MarkerKey())
	require.ErrorIs(t, err, errors.NotFound, "the marker outlived the load")
	require.NoError(t, r.Account(alice).Main().GetAs(&acct))
}

func TestABlockWhoseSectionCannotBeLoadedIsHeld(t *testing.T) {
	node, gate, alice, _, _ := sectionsFixture(t)
	gate.fail = true

	b := node.BeginExecuting(true)
	defer b.Discard()
	_, err := b.Account(alice).Main().Get()
	require.True(t, errors.Is(err, errors.NotReady), "a section that could not be loaded read as %v", err)
	require.NoError(t, b.UpdateBPT())
	err = b.Commit()
	require.Error(t, err, "a block that read a section it could not load committed")
	require.Empty(t, gate.committed)
	require.True(t, gate.NotLoaded(SectionOfAccount(alice)))
}

func TestAValidationLoadsWhatItReadsAndKeepsNothing(t *testing.T) {
	node, gate, alice, _, _ := sectionsFixture(t)

	b := node.BeginLoading(false)
	var acct *protocol.TokenAccount
	require.NoError(t, b.Account(alice).Main().GetAs(&acct), "a validation did not load the section it read")
	require.Equal(t, int64(5), acct.Balance.Int64())
	require.Equal(t, 1, gate.loads)
	b.Discard()

	require.Empty(t, gate.committed, "a validation's load counted as loaded")
	require.True(t, gate.NotLoaded(SectionOfAccount(alice)))
	r := node.Ungated().Begin(false)
	defer r.Discard()
	_, err := r.Account(alice).Main().Get()
	require.ErrorIs(t, err, errors.NotFound, "a validation's load was kept")
}

// countingGate is a gate with no section not loaded that counts how often a
// read asked it about one.
type countingGate struct{ asked atomic.Int64 }

func (g *countingGate) NotLoaded(bpt.Section) bool         { g.asked.Add(1); return false }
func (*countingGate) AnyNotLoaded() bool                   { return false }
func (*countingGate) Load(*Batch, bpt.Section, bool) error { return nil }
func (*countingGate) Committed([]bpt.Section)              {}
func (*countingGate) Opened(*Batch)                        {}
func (*countingGate) Discarded([]bpt.Section)              {}

// TestTheGateCostsNothingWhenNoSectionIsNotLoaded — #4485 review, finding 6.
// Every node past genesis runs with a gate for the life of its process, and
// almost all of that life no section is not loaded. Then a read of an account
// must not hash its key or ask the gate: the gate is a no-op.
func TestTheGateCostsNothingWhenNoSectionIsNotLoaded(t *testing.T) {
	db := OpenInMemory(nil)
	db.SetObserver(NewDatabaseObserver())
	alice := protocol.AccountUrl("alice", "tokens")
	putToken(t, db, alice, 5)
	gate := new(countingGate)
	db.SetSectionGate(gate)

	for _, b := range []*Batch{db.Begin(false), db.BeginExecuting(true), db.BeginLoading(false)} {
		var a *protocol.TokenAccount
		require.NoError(t, b.Account(alice).Main().GetAs(&a))
		_, err := b.Account(protocol.AccountUrl("nobody", "x")).Main().Get()
		require.ErrorIs(t, err, errors.NotFound)
		b.Discard()
	}
	require.Zero(t, gate.asked.Load(), "a read asked the gate about a section with no section not loaded")
}

// BenchmarkAnAccountReadWithNoSectionNotLoaded is what the gate costs a read
// on a node that has loaded every section, against a node with no gate.
func BenchmarkAnAccountReadWithNoSectionNotLoaded(b *testing.B) {
	alice := protocol.AccountUrl("alice", "tokens")
	for _, gated := range []bool{false, true} {
		name := "ungated"
		if gated {
			name = "gated"
		}
		b.Run(name, func(b *testing.B) {
			db := OpenInMemory(nil)
			db.SetObserver(NewDatabaseObserver())
			putToken(&testing.T{}, db, alice, 5)
			if gated {
				db.SetSectionGate(new(countingGate))
			}
			batch := db.BeginExecuting(false)
			defer batch.Discard()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// A fresh child batch, so the read reaches the store.
				c := batch.Begin(false)
				if _, err := c.Account(alice).Main().Get(); err != nil {
					b.Fatal(err)
				}
				c.Discard()
			}
		})
	}
}
