// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	apierrors "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// dbSource adapts a *database.Database to the pull.Source
// interface for tests. Production wraps an api.Querier2.
type dbSource struct{ db *database.Database }

func (s *dbSource) QueryAccount(_ context.Context, u *url.URL, _ *api.DefaultQuery) (*api.AccountRecord, error) {
	b := s.db.Begin(false)
	defer b.Discard()
	var acct protocol.Account
	if err := b.Account(u).Main().GetAs(&acct); err != nil {
		return nil, err
	}
	return &api.AccountRecord{Account: acct}, nil
}

func (s *dbSource) QueryDirectoryUrls(_ context.Context, u *url.URL, q *api.DirectoryQuery) (*api.RecordRange[*api.UrlRecord], error) {
	b := s.db.Begin(false)
	defer b.Discard()
	urls, err := b.Account(u).Directory().Get()
	if err != nil {
		return nil, err
	}
	out := &api.RecordRange[*api.UrlRecord]{Total: uint64(len(urls))}
	start := uint64(0)
	if q != nil && q.Range != nil {
		start = q.Range.Start
	}
	for i, du := range urls {
		if uint64(i) < start {
			continue
		}
		out.Records = append(out.Records, &api.UrlRecord{Value: du})
	}
	return out, nil
}

func (s *dbSource) QueryPendingIds(_ context.Context, u *url.URL, q *api.PendingQuery) (*api.RecordRange[*api.TxIDRecord], error) {
	b := s.db.Begin(false)
	defer b.Discard()
	ids, err := b.Account(u).Pending().Get()
	if err != nil {
		return nil, err
	}
	out := &api.RecordRange[*api.TxIDRecord]{Total: uint64(len(ids))}
	start := uint64(0)
	if q != nil && q.Range != nil {
		start = q.Range.Start
	}
	for i, id := range ids {
		if uint64(i) < start {
			continue
		}
		out.Records = append(out.Records, &api.TxIDRecord{Value: id})
	}
	return out, nil
}

func (s *dbSource) QueryAccountChains(_ context.Context, u *url.URL, _ *api.ChainQuery) (*api.RecordRange[*api.ChainRecord], error) {
	b := s.db.Begin(false)
	defer b.Discard()
	chains, err := b.Account(u).Chains().Get()
	if err != nil {
		return nil, err
	}
	out := &api.RecordRange[*api.ChainRecord]{Total: uint64(len(chains))}
	for _, cm := range chains {
		c2, err := b.Account(u).ChainByName(cm.Name)
		if err != nil {
			return nil, err
		}
		head, err := c2.Head().Get()
		if err != nil {
			return nil, err
		}
		out.Records = append(out.Records, &api.ChainRecord{
			Name:  cm.Name,
			Type:  cm.Type,
			Count: uint64(head.Count),
			State: head.Pending,
		})
	}
	return out, nil
}

// QueryMessage serves the message stored under the ID's hash, as the querier
// does; NotFound when there is none.
func (s *dbSource) QueryMessage(_ context.Context, id *url.TxID, _ *api.DefaultQuery) (*api.MessageRecord[messaging.Message], error) {
	b := s.db.Begin(false)
	defer b.Discard()
	msg, err := b.Message(id.Hash()).Main().Get()
	if err != nil {
		return nil, apierrors.NotFound.WithFormat("message %x: %w", id.Hash(), err)
	}
	return &api.MessageRecord[messaging.Message]{ID: id, Message: msg}, nil
}

