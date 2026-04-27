// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package backwalk

import (
	"errors"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/keybookat"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ErrSyntheticPath is returned when a transaction needs the synthetic
// (validator-quorum) verification path, which is not yet implemented.
// Callers can fall back to skipping the entry or surface for visibility.
var ErrSyntheticPath = errors.New("backwalk: synthetic-tx verification path not yet implemented")

// ErrSignatureInvalid is returned when at least one signature on a
// user-signed transaction does not verify against the resolved keypage.
var ErrSignatureInvalid = errors.New("backwalk: signature invalid")

// ErrNoSignatures is returned when an expected user-signed transaction
// has no signatures recorded at the signer.
var ErrNoSignatures = errors.New("backwalk: no signatures found at signer")

// VerifyUserSignaturesAt verifies the user-keypage signatures on the
// transaction at txnHash. It loads the SignatureSetEntries stored at
// signerUrl for this transaction, looks up the corresponding signature
// messages, resolves the signer's keybook to its state at blockTime
// via keybookat.Resolve, and verifies each signature against the
// resolved keypage.
//
// Returns nil iff at least one signature verifies and none fail.
// Returns ErrSyntheticPath if the transaction is identified as
// synthetic (signatures are validator-quorum, not user-keypage).
//
// signerUrl should be the URL of the signing keypage (e.g.
// alice.acme/book/1). The keybook URL is derived as the parent.
func VerifyUserSignaturesAt(
	batch *database.Batch,
	txnHash [32]byte,
	signerUrl *url.URL,
	blockTime time.Time,
) error {
	// Load the transaction message itself (used for canonical message hash
	// during signature verification).
	var msg messaging.Message
	err := batch.Message(txnHash).Main().GetAs(&msg)
	if err != nil {
		return fmt.Errorf("load message %x: %w", txnHash[:8], err)
	}
	txMsg, ok := msg.(*messaging.TransactionMessage)
	if !ok {
		return fmt.Errorf("message %x is not a transaction (got %T)", txnHash[:8], msg)
	}
	txn := txMsg.Transaction
	if txn == nil {
		return fmt.Errorf("transaction body is nil for %x", txnHash[:8])
	}

	// Load the signature set for this transaction at the signer.
	entries, err := batch.Account(signerUrl).Transaction(txnHash).Signatures().Get()
	if err != nil {
		return fmt.Errorf("load signatures at %s: %w", signerUrl, err)
	}
	if len(entries) == 0 {
		return ErrNoSignatures
	}

	// Resolve the signer's keybook to its state at blockTime. The
	// signer URL is a keypage like alice.acme/book/N; the keybook is
	// the parent.
	bookUrl, _, ok := protocol.ParseKeyPageUrl(signerUrl)
	if !ok {
		return fmt.Errorf("signer %s is not a key-page URL", signerUrl)
	}
	resolved, err := keybookat.Resolve(batch, bookUrl, blockTime)
	if err != nil {
		return fmt.Errorf("resolve keybook %s @ %s: %w", bookUrl, blockTime.Format(time.RFC3339), err)
	}

	// Look up the signing keypage by URL.
	var page *protocol.KeyPage
	for _, p := range resolved.Pages {
		if p.Url.Equal(signerUrl) {
			page = p
			break
		}
	}
	if page == nil {
		return fmt.Errorf("signer page %s not in resolved keybook", signerUrl)
	}

	// Verify each signature.
	verified := 0
	for _, e := range entries {
		// Skip placeholder entries (zero-hash); should not normally
		// appear but be defensive.
		if e.Hash == ([32]byte{}) {
			continue
		}
		// Load the signature message.
		var sigMsg messaging.Message
		err := batch.Message(e.Hash).Main().GetAs(&sigMsg)
		if err != nil {
			return fmt.Errorf("load sig msg %x: %w", e.Hash[:8], err)
		}
		signedMsg, ok := sigMsg.(*messaging.SignatureMessage)
		if !ok {
			// Some entries may reference non-signature messages (e.g.,
			// authority-vote messages). Skip; they don't provide
			// keypage-level cryptographic verification.
			continue
		}
		sig := signedMsg.Signature
		if sig == nil {
			return fmt.Errorf("nil signature in message %x", e.Hash[:8])
		}

		// Skip system signatures (validator-quorum / receipt / internal).
		// Those are handled by the synthetic-tx verification path,
		// not user-keypage verification.
		if sig.Type().IsSystem() {
			continue
		}

		// Only KeySignature carries the cryptographic Verify method we
		// need plus the public-key accessors for cross-checking against
		// the page.
		ksig, ok := sig.(protocol.KeySignature)
		if !ok {
			// Authority votes, delegated signatures, etc. — outside the
			// keypage verification path. Skip silently; if no other
			// entries verify, the function returns ErrNoSignatures.
			continue
		}

		// Locate the key in the resolved page.
		if int(e.KeyIndex) >= len(page.Keys) {
			return fmt.Errorf("key index %d out of range (page has %d keys)", e.KeyIndex, len(page.Keys))
		}
		expectedKey := page.Keys[e.KeyIndex]

		// Cross-check the signature's public-key hash matches the page entry.
		if got := ksig.GetPublicKeyHash(); !bytesEqual(got, expectedKey.PublicKeyHash) {
			return fmt.Errorf("%w: signature key hash %x does not match page entry %x",
				ErrSignatureInvalid, got, expectedKey.PublicKeyHash)
		}

		// Cryptographic verification.
		if !ksig.Verify(nil, txn) {
			return fmt.Errorf("%w: signature %x at key index %d",
				ErrSignatureInvalid, e.Hash[:8], e.KeyIndex)
		}

		verified++
	}

	if verified == 0 {
		return ErrNoSignatures
	}
	return nil
}

func bytesEqual(a, b []byte) bool {
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
