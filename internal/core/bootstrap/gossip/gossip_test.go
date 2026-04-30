// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// dbSource adapts a *database.Database to the pull.Source surface.
// Same shape as the pull package's test helper.
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
func (s *dbSource) QueryDirectoryUrls(_ context.Context, _ *url.URL, _ *api.DirectoryQuery) (*api.RecordRange[*api.UrlRecord], error) {
	return &api.RecordRange[*api.UrlRecord]{}, nil
}
func (s *dbSource) QueryPendingIds(_ context.Context, _ *url.URL, _ *api.PendingQuery) (*api.RecordRange[*api.TxIDRecord], error) {
	return &api.RecordRange[*api.TxIDRecord]{}, nil
}
func (s *dbSource) QueryAccountChains(_ context.Context, u *url.URL, _ *api.ChainQuery) (*api.RecordRange[*api.ChainRecord], error) {
	b := s.db.Begin(false)
	defer b.Discard()
	chains, err := b.Account(u).Chains().Get()
	if err != nil {
		return &api.RecordRange[*api.ChainRecord]{}, nil
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
func (s *dbSource) QueryChainEntries(_ context.Context, _ *url.URL, _ *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	// Not exercised by gossip ingestion (ModeStateOnly).
	return &api.RecordRange[*api.ChainEntryRecord[api.Record]]{}, nil
}
func (s *dbSource) QueryMessage(_ context.Context, _ *url.TxID, _ *api.DefaultQuery) (*api.MessageRecord[messaging.Message], error) {
	return nil, nil
}

func newObservedDB(t *testing.T) *database.Database {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	return db
}

// TestRunChannel_AppliesBlockEvents drives synthetic BlockEvents
// referencing accounts that exist on the source DB. After
// processing, the local DB should have those accounts' state and
// the BPT leaf hashes should match the source.
func TestRunChannel_AppliesBlockEvents(t *testing.T) {
	uA := protocol.DnUrl().JoinPath("alice")
	uB := protocol.DnUrl().JoinPath("bob")

	src := newObservedDB(t)
	{
		b := src.Begin(true)
		if err := b.Account(uA).Main().Put(&protocol.DataAccount{Url: uA}); err != nil {
			t.Fatal(err)
		}
		if err := b.Account(uB).Main().Put(&protocol.DataAccount{Url: uB}); err != nil {
			t.Fatal(err)
		}
		if err := b.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	srcRO := src.Begin(false)
	defer srcRO.Discard()

	dst := newObservedDB(t)

	// Construct a synthetic BlockEvent referencing both accounts.
	ev := &api.BlockEvent{
		Partition: "Directory",
		Index:     100,
		Time:      time.Unix(1700000000, 0).UTC(),
		Entries: []*api.ChainEntryRecord[api.Record]{
			{Account: uA, Name: "main", Index: 0},
			{Account: uB, Name: "main", Index: 0},
		},
	}

	ch := make(chan api.Event, 1)
	ch <- ev
	close(ch)

	var calls int32
	err := RunChannel(context.Background(), &dbSource{db: src}, ch, dst, Options{
		Partition: "Directory",
		OnBlock: func(_ *api.BlockEvent, touched []*url.URL) {
			atomic.AddInt32(&calls, 1)
			if len(touched) != 2 {
				t.Errorf("touched count = %d, want 2", len(touched))
			}
		},
	})
	if err != nil {
		t.Fatalf("RunChannel: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("OnBlock fired %d times, want 1", calls)
	}

	// Both accounts should now exist locally with matching BPT leaf hashes.
	dstRO := dst.Begin(false)
	defer dstRO.Discard()
	for _, u := range []*url.URL{uA, uB} {
		want, err := srcRO.Account(u).Hash()
		if err != nil {
			t.Fatal(err)
		}
		got, err := dstRO.Account(u).Hash()
		if err != nil {
			t.Fatalf("dst %s: %v", u, err)
		}
		if got != want {
			t.Errorf("BPT leaf hash mismatch for %s:\n  want: %x\n  got:  %x", u, want, got)
		}
	}
}

// TestRunChannel_DedupsAccountsInOneBlock — multiple chain entries
// on the same account in one block should result in a single state
// pull, not many.
func TestRunChannel_DedupsAccountsInOneBlock(t *testing.T) {
	u := protocol.DnUrl().JoinPath("alice")

	src := newObservedDB(t)
	{
		b := src.Begin(true)
		if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
			t.Fatal(err)
		}
		if err := b.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	ev := &api.BlockEvent{
		Partition: "Directory",
		Index:     1,
		Entries: []*api.ChainEntryRecord[api.Record]{
			{Account: u, Name: "main", Index: 0},
			{Account: u, Name: "signature", Index: 0},
			{Account: u, Name: "main", Index: 1},
		},
	}

	ch := make(chan api.Event, 1)
	ch <- ev
	close(ch)

	var seenTouched int
	err := RunChannel(context.Background(), &dbSource{db: src}, ch, newObservedDB(t), Options{
		Partition: "Directory",
		OnBlock: func(_ *api.BlockEvent, touched []*url.URL) {
			seenTouched = len(touched)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if seenTouched != 1 {
		t.Errorf("touched=%d after dedup, want 1", seenTouched)
	}
}

// TestRunChannel_IgnoresNonBlockEvents — ErrorEvent / GlobalsEvent
// pass through without producing pulls or errors.
func TestRunChannel_IgnoresNonBlockEvents(t *testing.T) {
	src := newObservedDB(t)
	dst := newObservedDB(t)

	ch := make(chan api.Event, 2)
	ch <- &api.GlobalsEvent{}
	ch <- &api.ErrorEvent{}
	close(ch)

	err := RunChannel(context.Background(), &dbSource{db: src}, ch, dst, Options{
		Partition: "Directory",
	})
	if err != nil {
		t.Fatalf("RunChannel: %v", err)
	}
}

// TestRunChannel_ContextCancel returns nil (clean shutdown) on
// cancel.
func TestRunChannel_ContextCancel(t *testing.T) {
	src := newObservedDB(t)
	dst := newObservedDB(t)
	ch := make(chan api.Event)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RunChannel(ctx, &dbSource{db: src}, ch, dst, Options{Partition: "Directory"})
	if err != nil {
		t.Errorf("expected nil error on canceled ctx, got %v", err)
	}
}

// TestRunChannel_RejectsMissingInputs — guards.
func TestRunChannel_RejectsMissingInputs(t *testing.T) {
	src := newObservedDB(t)
	dst := newObservedDB(t)
	ch := make(chan api.Event)
	cases := []struct {
		name string
		src  pull.Source
		db   *database.Database
	}{
		{"no src", nil, dst},
		{"no db", &dbSource{db: src}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := RunChannel(context.Background(), c.src, ch, c.db, Options{})
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