func (s *dbSource) QueryChainEntries(_ context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	b := s.db.Begin(false)
	defer b.Discard()
	c2, err := b.Account(u).ChainByName(q.Name)
	if err != nil {
		return nil, err
	}
	head, err := c2.Head().Get()
	if err != nil {
		return nil, err
	}
	out := &api.RecordRange[*api.ChainEntryRecord[api.Record]]{Total: uint64(head.Count)}
	start := uint64(0)
	if q.Range != nil {
		start = q.Range.Start
	}
	count := uint64(head.Count)
	if q.Range != nil && q.Range.Count != nil {
		count = *q.Range.Count
	}
	end := start + count
	if end > uint64(head.Count) {
		end = uint64(head.Count)
	}
	for i := start; i < end; i++ {
		entry, err := c2.Entry(int64(i))
		if err != nil {
			return nil, err
		}
		var hashArr [32]byte
		copy(hashArr[:], entry)
		r := &api.ChainEntryRecord[api.Record]{
			Account: u,
			Name:    q.Name,
			Type:    c2.Type(),
			Index:   i,
			Entry:   hashArr,
		}
		// Expanded, a transaction chain's entry carries the message behind
		// it, or an error record when there is none -- the querier's shape
		// (internal/api/v3/querier.go queryChainEntry).
		if q.Range != nil && q.Range.Expand != nil && *q.Range.Expand && c2.Type() == merkle.ChainTypeTransaction {
			msg, err := b.Message(hashArr).Main().Get()
			if err == nil {
				r.Value = &api.MessageRecord[messaging.Message]{ID: protocol.UnknownUrl().WithTxID(hashArr), Message: msg}
			} else {
				r.Value = &api.ErrorRecord{Value: apierrors.NotFound.WithFormat("message %x not found", hashArr[:4])}
			}
		}
		out.Records = append(out.Records, r)
	}
	return out, nil
}

func newObservedDB(t *testing.T) *database.Database {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	return db
}

// servedBlock is the block a peer fixture claims to have served state at.
const servedBlock = 17

// peer serves state out of a database and, with it, the receipt the real
// querier serves: the account's state proven into that database's BPT root.
type peer struct {
	*dbSource

	// empty makes the peer answer with a record carrying no account, which is
	// how a peer that has nothing to serve answers.
	empty bool
}

func (s *peer) QueryAccount(_ context.Context, u *url.URL, q *api.DefaultQuery) (*api.AccountRecord, error) {
	if s.empty {
		return new(api.AccountRecord), nil
	}

	b := s.db.Begin(false)
	defer b.Discard()

	var acct protocol.Account
	if err := b.Account(u).Main().GetAs(&acct); err != nil {
		return nil, err
	}
	rec := &api.AccountRecord{Account: acct}

	if q != nil && q.IncludeReceipt.Yes() {
		r, err := b.Account(u).StateReceipt()
		if err != nil {
			return nil, err
		}
		rec.Receipt = &api.Receipt{LocalBlock: servedBlock}
		rec.Receipt.Receipt = *r
	}
	return rec, nil
}

// alice builds a database holding one non-trivial account and returns it with
// the account's URL and the database's BPT root.
func alice(t *testing.T) (*database.Database, *url.URL, [32]byte) {
	t.Helper()
	u := protocol.DnUrl().JoinPath("alice")
	db := newObservedDB(t)

	b := db.Begin(true)
	if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		e := make([]byte, 32)
		e[0] = byte(i)
		e[31] = 0xab
		if err := b.Account(u).MainChain().Inner().AddEntry(e, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Account(u).Directory().Add(u.JoinPath("child")); err != nil {
		t.Fatal(err)
	}
	if err := b.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}

	ro := db.Begin(false)
	defer ro.Discard()
	root, err := ro.GetBptRootHash()
	if err != nil {
		t.Fatal(err)
	}
	return db, u, root
}

