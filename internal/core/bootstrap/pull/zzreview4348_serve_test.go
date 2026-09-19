// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestReview4348_WhatAHeadFirstNodeCanServe — a node that joined by pulling
// state only holds no entries below its open mark set. What can it serve to
// the next node?
//
// The branch's e2e tests build every peer that way (copyOf), but every chain
// in them is shorter than a mark point, where head-first keeps everything. On
// a chain longer than a mark point it does not.
func TestReview4348_WhatAHeadFirstNodeCanServe(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)

	// A node that executed the chain: 400 entries, mark points and all.
	full := newObservedDB(t)
	buildChain(t, full, u, 400, 0x11)

	// A node that joined from it, state only.
	joined := newObservedDB(t)
	b := joined.Begin(true)
	if err := Account(context.Background(), &dbSource{db: full}, b, u, Options{Mode: ModeStateOnly}); err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	t.Logf("the joined node's chain is %d entries", localHeight(t, joined, u))

	// A third node, empty, now pulls the spine from the joined node.
	third := newObservedDB(t)
	b3 := third.Begin(true)
	err := Account(context.Background(), &dbSource{db: joined}, b3, u, Options{Mode: ModeFullSpine})
	if err != nil {
		t.Logf("ModeFullSpine from a head-first node: %v", err)
		b3.Discard()
	} else {
		if err := b3.Commit(); err != nil {
			t.Fatal(err)
		}
		t.Logf("ModeFullSpine from a head-first node succeeded; the third node is at %d",
			localHeight(t, third, u))
		// Does what it got actually hold the history?
		tb := third.Begin(false)
		defer tb.Discard()
		c := tb.Account(u).MainChain().Inner()
		for _, i := range []int64{0, 100, 255, 300} {
			_, e := c.Entry(i)
			t.Logf("   third node Entry(%d): %v", i, e)
		}
	}

	// And state-only, which is what the long tail asks for.
	third2 := newObservedDB(t)
	b4 := third2.Begin(true)
	err = Account(context.Background(), &dbSource{db: joined}, b4, u, Options{Mode: ModeStateOnly})
	if err != nil {
		t.Logf("ModeStateOnly from a head-first node: %v", err)
		b4.Discard()
	} else {
		if err := b4.Commit(); err != nil {
			t.Fatal(err)
		}
		t.Logf("ModeStateOnly from a head-first node succeeded; the third node is at %d",
			localHeight(t, third2, u))
	}
}
