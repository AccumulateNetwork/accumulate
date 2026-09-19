// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"strings"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The meeting point is the ACCOUNT's, not one chain's. These are the cases the
// account-level decision can be in, each stated as a property and each with a
// test, rather than left to a fall-through (#4348).

// putBody writes an account body without touching any chain.
func putBody(t *testing.T, db *database.Database, u *url.URL, tag string) {
	t.Helper()
	b := db.Begin(true)
	defer b.Discard()
	if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u, Entry: &protocol.DoubleHashDataEntry{
		Data: [][]byte{[]byte(tag)},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
}

// bodyTag reads back what putBody wrote, so a test can say whose body the
// account ended up with.
func bodyTag(t *testing.T, db *database.Database, u *url.URL) string {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	var a *protocol.DataAccount
	if err := b.Account(u).Main().GetAs(&a); err != nil {
		t.Fatal(err)
	}
	if a.Entry == nil || len(a.Entry.GetData()) == 0 {
		return ""
	}
	return string(a.Entry.GetData()[0])
}

// addEntries appends n entries to a named chain of an account, starting at
// index start, from the given salt. Two chains built with the same salt agree
// entry for entry.
func addEntries(t *testing.T, db *database.Database, u *url.URL, chain string, start, n int, salt byte) {
	t.Helper()
	b := db.Begin(true)
	defer b.Discard()
	c, err := b.Account(u).ChainByName(chain)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(); err != nil { // indexes the chain, as an executor's append does
		t.Fatal(err)
	}
	for i := start; i < start+n; i++ {
		if err := c.Inner().AddEntry(entryAt(i, salt), false); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
}

func chainHeight(t *testing.T, db *database.Database, u *url.URL, chain string) int64 {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	c, err := b.Account(u).ChainByName(chain)
	if err != nil {
		t.Fatal(err)
	}
	head, err := c.Inner().Head().Get()
	if err != nil {
		t.Fatal(err)
	}
	return head.Count
}

func directoryOf(t *testing.T, db *database.Database, u *url.URL) []string {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	dir, err := b.Account(u).Directory().Get()
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(dir))
	for _, x := range dir {
		out = append(out, x.String())
	}
	return out
}

func putDirectory(t *testing.T, db *database.Database, u *url.URL, names ...string) {
	t.Helper()
	b := db.Begin(true)
	defer b.Discard()
	var all []*url.URL
	for _, n := range names {
		all = append(all, u.JoinPath(n))
	}
	if err := b.Account(u).Directory().Put(all); err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
}

// TestAccountMeetingPoint_MixedIsRefused — F3. A source that is beyond the
// node on one chain and behind it on another cannot be reconciled with the
// node: whichever way the pull went it would leave a leaf that is neither
// side's — the node's extra entries on one chain and the peer's body and
// entries on the other.
//
// This used to fall through to "not past", so the peer's body was written over
// the node's own executed height while the chain the node was ahead on kept
// its extra entries. The prior reasoning was that no coherent NODE can be in
// that state, which is true and beside the point: the state is the PEER's
// claim, ModeFullSpine is pulled with no Verify and settled with Keep, and one
// dishonest spine source one entry ahead on any one chain could therefore
// rewind the node's ledger.
func TestAccountMeetingPoint_MixedIsRefused(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	// The node: 300 on main, 100 on scratch.
	local := newObservedDB(t)
	putBody(t, local, u, "the node's")
	addEntries(t, local, u, "main", 0, 300, 0x11)
	addEntries(t, local, u, "scratch", 0, 100, 0x22)

	// The peer: behind on main, ahead on scratch. Both are the node's own
	// entries as far as they go, so neither chain disagrees.
	peer := newObservedDB(t)
	putBody(t, peer, u, "the peer's")
	addEntries(t, peer, u, "main", 0, 200, 0x11)
	addEntries(t, peer, u, "scratch", 0, 150, 0x22)

	for _, mode := range []struct {
		name string
		mode Mode
	}{{"full spine", ModeFullSpine}, {"state only", ModeStateOnly}} {
		t.Run(mode.name, func(t *testing.T) {
			b := local.Begin(true)
			defer b.Discard()
			err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: mode.mode})
			if err == nil {
				t.Fatal("a source beyond the node on one chain and behind it on another was accepted")
			}
			t.Logf("refused, as it must be: %v", err)
			for _, want := range []string{"past this peer on", "behind it on", "main", "scratch"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal does not say %q: %v", want, err)
				}
			}
		})
	}

	// And nothing was written by either attempt.
	if got := bodyTag(t, local, u); got != "the node's" {
		t.Errorf("the account's body is %q; a refused pull wrote the peer's over it", got)
	}
	if h := chainHeight(t, local, u, "main"); h != 300 {
		t.Errorf("main is %d entries; it was 300", h)
	}
	if h := chainHeight(t, local, u, "scratch"); h != 100 {
		t.Errorf("scratch is %d entries; it was 100", h)
	}
}