// TestStateOnly_BPTLeafMatches is the central correctness check for
// ModeStateOnly. Build a reference DB with a non-trivial account
// (state + Directory + Pending + chains with entries). Pull just
// the state into a fresh DB. The leaf hashes must match.
func TestStateOnly_BPTLeafMatches(t *testing.T) {
	u := protocol.DnUrl().JoinPath("alice")

	src := newObservedDB(t)
	{
		b := src.Begin(true)
		if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
			t.Fatal(err)
		}
		// Two chain entries on main chain.
		for i := 0; i < 2; i++ {
			e := make([]byte, 32)
			e[0] = byte(i)
			e[31] = 0xab
			if err := b.Account(u).MainChain().Inner().AddEntry(e, false); err != nil {
				t.Fatal(err)
			}
		}
		if err := b.Account(u).Directory().Add(u.JoinPath("child")); err != nil {
			t.Fatal(err)
		}
		var txid [32]byte
		txid[0] = 0xbe
		if err := b.Account(u).Pending().Add(u.WithTxID(txid)); err != nil {
			t.Fatal(err)
		}
		if err := b.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	srcRO := src.Begin(false)
	defer srcRO.Discard()
	wantHash, err := srcRO.Account(u).Hash()
	if err != nil {
		t.Fatal(err)
	}

	// Pull state-only into dst.
	dst := newObservedDB(t)
	dstBatch := dst.Begin(true)
	if err := Account(context.Background(), &dbSource{db: src}, dstBatch, u, Options{Mode: ModeStateOnly}); err != nil {
		t.Fatalf("pull.Account: %v", err)
	}
	if err := dstBatch.Commit(); err != nil {
		t.Fatal(err)
	}

	dstRO := dst.Begin(false)
	defer dstRO.Discard()
	gotHash, err := dstRO.Account(u).Hash()
	if err != nil {
		t.Fatal(err)
	}

	if gotHash != wantHash {
		t.Fatalf("BPT leaf hash mismatch after state-only pull:\n  want: %x\n  got:  %x", wantHash, gotHash)
	}
}

