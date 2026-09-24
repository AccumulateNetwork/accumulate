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

// entryAt is the entry a chain of a given salt holds at index i. Two chains
// built with the same salt agree entry for entry, so the shorter is a prefix
// of the longer -- which is what a peer that is simply behind looks like.
func entryAt(i int, salt byte) []byte {
	e := make([]byte, 32)
	e[0] = byte(i)
	e[1] = byte(i >> 8)
	e[31] = salt
	return e
}

// buildChainForking writes an account whose main chain holds n entries, of
// which the one at index fork (if 0 <= fork < n) is replaced by a different
// hash. That is a chain that DISAGREES with the salt's chain at a shared
// position, as opposed to one that is merely shorter.
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

// The three ways the spine pull failed on a twelve-node restart, against
// acc://bvn-BVN1.acme/anchors:
//
//	source 0: the replayed chain anchors to 767d... and the peer's head to 1e17...
//	source 1: the local chain is at 1303 and the peer served 631; it cannot be re-pulled
//	source 2: (as source 0, other hashes)

// TestMeetingPoint_LocalAheadOfAPeerThatAgrees is the restart case: the node
// executed to 1303 and asks a peer that is at 631, whose chain is the node's
// own first 631 entries. The meeting point is reached at 631 and there is
// nothing to fill.
func TestMeetingPoint_LocalAheadOfAPeerThatAgrees(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	peer := newObservedDB(t)
	buildChain(t, peer, u, 631, 0x11)

	local := newObservedDB(t)
	buildChain(t, local, u, 1303, 0x11) // the same chain, 672 entries further on

	b := local.Begin(true)
	defer b.Discard()
	err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeFullSpine})
	if err != nil {
		t.Fatalf("a peer that is simply behind was refused: %v", err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	if h := localHeight(t, local, u); h != 1303 {
		t.Fatalf("the local chain is %d entries after meeting a peer at 631; it was 1303", h)
	}
}

// TestMeetingPoint_LocalAheadOfAPeerThatDisagrees — ahead is not by itself
// safe. A peer at 631 whose entry 400 is not the node's entry 400 is a
// disagreement at a shared position, and must be refused, loudly and as a
// disagreement rather than as a height.
func TestMeetingPoint_LocalAheadOfAPeerThatDisagrees(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	peer := newObservedDB(t)
	buildChainForking(t, peer, u, 631, 0x11, 400)

	local := newObservedDB(t)
	buildChain(t, local, u, 1303, 0x11)

	b := local.Begin(true)
	defer b.Discard()
	err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeFullSpine})
	if err == nil {
		t.Fatal("a peer that disagrees with the node at entry 400 was accepted")
	}
	t.Logf("refused, as it must be: %v", err)
	// It must be refused AS A DISAGREEMENT. Refusing it for its height is
	// the same answer the agreeing peer above gets, and the two are not the
	// same thing: one of the two answers is then necessarily wrong.
	if !strings.Contains(err.Error(), "disagree") {
		t.Errorf("a disagreement was refused for some other reason: %v", err)
	}
	if !strings.Contains(err.Error(), "631") {
		t.Errorf("the refusal does not name the height they were compared at: %v", err)
	}
}

// TestMeetingPoint_LocalBehindAPeerThatDisagrees is sources 0 and 2: the node
// is behind, the fill runs, and the replay does not reproduce the peer's head
// because the two disagree below the node's own height. That stays loud.
func TestMeetingPoint_LocalBehindAPeerThatDisagrees(t *testing.T) {
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
}

// TestMeetingPoint_AheadWithNoStateAtThePeersHeight — the node is ahead, and
// cannot show that the peer's chain is its own prefix because it does not hold
// the history below its last mark point (a chain restored head-first). Ahead
// is only safe when agreement can be shown, so this is refused — and refused
// as something that could not be compared, not as a disagreement, because the
// node has not established one.
func TestMeetingPoint_AheadWithNoStateAtThePeersHeight(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	full := newObservedDB(t)
	buildChain(t, full, u, 300, 0x11)

	// The node holds this chain head-first: the head and the open mark set
	// [256, 300), and nothing below 256.
	local := newObservedDB(t)
	b0 := local.Begin(true)
	if err := Account(context.Background(), &dbSource{db: full}, b0, u, Options{Mode: ModeStateOnly}); err != nil {
		t.Fatal(err)
	}
	if err := b0.Commit(); err != nil {
		t.Fatal(err)
	}

	peer := newObservedDB(t)
	buildChain(t, peer, u, 200, 0x11) // genuinely the node's own prefix

	b := local.Begin(true)
	defer b.Discard()
	err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeFullSpine})
	if err == nil {
		t.Fatal("a chain that cannot be compared at the peer's height was accepted as ahead")
	}
	t.Logf("refused, as it must be: %v", err)
	if !strings.Contains(err.Error(), "cannot be read") {
		t.Errorf("refused for a reason it did not establish: %v", err)
	}
}

// TestMeetingPoint_TheThreeSourcesOfTheLiveFailure drives the shape the live
// network produced: pullSpine's FetchFrom over three peers, two of which
// disagree with the node and one of which is simply behind it. The node has
// what it needs from the third, and the pull must complete.
func TestMeetingPoint_TheThreeSourcesOfTheLiveFailure(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	forked1 := newObservedDB(t)
	buildChainForking(t, forked1, u, 1400, 0x11, 10)
	behind := newObservedDB(t)
	buildChain(t, behind, u, 631, 0x11)
	forked2 := newObservedDB(t)
	buildChainForking(t, forked2, u, 1400, 0x11, 900)

	local := newObservedDB(t)
	buildChain(t, local, u, 1303, 0x11)

	srcs := []Source{&dbSource{db: forked1}, &dbSource{db: behind}, &dbSource{db: forked2}}

	b := local.Begin(true)
	defer b.Discard()
	p, i, err := FetchFrom(context.Background(), srcs, b, u, Options{Mode: ModeFullSpine})
	if err != nil {
		t.Fatalf("no source served the spine account:\n%v", err)
	}
	if err := p.Keep(); err != nil {
		t.Fatal(err)
	}
	if i != 1 {
		t.Errorf("source %d answered, want source 1 (the one that is behind and agrees)", i)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	if h := localHeight(t, local, u); h != 1303 {
		t.Fatalf("the local chain is %d entries after the pull; it was 1303", h)
	}
}
