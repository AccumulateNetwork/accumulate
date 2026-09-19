// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestReview4348_WhichGuardFires prints the exact refusal for each peer height
// against a head-first chain, so the entry-readable guard can be told from the
// StateAt one — and so a guard that never fires can be told from one that does.
func TestReview4348_WhichGuardFires(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	full := newObservedDB(t)
	buildChain(t, full, u, 400, 0x11)

	local := newObservedDB(t)
	b0 := local.Begin(true)
	if err := Account(context.Background(), &dbSource{db: full}, b0, u, Options{Mode: ModeStateOnly}); err != nil {
		t.Fatal(err)
	}
	if err := b0.Commit(); err != nil {
		t.Fatal(err)
	}

	// What the node can actually read of its own chain.
	func() {
		b := local.Begin(false)
		defer b.Discard()
		c := b.Account(u).MainChain().Inner()
		for _, i := range []int64{0, 99, 255, 256, 300, 399} {
			_, eErr := c.Entry(i)
			st, sErr := c.StateAt(i)
			var n int64 = -1
			if st != nil {
				n = st.Count
			}
			t.Logf("local head-first at 400: Entry(%d) err=%v | StateAt(%d) count=%d err=%v", i, eErr, i, n, sErr)
		}
	}()

	for _, n := range []int{1, 100, 200, 256, 300, 350, 399, 400} {
		err := meetPeerAt(t, local, u, n, ModeFullSpine)
		t.Logf("peer at %d: %v", n, err)
	}
}

// TestReview4348_APeerThatServesAnEmptyChain — a peer that names a chain the
// node holds and serves Count 0 for it. What does the node do?
func TestReview4348_APeerThatServesAnEmptyChain(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	local := newObservedDB(t)
	buildChain(t, local, u, 300, 0x11)

	// A peer holding the account with a main chain that is registered but
	// empty. That is what a chain looks like the instant it is created.
	peer := newObservedDB(t)
	func() {
		b := peer.Begin(true)
		defer b.Discard()
		if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
			t.Fatal(err)
		}
		c, err := b.Account(u).ChainByName("main")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Get(); err != nil { // register it in the index
			t.Fatal(err)
		}
		if err := b.Commit(); err != nil {
			t.Fatal(err)
		}
	}()

	for _, mode := range []Mode{ModeStateOnly, ModeFullSpine} {
		b := local.Begin(true)
		err := Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: mode})
		t.Logf("mode %d against a peer serving an empty main chain: %v", mode, err)
		if err == nil {
			if err := b.Commit(); err != nil {
				t.Fatal(err)
			}
		} else {
			b.Discard()
		}
		t.Logf("  local height now %d", localHeight(t, local, u))
	}
}

// TestReview4348_TheHeightIsNotEnoughOnItsOwn measures StateAt directly on a
// head-first chain: does it hand back a state of the right height built out of
// the wrong hashes, and is the entry at that height readable?
func TestReview4348_TheHeightIsNotEnoughOnItsOwn(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	full := newObservedDB(t)
	buildChain(t, full, u, 400, 0x11)

	local := newObservedDB(t)
	b0 := local.Begin(true)
	if err := Account(context.Background(), &dbSource{db: full}, b0, u, Options{Mode: ModeStateOnly}); err != nil {
		t.Fatal(err)
	}
	if err := b0.Commit(); err != nil {
		t.Fatal(err)
	}

	b := local.Begin(false)
	defer b.Discard()
	mine := b.Account(u).MainChain().Inner()

	fb := full.Begin(false)
	defer fb.Discard()
	real := fb.Account(u).MainChain().Inner()

	for _, i := range []int64{0, 50, 99, 150, 255, 256, 300} {
		got, gErr := mine.StateAt(i)
		want, wErr := real.StateAt(i)
		if gErr != nil || wErr != nil {
			t.Logf("StateAt(%d): local err=%v real err=%v", i, gErr, wErr)
			continue
		}
		_, eErr := mine.Entry(i)
		same := string(got.Anchor()) == string(want.Anchor())
		t.Logf("StateAt(%d): local count=%d anchor matches the real chain: %v | Entry readable: %v",
			i, got.Count, same, eErr == nil)
		if !same && eErr == nil {
			t.Errorf("**** StateAt(%d) is the wrong state AND the entry is readable: the guard does not cover this", i)
		}
	}
	_ = merkle.State{}
	_ = database.Batch{}
	_ = url.URL{}
}
