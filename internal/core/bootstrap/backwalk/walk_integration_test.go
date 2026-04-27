// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package backwalk

import (
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestWalk_GenesisTransaction_Terminates exercises Walk against a
// synthetic chain whose only entry is a SystemGenesis transaction. Walk
// should:
//   - Return without error.
//   - Mark the resulting VerifiedEntry as GenesisTerm=true.
//   - Memoize the entry.
func TestWalk_GenesisTransaction_Terminates(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(nullObserver{})
	batch := db.Begin(true)
	defer batch.Discard()

	bookUrl := mustParse(t, "system.acme/book")
	pageUrl := protocol.FormatKeyPageUrl(bookUrl, 0)

	// Set up the keybook and page (used by signer-resolution paths).
	if err := batch.Account(bookUrl).Main().Put(&protocol.KeyBook{Url: bookUrl, PageCount: 1}); err != nil {
		t.Fatal(err)
	}
	if err := batch.Account(pageUrl).Main().Put(&protocol.KeyPage{Url: pageUrl, Version: 1}); err != nil {
		t.Fatal(err)
	}

	// Store a SystemGenesis transaction.
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: pageUrl},
		Body:   &protocol.SystemGenesis{},
	}
	txMsg := &messaging.TransactionMessage{Transaction: txn}
	var txnHash [32]byte
	copy(txnHash[:], txn.GetHash())
	if err := batch.Message(txnHash).Main().Put(txMsg); err != nil {
		t.Fatal(err)
	}

	// Add the transaction hash as the page's only main-chain entry.
	if err := batch.Account(pageUrl).MainChain().Inner().AddEntry(txnHash[:], false); err != nil {
		t.Fatal(err)
	}

	w := New(Options{PinnedGenesisHash: txnHash})
	ve, err := w.Walk(batch, pageUrl, time.Now())
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if ve == nil {
		t.Fatal("expected non-nil VerifiedEntry")
	}
	if !ve.GenesisTerm {
		t.Errorf("expected GenesisTerm=true (SystemGenesis terminates), got %+v", ve)
	}
	if !ve.Synthetic {
		// SystemGenesis is system class, treated as synthetic for the
		// back-walker's two-rule classification.
		t.Errorf("expected Synthetic=true for SystemGenesis, got %+v", ve)
	}
	if ve.Account == nil || !ve.Account.Equal(pageUrl) {
		t.Errorf("Account = %v, want %v", ve.Account, pageUrl)
	}
	if ve.TxHash != txnHash {
		t.Errorf("TxHash mismatch: got %x, want %x", ve.TxHash[:8], txnHash[:8])
	}

	// Memoization should now contain this entry; second Walk hits cache.
	if w.MemoSize() != 1 {
		t.Errorf("expected 1 memo, got %d", w.MemoSize())
	}
}

// TestWalk_MultiEntryChain exercises Walk against a chain with multiple
// entries. Walk should classify each, memoize each, and return the
// earliest in-window entry as the terminal.
//
// We use two SystemGenesis entries (synthetic-class) rather than mixing
// types, since user-signed entries require setting up real signature
// fixtures. This proves the multi-entry orchestration and memoization
// without the heavier signature-construction machinery.
func TestWalk_MultiEntryChain(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(nullObserver{})
	batch := db.Begin(true)
	defer batch.Discard()

	bookUrl := mustParse(t, "system.acme/book")
	pageUrl := protocol.FormatKeyPageUrl(bookUrl, 0)

	if err := batch.Account(bookUrl).Main().Put(&protocol.KeyBook{Url: bookUrl, PageCount: 1}); err != nil {
		t.Fatal(err)
	}
	if err := batch.Account(pageUrl).Main().Put(&protocol.KeyPage{Url: pageUrl, Version: 1}); err != nil {
		t.Fatal(err)
	}

	mkSysGen := func(memo string) [32]byte {
		txn := &protocol.Transaction{
			Header: protocol.TransactionHeader{Principal: pageUrl, Memo: memo},
			Body:   &protocol.SystemGenesis{},
		}
		txMsg := &messaging.TransactionMessage{Transaction: txn}
		var hashArr [32]byte
		copy(hashArr[:], txn.GetHash())
		if err := batch.Message(hashArr).Main().Put(txMsg); err != nil {
			t.Fatal(err)
		}
		if err := batch.Account(pageUrl).MainChain().Inner().AddEntry(hashArr[:], false); err != nil {
			t.Fatal(err)
		}
		return hashArr
	}
	hash1 := mkSysGen("first")
	mkSysGen("second")

	// Pin to hash1 — the earliest in-window entry — so the pinned-hash
	// cross-check accepts it as the genesis terminator.
	w := New(Options{PinnedGenesisHash: hash1})
	earliest, err := w.Walk(batch, pageUrl, time.Now())
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if earliest == nil {
		t.Fatal("expected non-nil earliest entry")
	}
	// The earliest in-window entry is main-chain index 0 = hash1.
	if earliest.TxHash != hash1 {
		t.Errorf("earliest TxHash mismatch: got %x, want %x (hash1)", earliest.TxHash[:8], hash1[:8])
	}
	if !earliest.GenesisTerm {
		t.Error("expected GenesisTerm=true on earliest SystemGenesis")
	}
	// With both entries sharing the same (zero) block time — index chain
	// isn't populated in this fixture — they collide on the memo key
	// (account, block_time). The walker's last write wins, so we expect
	// 1 memo entry containing the earliest-walked entry. A real chain
	// would have distinct block times via the main-index chain and yield
	// 2 memos.
	if w.MemoSize() != 1 {
		t.Errorf("expected 1 memoization (block-time collision), got %d", w.MemoSize())
	}
}

// TestWalk_EmptyChain returns a clear error.
func TestWalk_EmptyChain(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(nullObserver{})
	batch := db.Begin(true)
	defer batch.Discard()

	pageUrl := mustParse(t, "alice.acme/page/1")
	if err := batch.Account(pageUrl).Main().Put(&protocol.KeyPage{Url: pageUrl}); err != nil {
		t.Fatal(err)
	}

	w := New(Options{PinnedGenesisHash: [32]byte{0x01}})
	_, err := w.Walk(batch, pageUrl, time.Now())
	if err == nil || !contains(err.Error(), "no main-chain entries") {
		t.Fatalf("expected 'no main-chain entries' error, got %v", err)
	}
}
