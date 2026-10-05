// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestATakeRecordsWhatItLeavesMissing — #4493, executor spec "Past the
// match": the record is written by what makes a gap. Each case is a take of a
// chain at count C, after the node held it to prevCount with the gaps before.
func TestATakeRecordsWhatItLeavesMissing(t *testing.T) {
	type gap = database.SyncedGap
	cases := []struct {
		name            string
		chain           string
		typ             merkle.ChainType
		before          []gap
		prev, held, msg uint64
		count           uint64
		want            []gap
	}{
		{
			name: "a head taken alone, from nothing", chain: "main", typ: merkle.ChainTypeTransaction,
			prev: 0, held: 768, msg: 1000, count: 1000,
			want: []gap{{Low: 768, High: 1000, MessagesOnly: true}, {Low: 0, High: 768}},
		},
		{
			name: "a chain that carries no messages", chain: "root", typ: merkle.ChainTypeAnchor,
			prev: 0, held: 768, msg: 1000, count: 1000,
			want: []gap{{Low: 0, High: 768}},
		},
		{
			name: "a synthetic sequence chain carries no messages", chain: "synthetic-sequence(bvn1)", typ: merkle.ChainTypeTransaction,
			prev: 0, held: 768, msg: 1000, count: 1000,
			want: []gap{{Low: 0, High: 768}},
		},
		{
			name: "a chain taken whole", chain: "main", typ: merkle.ChainTypeTransaction,
			before: []gap{{Low: 0, High: 500}},
			prev:   600, held: 0, msg: 0, count: 1000,
		},
		{
			name: "a chain taken by its window", chain: "main", typ: merkle.ChainTypeTransaction,
			prev: 0, held: 900, msg: 900, count: 1000,
			want: []gap{{Low: 0, High: 900}},
		},
		{
			name: "a restart takes newer heads above what it held", chain: "main", typ: merkle.ChainTypeTransaction,
			before: []gap{{Low: 768, High: 1000, MessagesOnly: true}, {Low: 0, High: 512}},
			prev:   1000, held: 1280, msg: 1300, count: 1300,
			// The entries the node held stay held, their messages missing.
			want: []gap{
				{Low: 1280, High: 1300, MessagesOnly: true},
				{Low: 1000, High: 1280},
				{Low: 768, High: 1000, MessagesOnly: true},
				{Low: 0, High: 512},
			},
		},
		{
			name: "a restart whose new open set reaches below its old head", chain: "main", typ: merkle.ChainTypeTransaction,
			before: []gap{{Low: 900, High: 1000, MessagesOnly: true}, {Low: 0, High: 512}},
			prev:   1000, held: 768, msg: 1100, count: 1100,
			// The node held [768,900) with their messages; the take
			// rewrites those entries and loses nothing.
			want: []gap{
				{Low: 900, High: 1100, MessagesOnly: true},
				{Low: 0, High: 512},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := newObservedDB(t)
			u := protocol.DnUrl().JoinPath("anchors")
			b := db.Begin(true)
			defer b.Discard()
			var before []*gap
			for i := range c.before {
				before = append(before, &c.before[i])
			}
			if err := b.Account(u).SyncedEntries(c.chain).Put(&database.SyncedEntries{Gaps: before, HeadCount: c.prev}); err != nil {
				t.Fatal(err)
			}
			head := &merkle.State{Count: int64(c.count)}
			if err := recordTaken(b.Account(u), c.chain, c.typ, c.prev, head, c.held, c.msg); err != nil {
				t.Fatal(err)
			}
			rec, err := b.Account(u).SyncedEntries(c.chain).Get()
			if err != nil {
				t.Fatal(err)
			}
			ok := len(rec.Gaps) == len(c.want)
			for i := 0; ok && i < len(c.want); i++ {
				ok = rec.Gaps[i].Equal(&c.want[i])
			}
			if !ok || rec.HeadCount != c.count {
				var got []gap
				for _, g := range rec.Gaps {
					got = append(got, *g)
				}
				t.Fatalf("recorded %+v at %d, want %+v at %d", got, rec.HeadCount, c.want, c.count)
			}
		})
	}
}

// TestABackfillOfAChainWithNoGapAsksForNothing — #4493: the backfill asks
// for what the record names, and never asks the store which entries it
// holds. A chain taken whole has no gap: nothing is asked for, even of a
// store that has lost an entry.
func TestABackfillOfAChainWithNoGapAsksForNothing(t *testing.T) {
	const n = 1000
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	buildChain(t, src, u, n, 0x4a)
	dst := newObservedDB(t)
	b := dst.Begin(true)
	if err := Account(t.Context(), &dbSource{db: src}, b, u, Options{Mode: ModeFullSpine}); err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	requireGaps(t, dst, u)

	s := newPagedSource(src)
	if err := Backfill(t.Context(), []Source{s}, dst, u, 0, nil); err != nil {
		t.Fatal(err)
	}
	if pages, _ := s.count(); pages != 0 {
		t.Fatalf("a chain held whole asked for %d pages", pages)
	}
}
