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

	w := New(Options{PinnedGenesisHash: [32]byte{0x01}})
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

// TestWalk_NoEntries returns a clear error.
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
