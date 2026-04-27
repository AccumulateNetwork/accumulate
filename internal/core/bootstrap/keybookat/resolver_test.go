// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keybookat

import (
	"errors"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// nullObserver satisfies the database observer interface without affecting BPT.
type nullObserver struct{}

func (nullObserver) DidChangeAccount(*database.Batch, *database.Account) (hash.Hasher, error) {
	return nil, nil
}

func mustParse(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return u
}

func TestResolve_NoMutations_ReturnsCurrentState(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(nullObserver{})

	bookUrl := mustParse(t, "alice.acme/book")
	pageUrl := protocol.FormatKeyPageUrl(bookUrl, 0)

	// Set up a keybook with one page containing one key, both stored
	// directly with no main-chain history.
	keyHash := [32]byte{0xab}
	batch := db.Begin(true)
	defer batch.Discard()

	book := &protocol.KeyBook{Url: bookUrl, PageCount: 1}
	if err := batch.Account(bookUrl).Main().Put(book); err != nil {
		t.Fatal(err)
	}

	page := &protocol.KeyPage{Url: pageUrl, Version: 1}
	page.AddKeySpec(&protocol.KeySpec{PublicKeyHash: keyHash[:]})
	if err := batch.Account(pageUrl).Main().Put(page); err != nil {
		t.Fatal(err)
	}

	res, err := Resolve(batch, bookUrl, time.Now())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(res.Pages))
	}
	if len(res.Pages[0].Keys) != 1 {
		t.Fatalf("expected 1 key on page, got %d", len(res.Pages[0].Keys))
	}
	if got := res.Pages[0].Keys[0].PublicKeyHash; len(got) != len(keyHash) || !equalBytes(got, keyHash[:]) {
		t.Fatalf("key hash mismatch: got %x, want %x", got, keyHash[:])
	}
}

func TestResolve_NilUrl(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(nullObserver{})

	batch := db.Begin(false)
	defer batch.Discard()

	_, err := Resolve(batch, nil, time.Now())
	if err == nil {
		t.Fatal("expected error for nil url")
	}
}

func TestResolve_BookWithZeroPages(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(nullObserver{})

	bookUrl := mustParse(t, "alice.acme/empty")
	batch := db.Begin(true)
	defer batch.Discard()

	book := &protocol.KeyBook{Url: bookUrl, PageCount: 0}
	if err := batch.Account(bookUrl).Main().Put(book); err != nil {
		t.Fatal(err)
	}

	res, err := Resolve(batch, bookUrl, time.Now())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Pages) != 0 {
		t.Fatalf("expected 0 pages, got %d", len(res.Pages))
	}
}

func TestReplayPage_AppliesCreateKeyPage(t *testing.T) {
	// Verify that replayPage correctly applies a CreateKeyPage transaction
	// to build initial page state. This exercises the algorithm directly
	// without setting up the full chain plumbing — we call replayPage's
	// applyTransactionToPage helper directly via a synthetic transaction.
	bookUrl := mustParse(t, "bob.acme/book")
	pageUrl := protocol.FormatKeyPageUrl(bookUrl, 0)

	page := &protocol.KeyPage{Url: pageUrl}
	book := &protocol.KeyBook{Url: bookUrl, PageCount: 1}

	// Build a CreateKeyPage transaction with two initial keys.
	hash1 := []byte{0x01, 0x02, 0x03}
	hash2 := []byte{0x04, 0x05, 0x06}
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: bookUrl},
		Body: &protocol.CreateKeyPage{
			Keys: []*protocol.KeySpecParams{
				{KeyHash: hash1},
				{KeyHash: hash2},
			},
		},
	}

	if err := applyTransactionToPage(page, book, txn); err != nil {
		t.Fatalf("apply CreateKeyPage: %v", err)
	}
	if len(page.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(page.Keys))
	}
	if page.Version != 1 {
		t.Errorf("expected Version=1, got %d", page.Version)
	}
}

func TestReplayPage_AppliesUpdateKeyPage(t *testing.T) {
	bookUrl := mustParse(t, "carol.acme/book")
	pageUrl := protocol.FormatKeyPageUrl(bookUrl, 0)

	page := &protocol.KeyPage{Url: pageUrl, Version: 1}
	page.AddKeySpec(&protocol.KeySpec{PublicKeyHash: []byte{0xaa}})
	book := &protocol.KeyBook{Url: bookUrl, PageCount: 1}

	// UpdateKeyPage adding a second key.
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: pageUrl},
		Body: &protocol.UpdateKeyPage{
			Operation: []protocol.KeyPageOperation{
				&protocol.AddKeyOperation{
					Entry: protocol.KeySpecParams{KeyHash: []byte{0xbb}},
				},
			},
		},
	}

	if err := applyTransactionToPage(page, book, txn); err != nil {
		t.Fatalf("apply UpdateKeyPage: %v", err)
	}
	if len(page.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(page.Keys))
	}
	if page.Version != 2 {
		t.Errorf("expected Version=2 after update, got %d", page.Version)
	}
}

func TestReplayPage_RejectsUnsupportedTxType(t *testing.T) {
	bookUrl := mustParse(t, "dave.acme/book")
	pageUrl := protocol.FormatKeyPageUrl(bookUrl, 0)

	page := &protocol.KeyPage{Url: pageUrl}
	book := &protocol.KeyBook{Url: bookUrl, PageCount: 1}

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: pageUrl},
		Body:   &protocol.UpdateKey{NewKeyHash: []byte{0xcc}},
	}

	err := applyTransactionToPage(page, book, txn)
	if !errors.Is(err, ErrUnsupportedTxType) {
		t.Fatalf("expected ErrUnsupportedTxType, got %v", err)
	}
}

// Sanity check: the loadTransaction helper short-circuits gracefully
// for messages that aren't transactions.
func TestLoadTransaction_NonTransactionMessageReturnsNil(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(nullObserver{})
	batch := db.Begin(true)
	defer batch.Discard()

	hash := [32]byte{0x99}
	// Store a SignatureMessage (not a TransactionMessage) at this hash.
	sigMsg := &messaging.SignatureMessage{
		Signature: &protocol.ED25519Signature{},
	}
	if err := batch.Message(hash).Main().Put(sigMsg); err != nil {
		t.Fatal(err)
	}

	tx, err := loadTransaction(batch, hash)
	if err != nil {
		t.Fatalf("loadTransaction: %v", err)
	}
	if tx != nil {
		t.Fatalf("expected nil transaction for non-tx message, got %v", tx)
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
