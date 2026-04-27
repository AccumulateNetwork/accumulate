// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package backwalk

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

// TestVerify_RejectsSyntheticTx exercises the early-exit when a transaction
// at txnHash is not a TransactionMessage (e.g., a system message landed
// at the same hash, which shouldn't happen in practice but is a defensive
// check).
func TestVerify_NotATransaction(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(nullObserver{})
	batch := db.Begin(true)
	defer batch.Discard()

	hash := [32]byte{0x01}
	// Store a SignatureMessage at txnHash (not a transaction).
	msg := &messaging.SignatureMessage{Signature: &protocol.ED25519Signature{}}
	if err := batch.Message(hash).Main().Put(msg); err != nil {
		t.Fatal(err)
	}

	signer := mustParse(t, "alice.acme/book/1")
	err := VerifyUserSignaturesAt(batch, hash, signer, time.Now())
	if err == nil || !contains(err.Error(), "not a transaction") {
		t.Fatalf("expected 'not a transaction' error, got %v", err)
	}
}

// TestVerify_NoSignaturesAtSigner — the signer has no SignatureSet for
// this transaction, so verification returns ErrNoSignatures.
func TestVerify_NoSignaturesAtSigner(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(nullObserver{})
	batch := db.Begin(true)
	defer batch.Discard()

	signerUrl := mustParse(t, "alice.acme/book/1")
	bookUrl := mustParse(t, "alice.acme/book")

	// Set up a minimal keybook + page so Resolve succeeds.
	book := &protocol.KeyBook{Url: bookUrl, PageCount: 1}
	if err := batch.Account(bookUrl).Main().Put(book); err != nil {
		t.Fatal(err)
	}
	page := &protocol.KeyPage{Url: signerUrl, Version: 1}
	if err := batch.Account(signerUrl).Main().Put(page); err != nil {
		t.Fatal(err)
	}

	// Store a transaction message.
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: signerUrl},
		Body:   &protocol.UpdateKeyPage{},
	}
	txMsg := &messaging.TransactionMessage{Transaction: txn}
	txnHash := txn.GetHash()
	var hashArr [32]byte
	copy(hashArr[:], txnHash)
	if err := batch.Message(hashArr).Main().Put(txMsg); err != nil {
		t.Fatal(err)
	}

	err := VerifyUserSignaturesAt(batch, hashArr, signerUrl, time.Now())
	if !errors.Is(err, ErrNoSignatures) {
		t.Fatalf("expected ErrNoSignatures, got %v", err)
	}
}

// TestVerify_SignerNotKeyPageURL — passing a non-keypage URL as signer
// should fail with a clear error.
func TestVerify_SignerNotKeyPageURL(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(nullObserver{})
	batch := db.Begin(true)
	defer batch.Discard()

	notAPage := mustParse(t, "alice.acme/book") // book, not /N

	// Store a tx so the message lookup succeeds.
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: notAPage},
		Body:   &protocol.UpdateKeyPage{},
	}
	txMsg := &messaging.TransactionMessage{Transaction: txn}
	txnHash := txn.GetHash()
	var hashArr [32]byte
	copy(hashArr[:], txnHash)
	if err := batch.Message(hashArr).Main().Put(txMsg); err != nil {
		t.Fatal(err)
	}
	// Add a signature entry so we get past the no-signatures check.
	sigEntry := &database.SignatureSetEntry{
		KeyIndex: 0,
		Version:  1,
		Hash:     [32]byte{0xee},
	}
	if err := batch.Account(notAPage).Transaction(hashArr).Signatures().Add(sigEntry); err != nil {
		t.Fatal(err)
	}

	err := VerifyUserSignaturesAt(batch, hashArr, notAPage, time.Now())
	if err == nil || !contains(err.Error(), "key-page URL") {
		t.Fatalf("expected 'key-page URL' error, got %v", err)
	}
}

// End-to-end signature verification with a real ED25519 signature
// requires reproducing the protocol's signing-hash computation
// faithfully — better exercised through the simulator harness once that
// test infrastructure lands. The structural tests above cover the
// plumbing (message lookup, sig-set lookup, keybook resolution, signer
// URL parsing); the cryptographic Verify call is well-covered by the
// existing tests in protocol/signature_test.go.

func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
