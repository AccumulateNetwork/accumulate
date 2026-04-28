// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package convergence

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestVerify_PullThenConverge is the v2 happy-path proof of life:
// reference DB, pull every account into a fresh DB through the v2
// puller, run convergence against the reference's BPT root. Pass
// means the launcher would promote BOOTING → ACTIVE here.
func TestVerify_PullThenConverge(t *testing.T) {
	urls := []*url.URL{
		protocol.DnUrl().JoinPath("alice"),
		protocol.DnUrl().JoinPath("bob"),
		protocol.DnUrl().JoinPath("carol"),
	}

	// Reference DB.
	ref := database.OpenInMemory(nil)
	ref.SetObserver(execute.NewDatabaseObserver())
	rb := ref.Begin(true)
	for _, u := range urls {
		if err := rb.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
			t.Fatal(err)
		}
		if err := rb.Account(u).MainChain().Inner().AddEntry([]byte("entry-padded-to-32-bytes-1234567"), false); err != nil {
			t.Fatal(err)
		}
	}
	if err := rb.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	if err := rb.Commit(); err != nil {
		t.Fatal(err)
	}
	rRO := ref.Begin(false)
	defer rRO.Discard()
	expected, err := rRO.GetBptRootHash()
	if err != nil {
		t.Fatal(err)
	}

	// Target DB: pull each account via the v2 puller, then converge.
	tgt := database.OpenInMemory(nil)
	tgt.SetObserver(execute.NewDatabaseObserver())
	tb := tgt.Begin(true)
	for _, u := range urls {
		if err := pull.Account(context.Background(), pull.NewDBSource(rRO), tb, u); err != nil {
			t.Fatalf("pull %s: %v", u, err)
		}
	}

	if err := Verify(tb, expected); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := tb.Commit(); err != nil {
		t.Fatal(err)
	}
}

// TestVerify_FailsOnTamperedState catches the case where local state
// diverges from the network. Pulls all accounts, then injects a
// tamper before running convergence; the equality check must fail.
func TestVerify_FailsOnTamperedState(t *testing.T) {
	u := protocol.DnUrl().JoinPath("alice")

	ref := database.OpenInMemory(nil)
	ref.SetObserver(execute.NewDatabaseObserver())
	rb := ref.Begin(true)
	if err := rb.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
		t.Fatal(err)
	}
	if err := rb.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	if err := rb.Commit(); err != nil {
		t.Fatal(err)
	}
	rRO := ref.Begin(false)
	defer rRO.Discard()
	expected, err := rRO.GetBptRootHash()
	if err != nil {
		t.Fatal(err)
	}

	// Pull, then tamper: add a chain entry that wasn't in the ref.
	tgt := database.OpenInMemory(nil)
	tgt.SetObserver(execute.NewDatabaseObserver())
	tb := tgt.Begin(true)
	if err := pull.Account(context.Background(), pull.NewDBSource(rRO), tb, u); err != nil {
		t.Fatal(err)
	}
	tamper := make([]byte, 32)
	tamper[0] = 0xff
	if err := tb.Account(u).MainChain().Inner().AddEntry(tamper, false); err != nil {
		t.Fatal(err)
	}

	err = Verify(tb, expected)
	if err == nil {
		t.Fatal("Verify should fail when local state is tampered")
	}
	if !errors.Is(err, ErrMismatch) {
		t.Errorf("err = %v, want ErrMismatch", err)
	}
}

// TestVerify_RejectsZeroAnchor is a guard rail: a zero anchor almost
// certainly means the header walk produced no terminal. Verifying
// against zero would silently pass any empty BPT. Refuse.
func TestVerify_RejectsZeroAnchor(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(execute.NewDatabaseObserver())
	b := db.Begin(true)
	defer b.Discard()
	err := Verify(b, [32]byte{})
	if err == nil {
		t.Fatal("Verify should reject a zero anchor")
	}
}

// TestVerify_NilBatch is a basic input check.
func TestVerify_NilBatch(t *testing.T) {
	if err := Verify(nil, [32]byte{0xab}); err == nil {
		t.Error("expected error for nil batch")
	}
}
