// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package completeness pins the surface of fields the production
// observer hashes into a BPT leaf, so the v2 bootstrap puller can
// guarantee local reconstruction matches the network byte-for-byte.
//
// The bootstrap design's convergence step is "local UpdateBPT() root
// equals trusted current-block StateTreeAnchor." That equality only
// holds if every field the production observer reads is round-tripped
// faithfully into the launcher's database. This package's tests pin
// which fields matter; the puller (next phase) is defined by those
// fields.
//
// Reference: internal/core/execute/internal/bpt_prod.go observedAccount
// hashState. Any change there must surface here as a test diff.
package completeness

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// dbWithObserver returns an in-memory database wired to the production
// observer — same one the network uses on every commit.
func dbWithObserver(t *testing.T) *database.Database {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(execute.NewDatabaseObserver())
	return db
}

// hashWithMain populates only Main on the account at u and returns the
// computed leaf hash. Helper for the field-sensitivity test.
func hashWithMain(t *testing.T, db *database.Database, u *url.URL) [32]byte {
	t.Helper()
	batch := db.Begin(true)
	defer batch.Discard()
	if err := batch.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
		t.Fatal(err)
	}
	h, err := batch.Account(u).Hash()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestFieldSensitivity_MainAlone — baseline: a Main-only account
// produces a non-zero, deterministic leaf hash. If this regresses,
// nothing else in the bootstrap will work.
func TestFieldSensitivity_MainAlone(t *testing.T) {
	u := protocol.DnUrl().JoinPath("alice")

	db1 := dbWithObserver(t)
	h1 := hashWithMain(t, db1, u)

	db2 := dbWithObserver(t)
	h2 := hashWithMain(t, db2, u)

	if h1 == ([32]byte{}) {
		t.Fatal("expected non-zero leaf hash for Main-only account")
	}
	if h1 != h2 {
		t.Fatalf("Main-only hashing is non-deterministic: %x vs %x", h1, h2)
	}
}

// TestFieldSensitivity_AddingChainEntry_ChangesHash documents that
// chain entries are part of the BPT leaf. Adding a main-chain entry
// changes the leaf hash. The bootstrap puller MUST round-trip chain
// entries; pulling only Main is insufficient.
func TestFieldSensitivity_AddingChainEntry_ChangesHash(t *testing.T) {
	u := protocol.DnUrl().JoinPath("alice")

	db := dbWithObserver(t)
	mainOnly := hashWithMain(t, db, u)

	// Add a main-chain entry.
	batch := db.Begin(true)
	entry := make([]byte, 32)
	entry[0] = 0xab
	if err := batch.Account(u).MainChain().Inner().AddEntry(entry, false); err != nil {
		t.Fatal(err)
	}
	h, err := batch.Account(u).Hash()
	if err != nil {
		t.Fatal(err)
	}
	batch.Discard()

	if h == mainOnly {
		t.Error("adding a chain entry should change the leaf hash, but it did not — chains are not in the leaf surface")
	}
}

// TestFieldSensitivity_AddingDirectory_ChangesHash documents that the
// Directory list (sub-accounts of an ADI) is part of the BPT leaf. The
// puller MUST round-trip Directory entries.
func TestFieldSensitivity_AddingDirectory_ChangesHash(t *testing.T) {
	u := protocol.DnUrl().JoinPath("alice")
	child := u.JoinPath("data")

	db := dbWithObserver(t)
	mainOnly := hashWithMain(t, db, u)

	batch := db.Begin(true)
	if err := batch.Account(u).Directory().Add(child); err != nil {
		t.Fatal(err)
	}
	h, err := batch.Account(u).Hash()
	if err != nil {
		t.Fatal(err)
	}
	batch.Discard()

	if h == mainOnly {
		t.Error("adding a directory entry should change the leaf hash, but it did not — Directory is not in the leaf surface")
	}
}

