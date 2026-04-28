// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestAccount_RoundTripVsCompletenessContract is the load-bearing
// test for the v2 data phase. It populates a reference database with
// a non-trivial account state, runs Account against a fresh database
// using a DBSource over the reference, and verifies the leaf hash
// matches byte-for-byte.
//
// This is the same contract pinned in the completeness package, but
// with the Account function under test in the middle instead of an
// inline copy. If this test passes, the v2 puller can reproduce any
// network leaf the completeness suite covers.
func TestAccount_RoundTripVsCompletenessContract(t *testing.T) {
	u := protocol.DnUrl().JoinPath("alice")

	// Reference DB with the full surface.
	src := database.OpenInMemory(nil)
	src.SetObserver(execute.NewDatabaseObserver())
	{
		b := src.Begin(true)
		if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
			t.Fatal(err)
		}
		if err := b.Account(u).MainChain().Inner().AddEntry([]byte("entry1-padded-to-32-bytes-12345!"), false); err != nil {
			t.Fatal(err)
		}
		if err := b.Account(u).MainChain().Inner().AddEntry([]byte("entry2-padded-to-32-bytes-12345!"), false); err != nil {
			t.Fatal(err)
		}
		if err := b.Account(u).Directory().Add(u.JoinPath("child1")); err != nil {
			t.Fatal(err)
		}
		if err := b.Account(u).Directory().Add(u.JoinPath("child2")); err != nil {
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
	want, err := srcRO.Account(u).Hash()
	if err != nil {
		t.Fatal(err)
	}

	// Pull into a fresh DB through the puller under test.
	dst := database.OpenInMemory(nil)
	dst.SetObserver(execute.NewDatabaseObserver())
	dstBatch := dst.Begin(true)
	if err := Account(context.Background(), NewDBSource(srcRO), dstBatch, u); err != nil {
		t.Fatalf("pull.Account: %v", err)
	}
	if err := dstBatch.Commit(); err != nil {
		t.Fatal(err)
	}

	dstRO := dst.Begin(false)
	defer dstRO.Discard()
	got, err := dstRO.Account(u).Hash()
	if err != nil {
		t.Fatal(err)
	}

	if got != want {
		t.Fatalf("leaf hash mismatch after pull:\n  want: %x\n  got:  %x\nthe puller is missing a field; check internal/core/execute/internal/bpt_prod.go", want, got)
	}
}

// TestAccount_MultipleAccounts_SameRoot verifies that pulling two
// accounts in sequence produces the same final BPT root as the
// reference database holding both. This is the multi-account
// generalization the bootstrap pipeline relies on.
func TestAccount_MultipleAccounts_SameRoot(t *testing.T) {
	urls := []string{"acc://dn.acme/alice", "acc://dn.acme/bob", "acc://dn.acme/carol"}

	src := database.OpenInMemory(nil)
	src.SetObserver(execute.NewDatabaseObserver())
	srcBatch := src.Begin(true)
	for _, us := range urls {
		u := mustParse(t, us)
		if err := srcBatch.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
			t.Fatal(err)
		}
		if err := srcBatch.Account(u).MainChain().Inner().AddEntry([]byte("entry-padded-to-32-bytes-1234567"), false); err != nil {
			t.Fatal(err)
		}
	}
	if err := srcBatch.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	if err := srcBatch.Commit(); err != nil {
		t.Fatal(err)
	}
	srcRO := src.Begin(false)
	defer srcRO.Discard()
	wantRoot, err := srcRO.GetBptRootHash()
	if err != nil {
		t.Fatal(err)
	}

	// Pull every account into a fresh DB through the puller.
	dst := database.OpenInMemory(nil)
	dst.SetObserver(execute.NewDatabaseObserver())
	dstBatch := dst.Begin(true)
	for _, us := range urls {
		if err := Account(context.Background(), NewDBSource(srcRO), dstBatch, mustParse(t, us)); err != nil {
			t.Fatalf("pull %s: %v", us, err)
		}
	}
	if err := dstBatch.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	if err := dstBatch.Commit(); err != nil {
		t.Fatal(err)
	}

	dstRO := dst.Begin(false)
	defer dstRO.Discard()
	gotRoot, err := dstRO.GetBptRootHash()
	if err != nil {
		t.Fatal(err)
	}

	if gotRoot != wantRoot {
		t.Fatalf("BPT root mismatch after multi-account pull:\n  want: %x\n  got:  %x", wantRoot, gotRoot)
	}
}

func mustParse(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
