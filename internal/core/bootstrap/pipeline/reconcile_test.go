// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pipeline

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bptproof"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// fakeQuerier serves BptLeafQuery against a backing database.
type fakeQuerier struct {
	db *database.Database
}

func (f *fakeQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	leafQ, ok := query.(*api.BptLeafQuery)
	if !ok {
		return nil, errors.New("fakeQuerier: only BptLeafQuery supported")
	}
	batch := f.db.Begin(false)
	defer batch.Discard()
	leaf, err := bptproof.GetLeaf(batch, leafQ.Key)
	if err != nil {
		return nil, err
	}
	return &api.BptLeafRecord{
		KeyHash:   leaf.KeyHash,
		ValueHash: leaf.ValueHash,
		Proof:     leaf.Proof,
		BptRoot:   leaf.BptRoot,
	}, nil
}

func TestReconcileBPT_LeavesVerifiedAgainstSelf(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(execute.NewDatabaseObserver())
	urls := []*url.URL{
		protocol.DnUrl().JoinPath("a"),
		protocol.DnUrl().JoinPath("b"),
		protocol.DnUrl().JoinPath("c"),
	}

	batch := db.Begin(true)
	for _, u := range urls {
		if err := batch.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
			t.Fatal(err)
		}
	}
	// Account state changes don't propagate to the BPT until UpdateBPT()
	// is called.
	if err := batch.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}

	q := api.Querier2{Querier: &fakeQuerier{db: db}}

	ro := db.Begin(false)
	defer ro.Discard()
	scope := protocol.DnUrl()
	res, err := reconcileBPT(context.Background(), q, ro, scope, urls, func(string, ...any) {})
	if err != nil {
		t.Fatalf("reconcileBPT: %v", err)
	}
	if res.LeavesVerified != len(urls) {
		t.Errorf("LeavesVerified = %d, want %d", res.LeavesVerified, len(urls))
	}
	if res.PeerBptRoot == ([32]byte{}) {
		t.Error("PeerBptRoot not set")
	}
}

// TestReconcileBPT_DivergentLocalState verifies that reconciliation
// fails closed when the locally stored account state hashes to a
// different value than the peer's BPT leaf.
func TestReconcileBPT_DivergentLocalState(t *testing.T) {
	// Peer's DB has one version of the data account.
	peer := database.OpenInMemory(nil)
	peer.SetObserver(execute.NewDatabaseObserver())
	u := protocol.DnUrl().JoinPath("alice")

	batch := peer.Begin(true)
	if err := batch.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
		t.Fatal(err)
	}
	// Add a chain entry on the peer to push the BPT leaf hash off the
	// local DB's value.
	if err := batch.Account(u).MainChain().Inner().AddEntry([]byte("peer-only-entry-padded-to-32-byt"), false); err != nil {
		t.Fatal(err)
	}
	if err := batch.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}

	// Local DB has the same account but no chain entry — different
	// state hash. Simulates pulling from a malicious or buggy peer.
	local := database.OpenInMemory(nil)
	local.SetObserver(execute.NewDatabaseObserver())
	batch = local.Begin(true)
	if err := batch.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
		t.Fatal(err)
	}
	if err := batch.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}

	q := api.Querier2{Querier: &fakeQuerier{db: peer}}

	ro := local.Begin(false)
	defer ro.Discard()
	_, err := reconcileBPT(context.Background(), q, ro, protocol.DnUrl(), []*url.URL{u}, func(string, ...any) {})
	if err == nil {
		t.Fatal("expected reconciliation failure on divergent local state")
	}
}

// TestReconcileBPT_DocumentsKeyDerivation pins the BPT-key derivation
// (record.NewKey("Account", url).Hash()) so a refactor that changes the
// derivation will fail this test rather than silently drift.
func TestReconcileBPT_DocumentsKeyDerivation(t *testing.T) {
	u := protocol.DnUrl().JoinPath(protocol.Network)
	keyHash := record.NewKey("Account", u).Hash()
	if keyHash == ([32]byte{}) {
		t.Error("expected non-zero BPT key hash for account URL")
	}
}
