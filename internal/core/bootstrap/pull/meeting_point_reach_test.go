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

// F4. Being ahead is only safe when the prefix can be SHOWN, and a chain the
// node holds only from a mark point on cannot show it. These two tests state
// how far that refusal reaches and what the ModeStateOnly fix takes off it.

// meetPeerAt builds a peer holding the first n entries of the node's own chain
// and reports what the pull said about it.
func meetPeerAt(t *testing.T, local *database.Database, u *url.URL, n int, mode Mode) error {
	t.Helper()
	peer := newObservedDB(t)
	buildChain(t, peer, u, n, 0x11)
	b := local.Begin(true)
	defer b.Discard()
	err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: mode})
	if err == nil {
		if err := b.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	return err
}

// TestMeetingPoint_TheReachOfCannotBeCompared measures how far "the local
// chain holds no state at N" reaches on a chain the node received head-first.
//
// The refusal is the right conservative answer — a node that cannot compute
// its own state at the peer's height cannot tell being ahead from holding
// different history — but its reach is worth saying out loud: EVERY peer below
// the node's own height is refused, not merely peers below the mark point, and
// only an exactly-equal peer is accepted, until the node's own execution
// crosses a mark point and starts writing mark states of its own.
func TestMeetingPoint_TheReachOfCannotBeCompared(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	full := newObservedDB(t)
	buildChain(t, full, u, 400, 0x11)

	// A node that received this chain head-first, as a join's long tail
	// delivers every chain it pulls.
	local := newObservedDB(t)
	b0 := local.Begin(true)
	if err := Account(context.Background(), &dbSource{db: full}, b0, u, Options{Mode: ModeStateOnly}); err != nil {
		t.Fatal(err)
	}
	if err := b0.Commit(); err != nil {
		t.Fatal(err)
	}
	if h := localHeight(t, local, u); h != 400 {
		t.Fatalf("the restored chain is %d entries, want 400", h)
	}

	var accepted, refused []int
	for _, n := range []int{100, 200, 300, 350, 399, 400} {
		err := meetPeerAt(t, local, u, n, ModeFullSpine)
		switch {
		case err == nil:
			accepted = append(accepted, n)
		case strings.Contains(err.Error(), "cannot be read"):
			refused = append(refused, n)
		default:
			t.Fatalf("a peer at %d was refused for an unexpected reason: %v", n, err)
		}
	}
	t.Logf("head-first at 400: accepted %v, refused as incomparable %v", accepted, refused)
	if len(accepted) != 1 || accepted[0] != 400 {
		t.Errorf("accepted %v, want only the exactly-equal peer at 400", accepted)
	}
	if len(refused) != 5 {
		t.Errorf("refused %v, want every non-equal peer below 400", refused)
	}

	// The reach ends where the node's own execution starts writing mark
	// states: a peer at a height the node has a mark point below is
	// comparable again.
	addEntries(t, local, u, "main", 400, 200, 0x11) // on to 600, across the mark at 512
	if err := meetPeerAt(t, local, u, 520, ModeFullSpine); err != nil {
		t.Errorf("a peer above the mark point the node crossed itself was still refused: %v", err)
	} else {
		t.Log("after the node executed across a mark point, a peer at 520 is comparable again")
	}
	if err := meetPeerAt(t, local, u, 450, ModeFullSpine); err == nil {
		t.Log("a peer at 450 is comparable too")
	} else {
		t.Logf("a peer at 450 is still incomparable: %v", err)
	}
}

// TestMeetingPoint_AJoinNoLongerTruncatesTheNodesOwnHistory — the reach above
// used to be self-inflicted. ModeStateOnly restored every head the peer served
// unconditionally, so a node that held a chain in full and pulled it from a
// peer that was behind came out the other side holding it head-first, from a
// mark point: it could compare nothing on its next restart, and its next join
// refused every peer that was not exactly level with it.
//
// A node that holds a chain in full now keeps it, so the state it can compute
// against survives the join that used to remove it.
func TestMeetingPoint_AJoinNoLongerTruncatesTheNodesOwnHistory(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	// A node that executed this chain itself: it holds every entry and the
	// mark states that go with them.
	local := newObservedDB(t)
	buildChain(t, local, u, 400, 0x11)

	// One join round against a peer that is behind.
	peer := newObservedDB(t)
	buildChain(t, peer, u, 200, 0x11)
	b := local.Begin(true)
	if err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeStateOnly}); err != nil {
		t.Fatalf("a peer that is simply behind was refused: %v", err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	if h := localHeight(t, local, u); h != 400 {
		t.Fatalf("the chain is %d entries after a state-only pull from a peer at 200; it was 400", h)
	}

	// The node's history is still there, so the next round can still be
	// compared -- against this peer and against one at any other height.
	for _, n := range []int{100, 200, 333, 400} {
		if err := meetPeerAt(t, local, u, n, ModeStateOnly); err != nil {
			t.Errorf("after the join, a peer at %d could no longer be compared: %v", n, err)
		}
	}
	if h := localHeight(t, local, u); h != 400 {
		t.Fatalf("the chain is %d entries after four more pulls; it was 400", h)
	}
}