// TestAccountMeetingPoint_EveryChainEqualTakesThePeersBody — F5, stated
// because it is an assumption and not a proof.
//
// When every chain of the account is at exactly the peer's height and agrees
// with it, the node is not PAST the peer — it is level with it — and the
// peer's body, directory and pending list are taken as they always were.
//
// What that assumes is that an account's body does not move without one of its
// chains moving. It holds for everything a block writes through the executor,
// which records every account it changes on a chain of that account, and it is
// what the block ledger and the state tree are built on. It is not enforced
// here, and for an unverified pull (the spine, Verify nil) it is not enforced
// anywhere: a source could serve a body of its own with chains that match.
// A verified pull cannot — the leaf would not be the one its receipt proves.
//
// Not reachable today for <partition>/ledger, whose root chain moves several
// entries per block. Reachable in principle for any account whose chains are
// still while its body changes; if one is ever found, this is the test that
// says what the pull does with it.
func TestAccountMeetingPoint_EveryChainEqualTakesThePeersBody(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	local := newObservedDB(t)
	putBody(t, local, u, "the node's")
	addEntries(t, local, u, "main", 0, 300, 0x11)

	peer := newObservedDB(t)
	putBody(t, peer, u, "the peer's")
	addEntries(t, peer, u, "main", 0, 300, 0x11) // the same chain, entry for entry

	b := local.Begin(true)
	defer b.Discard()
	if err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeFullSpine}); err != nil {
		t.Fatalf("a peer level with the node was refused: %v", err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := bodyTag(t, local, u); got != "the peer's" {
		t.Fatalf("the account's body is %q; level with the peer, the peer's body is taken", got)
	}
	if h := chainHeight(t, local, u, "main"); h != 300 {
		t.Fatalf("main is %d entries; it was 300", h)
	}
}

// TestAccountMeetingPoint_APeerThatServesNoChains — F6, both halves.
//
// A peer that serves an empty chain list is not by itself a broken peer: an
// account created without a transaction of its own has no chains at all
// (acc://dn.acme/ledger/1, and every account genesis writes directly). A node
// that holds nothing for such an account must take the peer's body, or it can
// never bootstrap one.
//
// A node that DOES hold chains for the account is a different matter: a peer
// that cannot name a chain the node holds entries of is a peer the node is
// past, and its body must not be installed over the node's. That case used to
// be indistinguishable from the first — the chain list was empty, nothing was
// compared, "past" was false and the body went in.
func TestAccountMeetingPoint_APeerThatServesNoChains(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger, "1")

	t.Run("the node holds nothing", func(t *testing.T) {
		peer := newObservedDB(t)
		putBody(t, peer, u, "the peer's")

		local := newObservedDB(t)
		b := local.Begin(true)
		defer b.Discard()
		if err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeStateOnly}); err != nil {
			t.Fatalf("an account with no chains could not be bootstrapped: %v", err)
		}
		if err := b.Commit(); err != nil {
			t.Fatal(err)
		}
		if got := bodyTag(t, local, u); got != "the peer's" {
			t.Fatalf("the account's body is %q; the node held nothing, so the peer's is taken", got)
		}
	})

	t.Run("the node holds chains", func(t *testing.T) {
		peer := newObservedDB(t)
		putBody(t, peer, u, "the peer's")

		local := newObservedDB(t)
		putBody(t, local, u, "the node's")
		addEntries(t, local, u, "main", 0, 40, 0x11)

		b := local.Begin(true)
		defer b.Discard()
		if err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeStateOnly}); err != nil {
			t.Fatalf("a peer with nothing to give was refused rather than passed over: %v", err)
		}
		if err := b.Commit(); err != nil {
			t.Fatal(err)
		}
		if got := bodyTag(t, local, u); got != "the node's" {
			t.Fatalf("the account's body is %q; a peer that serves no chain the node holds installed its body", got)
		}
		if h := chainHeight(t, local, u, "main"); h != 40 {
			t.Fatalf("main is %d entries; it was 40", h)
		}
	})
}