// TestFullSpine_ChainEntriesReplayed verifies ModeFullSpine actually
// pulls every chain entry. After pulling, querying the chain locally
// returns the same entries the source has.
func TestFullSpine_ChainEntriesReplayed(t *testing.T) {
	u := protocol.DnUrl().JoinPath("anchors")

	src := newObservedDB(t)
	{
		b := src.Begin(true)
		if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 5; i++ {
			addTransactionEntry(t, b, u, i, 0x99)
		}
		if err := b.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	dst := newObservedDB(t)
	dstBatch := dst.Begin(true)
	if err := Account(context.Background(), &dbSource{db: src}, dstBatch, u, Options{Mode: ModeFullSpine}); err != nil {
		t.Fatalf("pull.Account: %v", err)
	}
	if err := dstBatch.Commit(); err != nil {
		t.Fatal(err)
	}

	srcRO := src.Begin(false)
	defer srcRO.Discard()
	dstRO := dst.Begin(false)
	defer dstRO.Discard()

	// Both should have 5 entries on main chain with matching content.
	srcChain, err := srcRO.Account(u).MainChain().Get()
	if err != nil {
		t.Fatal(err)
	}
	dstChain, err := dstRO.Account(u).MainChain().Get()
	if err != nil {
		t.Fatal(err)
	}
	srcHead := srcChain.CurrentState()
	dstHead := dstChain.CurrentState()
	if srcHead.Count != dstHead.Count {
		t.Errorf("entry count mismatch: src=%d dst=%d", srcHead.Count, dstHead.Count)
	}
	for i := int64(0); i < srcHead.Count; i++ {
		se, err := srcChain.Entry(i)
		if err != nil {
			t.Fatal(err)
		}
		de, err := dstChain.Entry(i)
		if err != nil {
			t.Fatal(err)
		}
		if string(se) != string(de) {
			t.Errorf("entry %d mismatch: src=%x dst=%x", i, se, de)
		}
	}

	// And BPT leaf should match too.
	wantHash, _ := srcRO.Account(u).Hash()
	gotHash, _ := dstRO.Account(u).Hash()
	if gotHash != wantHash {
		t.Errorf("BPT leaf hash mismatch after spine pull:\n  want: %x\n  got:  %x", wantHash, gotHash)
	}
}

// buildChain writes an account whose main chain holds n entries.
func buildChain(t *testing.T, db *database.Database, u *url.URL, n int, salt byte) {
	t.Helper()
	b := db.Begin(true)
	defer b.Discard()
	if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		addTransactionEntry(t, b, u, i, salt)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
}

// addTransactionEntry stores a transaction and appends its hash to u's main
// chain, as execution does: a transaction chain's entry names a message the
// store holds.
func addTransactionEntry(t *testing.T, b *database.Batch, u *url.URL, i int, salt byte) [32]byte {
	t.Helper()
	txn := new(protocol.Transaction)
	txn.Header.Principal = u
	txn.Body = &protocol.WriteData{Entry: &protocol.DoubleHashDataEntry{Data: [][]byte{{byte(i), byte(i >> 8), salt}}}}
	msg := &messaging.TransactionMessage{Transaction: txn}
	h := msg.Hash()
	if err := b.Message(h).Main().Put(msg); err != nil {
		t.Fatal(err)
	}
	if err := b.Account(u).MainChain().Inner().AddEntry(h[:], false); err != nil {
		t.Fatal(err)
	}
	return h
}

// TestStateOnly_PullsTheOpenMarkSetOnly — ModeStateOnly does not replay a
// chain's history, but it cannot skip the open mark set either: an append
// rebuilds the tail chunk from the elements of the open set. Below the last
// mark point nothing is pulled.
func TestStateOnly_PullsTheOpenMarkSetOnly(t *testing.T) {
	u := protocol.DnUrl().JoinPath("alice")

	src := newObservedDB(t)
	buildChain(t, src, u, 260, 0xcc) // past the 256-entry mark point

	dst := newObservedDB(t)
	dstBatch := dst.Begin(true)
	if err := Account(context.Background(), &dbSource{db: src}, dstBatch, u, Options{Mode: ModeStateOnly}); err != nil {
		t.Fatal(err)
	}
	if err := dstBatch.Commit(); err != nil {
		t.Fatal(err)
	}

	dstRO := dst.Begin(false)
	defer dstRO.Discard()
	dstChain, err := dstRO.Account(u).MainChain().Get()
	if err != nil {
		t.Fatal(err)
	}
	if dstChain.CurrentState().Count != 260 {
		t.Errorf("Head Count = %d, want 260", dstChain.CurrentState().Count)
	}
	// The open mark set is [256, 260) — those are held.
	for i := int64(256); i < 260; i++ {
		if _, err := dstChain.Entry(i); err != nil {
			t.Errorf("entry %d of the open mark set was not pulled: %v", i, err)
		}
	}
	// Below the last mark point nothing was pulled.
	if _, err := dstChain.Entry(0); err == nil {
		t.Error("ModeStateOnly pulled an entry below the last mark point")
	}
}

// TestStateOnly_ChainCanBeAppendedTo is the rule a joined node depends on: it
// executes block Q+1, which appends to the chains it pulled. A pulled chain
// must therefore take the same entry the source takes and anchor to the same
// place — otherwise the node's first block differs from its peers' and its
// root chain never matches again (#4290).
func TestStateOnly_ChainCanBeAppendedTo(t *testing.T) {
	// 3: a chain whose whole set is open. 255: the append closes the mark
	// set, which is assembled from every chunk of it. 256: the set is closed
	// and the open one is empty. 260: past a mark point.
	for _, height := range []int{3, 255, 256, 260} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			u := protocol.DnUrl().JoinPath("alice")

			src := newObservedDB(t)
			buildChain(t, src, u, height, 0xcc)

			dst := newObservedDB(t)
			b := dst.Begin(true)
			if err := Account(context.Background(), &dbSource{db: src}, b, u, Options{Mode: ModeStateOnly}); err != nil {
				t.Fatal(err)
			}
			if err := b.Commit(); err != nil {
				t.Fatal(err)
			}

			next := make([]byte, 32)
			next[0] = 0x77
			next[31] = 0x77

			sb := src.Begin(true)
			if err := sb.Account(u).MainChain().Inner().AddEntry(next, false); err != nil {
				t.Fatal(err)
			}
			if err := sb.Commit(); err != nil {
				t.Fatal(err)
			}

			db := dst.Begin(true)
			if err := db.Account(u).MainChain().Inner().AddEntry(next, false); err != nil {
				t.Fatalf("a pulled chain could not be appended to: %v", err)
			}
			if err := db.Commit(); err != nil {
				t.Fatal(err)
			}

			sro, dro := src.Begin(false), dst.Begin(false)
			defer sro.Discard()
			defer dro.Discard()
			want, err := sro.Account(u).MainChain().Anchor()
			if err != nil {
				t.Fatal(err)
			}
			got, err := dro.Account(u).MainChain().Anchor()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(want, got) {
				t.Fatalf("after the same append the anchors differ:\n  source %x\n  pulled %x", want, got)
			}
			if want, err := sro.Account(u).Hash(); err != nil {
				t.Fatal(err)
			} else if got, err := dro.Account(u).Hash(); err != nil {
				t.Fatal(err)
			} else if got != want {
				t.Fatalf("after the same append the leaves differ:\n  source %x\n  pulled %x", want, got)
			}
		})
	}
}

