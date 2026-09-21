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

// The account-level meeting point is gone, and these are the rules that stand
// in its place (#4362, #4348 x2, #4350).
//
// The meeting point asked "am I ahead of this peer, level with it, or behind
// it", and every answer it could give was wrong at least sometimes. The
// question only existed because an account was compared against a peer's
// CURRENT state, which is a moving target no anchor covers. A pull asks for a
// block a quorum signed an anchor for, and at that block a chain had one
// length and one head: the node's chain is shorter than it, equal to it, or
// longer than it, and there is no fourth case and no ordering to decide.
//
// What the meeting point's tests proved and these keep: a chain the node built
// is never shortened, and a disagreement below a shared position is refused
// as a disagreement rather than as a height.

// entryAt is the entry a chain of a given salt holds at index i. Two chains
// built with the same salt agree entry for entry, so the shorter is a prefix
// of the longer.
func entryAt(i int, salt byte) []byte {
	e := make([]byte, 32)
	e[0] = byte(i)
	e[1] = byte(i >> 8)
	e[31] = salt
	return e
}

// buildChainForking writes an account whose main chain holds n entries, of
// which the one at index fork (if 0 <= fork < n) is replaced by a different
// hash: a chain that DISAGREES with the salt's chain at a shared position, as
// opposed to one that is merely shorter.
func buildChainForking(t *testing.T, db *database.Database, u *url.URL, n int, salt byte, fork int) {
	t.Helper()
	b := db.Begin(true)
	defer b.Discard()
	if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		e := entryAt(i, salt)
		if i == fork {
			e[30] = 0xff
		}
		if err := b.Account(u).MainChain().Inner().AddEntry(e, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
}

// localHeight reports the height of u's main chain in db.
func localHeight(t *testing.T, db *database.Database, u *url.URL) int64 {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	head, err := b.Account(u).MainChain().Inner().Head().Get()
	if err != nil {
		t.Fatal(err)
	}
	return head.Count
}

// TestChainsAtBlock_ALocalChainPastTheBlockIsRefusedNotShortened is the
// invariant #4348 is really about. A joining node has executed nothing since
// it stopped, and it asks about a block at or after the one it stopped at, so
// holding MORE than that block held is either a peer serving a length that is
// not the block's or a node holding something no node held.
//
// It is refused, and it is never shortened. Rewinding the chain would make the
// account hash to exactly the leaf the peer's receipt proves, so nothing
// downstream could catch it.
func TestChainsAtBlock_ALocalChainPastTheBlockIsRefusedNotShortened(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	peer := newObservedDB(t)
	buildChain(t, peer, u, 631, 0x11)

	local := newObservedDB(t)
	buildChain(t, local, u, 1303, 0x11) // the same chain, 672 entries further on

	b := local.Begin(true)
	defer b.Discard()
	err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeFullSpine})
	if err == nil {
		t.Fatal("a peer serving a shorter chain than the node holds was accepted")
	}
	t.Logf("refused, as it must be: %v", err)
	if !strings.Contains(err.Error(), "cannot be past the block it is pulling") {
		t.Errorf("refused for some other reason: %v", err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	if h := localHeight(t, local, u); h != 1303 {
		t.Fatalf("the local chain is %d entries after a refused pull; it was 1303 and must not have been shortened", h)
	}
}

// TestChainsAtBlock_ADisagreementIsRefusedAsADisagreement — the node is behind
// and the fill runs, but the replay does not reproduce the head the peer
// served, because the two hold different entries below the node's own height.
// That is two nodes holding different history and it stays loud.
func TestChainsAtBlock_ADisagreementIsRefusedAsADisagreement(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	peer := newObservedDB(t)
	buildChain(t, peer, u, 1303, 0x11)

	local := newObservedDB(t)
	buildChainForking(t, local, u, 631, 0x11, 400)

	b := local.Begin(true)
	defer b.Discard()
	err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeFullSpine})
	if err == nil {
		t.Fatal("a fill whose replay does not reproduce the peer's head was accepted")
	}
	t.Logf("refused, as it must be: %v", err)
	if !strings.Contains(err.Error(), "disagree") {
		t.Errorf("a disagreement was refused for some other reason: %v", err)
	}
}

// TestChainsAtBlock_LevelAndAgreeingTakesNothingAndRefusesNothing — the node's
// chain is exactly the block's, and the two anchor to the same value. There is
// nothing to fetch and nothing to refuse.
func TestChainsAtBlock_LevelAndAgreeingTakesNothingAndRefusesNothing(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	peer := newObservedDB(t)
	buildChain(t, peer, u, 631, 0x11)

	local := newObservedDB(t)
	buildChain(t, local, u, 631, 0x11)

	b := local.Begin(true)
	defer b.Discard()
	if err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeStateOnly}); err != nil {
		t.Fatalf("a chain level with the block's and agreeing with it was refused: %v", err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	if h := localHeight(t, local, u); h != 631 {
		t.Fatalf("the local chain is %d entries; it was 631 and nothing should have moved it", h)
	}
}

// TestChainsAtBlock_LevelAndDisagreeingIsRefused — the same length, different
// history. The old level case took the peer's body without noticing, which is
// what #4350 is: an account's body moves with all of its chains standing
// still, so length alone says nothing.
func TestChainsAtBlock_LevelAndDisagreeingIsRefused(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	peer := newObservedDB(t)
	buildChain(t, peer, u, 631, 0x11)

	local := newObservedDB(t)
	buildChainForking(t, local, u, 631, 0x11, 400)

	b := local.Begin(true)
	defer b.Discard()
	err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeStateOnly})
	if err == nil {
		t.Fatal("a chain of the same length holding different history was accepted")
	}
	t.Logf("refused, as it must be: %v", err)
	if !strings.Contains(err.Error(), "disagree") {
		t.Errorf("refused for some other reason: %v", err)
	}
}

// TestChainsAtBlock_APeerThatServesNoChainsIsRefused — a peer that names none
// of the chains the node holds is not a peer the node is "past"; it is a peer
// whose account is not this account. Taking its body would install it over a
// chain the node built.
func TestChainsAtBlock_APeerThatServesNoChainsIsRefused(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	peer := newObservedDB(t)
	b0 := peer.Begin(true)
	if err := b0.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
		t.Fatal(err)
	}
	if err := b0.Commit(); err != nil {
		t.Fatal(err)
	}

	local := newObservedDB(t)
	buildChain(t, local, u, 631, 0x11)

	b := local.Begin(true)
	defer b.Discard()
	err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeStateOnly})
	if err == nil {
		t.Fatal("a peer that serves none of the node's chains was accepted")
	}
	t.Logf("refused, as it must be: %v", err)
	if !strings.Contains(err.Error(), "serves none") {
		t.Errorf("refused for some other reason: %v", err)
	}
}