// TestFieldSensitivity_AddingPending_ChangesHash documents that the
// Pending transaction list is part of the BPT leaf.
func TestFieldSensitivity_AddingPending_ChangesHash(t *testing.T) {
	u := protocol.DnUrl().JoinPath("alice")

	db := dbWithObserver(t)
	mainOnly := hashWithMain(t, db, u)

	batch := db.Begin(true)
	var txnHash [32]byte
	txnHash[0] = 0xcd
	txid := u.WithTxID(txnHash)
	if err := batch.Account(u).Pending().Add(txid); err != nil {
		t.Fatal(err)
	}
	h, err := batch.Account(u).Hash()
	if err != nil {
		t.Fatal(err)
	}
	batch.Discard()

	if h == mainOnly {
		t.Error("adding a pending transaction should change the leaf hash, but it did not — Pending is not in the leaf surface")
	}
}

// TestRoundTrip_DataAccount_FullSurface is the reference for the
// puller's required surface. Database A holds an account with every
// field a v2 puller will need to round-trip. Database B copies those
// fields via the same APIs the puller will use. Hash() must match.
//
// If this test fails, the puller cannot reconstruct the BPT and the
// whole v2 design's convergence step breaks. If a new field is added
// to the production observer, this test fails and forces an update
// to the puller.
func TestRoundTrip_DataAccount_FullSurface(t *testing.T) {
	u := protocol.DnUrl().JoinPath("alice")

	// --- Reference DB: populate the full surface.
	src := dbWithObserver(t)
	{
		b := src.Begin(true)
		acct := &protocol.DataAccount{Url: u}
		if err := b.Account(u).Main().Put(acct); err != nil {
			t.Fatal(err)
		}
		// Chains.
		if err := b.Account(u).MainChain().Inner().AddEntry([]byte("entry1-padded-to-32-bytes-12345!"), false); err != nil {
			t.Fatal(err)
		}
		// Directory entry.
		if err := b.Account(u).Directory().Add(u.JoinPath("child")); err != nil {
			t.Fatal(err)
		}
		// Pending.
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

	// --- Target DB: round-trip via the same primitives the puller
	// will use to populate state from the network. We're simulating
	// "I read these fields from a peer and wrote them locally."
	dst := dbWithObserver(t)
	{
		b := dst.Begin(true)

		// Main.
		var acct protocol.Account
		err := srcRO.Account(u).Main().GetAs(&acct)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.Account(u).Main().Put(acct); err != nil {
			t.Fatal(err)
		}

		// Chains: enumerate, then for each, replay every entry.
		chains, err := srcRO.Account(u).Chains().Get()
		if err != nil {
			t.Fatal(err)
		}
		for _, cm := range chains {
			srcChain, err := srcRO.Account(u).ChainByName(cm.Name)
			if err != nil {
				t.Fatal(err)
			}
			dstChain, err := b.Account(u).ChainByName(cm.Name)
			if err != nil {
				t.Fatal(err)
			}
			head, err := srcChain.Head().Get()
			if err != nil {
				t.Fatal(err)
			}
			for i := int64(0); i < head.Count; i++ {
				e, err := srcChain.Entry(i)
				if err != nil {
					t.Fatal(err)
				}
				if err := dstChain.Inner().AddEntry(e, false); err != nil {
					t.Fatal(err)
				}
			}
		}

		// Directory.
		dirs, err := srcRO.Account(u).Directory().Get()
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range dirs {
			if err := b.Account(u).Directory().Add(d); err != nil {
				t.Fatal(err)
			}
		}

		// Pending.
		pending, err := srcRO.Account(u).Pending().Get()
		if err != nil {
			t.Fatal(err)
		}
		for _, txid := range pending {
			if err := b.Account(u).Pending().Add(txid); err != nil {
				t.Fatal(err)
			}
		}

		if err := b.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	dstRO := dst.Begin(false)
	defer dstRO.Discard()
	got, err := dstRO.Account(u).Hash()
	if err != nil {
		t.Fatal(err)
	}

	if got != want {
		t.Fatalf("round-trip leaf hash mismatch:\n  want: %x\n  got:  %x\nthe puller's field surface is incomplete; check the production observer in bpt_prod.go", want, got)
	}
}
