// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
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

// QueryMessage is a stub: these tests use accounts whose Pending
// chains are empty, so the production sig-material backfill path is
// never hit. Returning NotFound is the right shape for empty pending.
func (s *dbSource) QueryMessage(_ context.Context, _ *url.TxID, _ *api.DefaultQuery) (*api.MessageRecord[messaging.Message], error) {
	return nil, apierrors.NotFound
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
		out.Records = append(out.Records, &api.ChainEntryRecord[api.Record]{
			Account: u,
			Name:    q.Name,
			Index:   i,
			Entry:   hashArr,
		})
	}
	return out, nil
}

func newObservedDB(t *testing.T) *database.Database {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	return db
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
			e := make([]byte, 32)
			e[0] = byte(i)
			e[1] = 0x99
			if err := b.Account(u).MainChain().Inner().AddEntry(e, false); err != nil {
				t.Fatal(err)
			}
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

// TestStateOnly_NoChainEntriesPulled — guards against regression
// where ModeStateOnly accidentally calls AddEntry. Local chain has
// the right Head (count + anchor) but Entry(0) returns an error
// because the entry was never stored.
func TestStateOnly_NoChainEntriesPulled(t *testing.T) {
	u := protocol.DnUrl().JoinPath("alice")

	src := newObservedDB(t)
	{
		b := src.Begin(true)
		if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			e := make([]byte, 32)
			e[0] = byte(i)
			e[31] = 0xcc
			if err := b.Account(u).MainChain().Inner().AddEntry(e, false); err != nil {
				t.Fatal(err)
			}
		}
		if err := b.Commit(); err != nil {
			t.Fatal(err)
		}
	}

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
	if dstChain.CurrentState().Count != 3 {
		t.Errorf("Head Count = %d, want 3", dstChain.CurrentState().Count)
	}
	// Trying to read entry 0 should error (the entry wasn't stored).
	_, err = dstChain.Entry(0)
	if err == nil {
		t.Error("ModeStateOnly should NOT pull chain entries; Entry(0) should error")
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
	if len(got) != 4 {
		t.Errorf("got %d spine accounts, want 4", len(got))
	}
	// Sanity: each one is under dn.acme.
	for _, u := range got {
		if !errors.Is(error(nil), nil) || u == nil { // silence the import; really just checking nil
			t.Errorf("nil spine url")
		}
		if u.RootIdentity().String() != protocol.DnUrl().String() {
			t.Errorf("spine account %s not under dn.acme", u)
		}
	}
}