// TestAccountMeetingPoint_PastTakesNothingAtAll — the past case writes
// nothing: not the body, not the directory list, not the pending list, not one
// chain. That is F2 (the account's leaf is all four together, and keeping the
// body while taking the peer's directory builds a leaf neither side has) and
// F7 (a batch holding the node's own state cannot hash to the leaf the peer's
// receipt proves, so a past pull that carried one could never be verified).
func TestAccountMeetingPoint_PastTakesNothingAtAll(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	peer := newObservedDB(t)
	putBody(t, peer, u, "the peer's")
	addEntries(t, peer, u, "main", 0, 200, 0x11)
	putDirectory(t, peer, u, "one")

	local := newObservedDB(t)
	putBody(t, local, u, "the node's")
	addEntries(t, local, u, "main", 0, 300, 0x11)
	putDirectory(t, local, u, "one", "two")

	for _, mode := range []struct {
		name string
		mode Mode
	}{{"full spine", ModeFullSpine}, {"state only", ModeStateOnly}} {
		t.Run(mode.name, func(t *testing.T) {
			b := local.Begin(true)
			defer b.Discard()
			p, err := Fetch(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: mode.mode}, false)
			if err != nil {
				t.Fatalf("a peer that is simply behind was refused: %v", err)
			}
			if !p.Past() {
				t.Fatal("the pull did not report the node past the peer")
			}
			if err := p.Keep(); err != nil {
				t.Fatal(err)
			}
			if err := b.Commit(); err != nil {
				t.Fatal(err)
			}
			if got := bodyTag(t, local, u); got != "the node's" {
				t.Errorf("the account's body is %q", got)
			}
			if got := directoryOf(t, local, u); len(got) != 2 {
				t.Errorf("the account's directory is %v; it named two", got)
			}
			if h := chainHeight(t, local, u, "main"); h != 300 {
				t.Errorf("main is %d entries; it was 300", h)
			}
		})
	}
}

// settledRoot is a Verifier for a pull that is expected to take nothing. If
// anything asks it for a root, the pull was not past after all.
type refusingVerifier struct{ t *testing.T }

func (v refusingVerifier) AnchoredRoot(context.Context, *url.URL, uint64) ([32]byte, error) {
	v.t.Helper()
	v.t.Error("the past case asked for an anchored root; it has nothing to verify")
	return [32]byte{}, nil
}

// TestAccountMeetingPoint_PastSettlesWithoutVerifying — F7, made explicit.
//
// The past case is structurally incompatible with verification: what the node
// holds is not what the peer serves, so it cannot hash to the leaf the peer's
// receipt proves. The invariant that keeps that from being a trap is that the
// past case takes NOTHING — its batch is discarded where the meeting point is
// decided — so there is nothing to verify and nothing to write.
//
// This is the long tail's own configuration (join.fetch: ModeStateOnly with a
// Verifier), so it is not a hypothetical: without the invariant, every account
// the node is ahead on would be refused by its own verifier every round.
func TestAccountMeetingPoint_PastSettlesWithoutVerifying(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	peer := newObservedDB(t)
	putBody(t, peer, u, "the peer's")
	addEntries(t, peer, u, "main", 0, 200, 0x11)

	local := newObservedDB(t)
	putBody(t, local, u, "the node's")
	addEntries(t, local, u, "main", 0, 300, 0x11)

	b := local.Begin(true)
	defer b.Discard()
	p, err := Fetch(context.Background(), &dbSource{db: peer}, b, u, Options{
		Mode:      ModeStateOnly,
		Verify:    refusingVerifier{t},
		Partition: protocol.PartitionUrl("BVN1"),
	}, false)
	if err != nil {
		t.Fatalf("a peer that is simply behind was refused: %v", err)
	}
	if !p.Past() {
		t.Fatal("the pull did not report the node past the peer")
	}
	// Settled against a root nobody anchored. It holds because there is
	// nothing in the batch to hash: an empty root would fail any real state.
	if err := p.Settle([32]byte{}); err != nil {
		t.Fatalf("a pull that took nothing could not be settled: %v", err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := bodyTag(t, local, u); got != "the node's" {
		t.Fatalf("the account's body is %q", got)
	}
	if h := chainHeight(t, local, u, "main"); h != 300 {
		t.Fatalf("main is %d entries; it was 300", h)
	}
}