// TestFullSpine_RePullIsIdempotent — a restarting node re-pulls the spine. A
// pull that appended from index 0 every time doubled the chain.
func TestFullSpine_RePullIsIdempotent(t *testing.T) {
	u := protocol.DnUrl().JoinPath("anchors")

	src := newObservedDB(t)
	buildChain(t, src, u, 3, 0x99)

	dst := newObservedDB(t)
	for i := 0; i < 2; i++ {
		b := dst.Begin(true)
		if err := Account(context.Background(), &dbSource{db: src}, b, u, Options{Mode: ModeFullSpine}); err != nil {
			t.Fatalf("pull %d: %v", i+1, err)
		}
		if err := b.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	ro := dst.Begin(false)
	defer ro.Discard()
	c, err := ro.Account(u).MainChain().Get()
	if err != nil {
		t.Fatal(err)
	}
	if c.CurrentState().Count != 3 {
		t.Fatalf("the chain is %d entries after two pulls of a 3-entry chain", c.CurrentState().Count)
	}

	// A peer serving a shorter chain cannot be reproduced by appending, so it
	// is refused rather than silently left as it is.
	shorter := newObservedDB(t)
	buildChain(t, shorter, u, 1, 0x99)
	b := dst.Begin(true)
	defer b.Discard()
	if err := Account(context.Background(), &dbSource{db: shorter}, b, u, Options{Mode: ModeFullSpine}); err == nil {
		t.Fatal("a peer serving a shorter chain than the node holds was accepted")
	}
}

// TestRejectsMissingInputs — guards.
func TestRejectsMissingInputs(t *testing.T) {
	u := protocol.DnUrl().JoinPath("alice")
	dst := newObservedDB(t)
	batch := dst.Begin(true)
	defer batch.Discard()

	cases := []struct {
		name string
		src  Source
		bt   *database.Batch
		u    *url.URL
	}{
		{"no src", nil, batch, u},
		{"no batch", &dbSource{db: dst}, nil, u},
		{"no url", &dbSource{db: dst}, batch, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Account(context.Background(), c.src, c.bt, c.u, Options{})
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDnSpineAccounts(t *testing.T) {
	got := DnSpineAccounts()
	// The anchor pool, the ledger, the operator book and its page, plus the
	// network definition and the globals -- the accounts that say who may
	// sign an anchor, which a joining node can only move by pulling them
	// verified (#4301).
	want := []string{"anchors", "ledger", "operators", "operators/1", "network", "globals"}
	if len(got) != len(want) {
		t.Errorf("got %d spine accounts, want %d", len(got), len(want))
	}
	for i, name := range want {
		if i < len(got) && !strings.EqualFold(strings.Trim(got[i].Path, "/"), name) {
			t.Errorf("spine account %d is %v, want %s", i, got[i], name)
		}
	}
	// Sanity: each one is under dn.acme.
	for _, u := range got {
		if u == nil {
			t.Fatal("nil spine url")
		}
		if u.RootIdentity().String() != protocol.DnUrl().String() {
			t.Errorf("spine account %s not under dn.acme", u)
		}
	}
}
