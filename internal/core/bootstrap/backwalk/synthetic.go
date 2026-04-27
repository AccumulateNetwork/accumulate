// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package backwalk

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// SyntheticVerification holds the partial result of synthetic-tx
// verification. The Cause traversal completes structurally; the
// validator-quorum check on the anchoring transaction is deferred (a
// follow-up slice on this branch).
type SyntheticVerification struct {
	// Causes are the transaction IDs that caused this synthetic. For
	// well-formed synthetics there is at least one; for cross-partition
	// forwards the producing chain may yield multiple causes.
	Causes []*url.TxID

	// QuorumPending is true while the validator-quorum cryptographic
	// check on the anchoring transaction is not yet implemented. The
	// structural plumbing (Cause traversal) is complete; the caller
	// can record this in the proof artifact as "needs quorum check".
	QuorumPending bool
}

// verifySynthetic does the structural part of synthetic-tx verification:
// it confirms the transaction body type is synthetic and traces the
// Cause links to the producing transaction(s). The cryptographic check
// of the anchoring transaction's validator-quorum signature is deferred.
//
// Returns ErrNotSynthetic if the transaction is not in fact synthetic
// (caller misclassified). Otherwise returns a SyntheticVerification
// with Causes populated and QuorumPending=true.
func verifySynthetic(batch *database.Batch, txnHash [32]byte, txn *protocol.Transaction) (*SyntheticVerification, error) {
	if !txn.Body.Type().IsSynthetic() && !txn.Body.Type().IsSystem() {
		return nil, fmt.Errorf("backwalk: tx %x is not synthetic (type=%v)", txnHash[:8], txn.Body.Type())
	}

	// Trace the cause links recorded on the message.
	msg := batch.Message(txnHash)
	causeSet, err := msg.Cause().Get()
	if err != nil {
		return nil, fmt.Errorf("read causes for %x: %w", txnHash[:8], err)
	}

	return &SyntheticVerification{
		Causes:        causeSet,
		QuorumPending: true,
	}, nil
}

// signerForTransaction returns candidate signer URLs for verifying
// user signatures on `txn`. The first candidate is always the principal
// itself (correct for transactions whose principal is a keypage, like
// UpdateKeyPage on a page). Additional candidates follow the
// principal's AccountAuth: each Authority's URL plus its first key
// page (since signatures land at a specific keypage).
//
// The caller (verifyEntry) tries each candidate in order; the first
// candidate that has signatures recorded for this transaction is used.
func signerForTransaction(batch *database.Batch, txn *protocol.Transaction) ([]*url.URL, error) {
	if txn == nil || txn.Header.Principal == nil {
		return nil, fmt.Errorf("nil transaction or principal")
	}
	candidates := []*url.URL{txn.Header.Principal}

	// If the principal is itself a keypage, no further candidates.
	if _, _, ok := protocol.ParseKeyPageUrl(txn.Header.Principal); ok {
		return candidates, nil
	}

	// Otherwise look up the principal's AccountAuth and add each
	// authority's first key page. We surface multiple candidates so the
	// caller can try them all when discovering which keybook signed.
	auth, ok := loadAccountAuth(batch, txn.Header.Principal)
	if !ok {
		return candidates, nil
	}
	for _, entry := range auth.Authorities {
		if entry.Url == nil {
			continue
		}
		// Treat the authority URL as a keybook; add its page 1.
		candidates = append(candidates, protocol.FormatKeyPageUrl(entry.Url, 0))
	}
	return candidates, nil
}

// loadAccountAuth attempts to load the account at u and return its
// AccountAuth (the embedded authority list). Returns ok=false if the
// account isn't present locally or doesn't carry an AccountAuth.
func loadAccountAuth(batch *database.Batch, u *url.URL) (*protocol.AccountAuth, bool) {
	var acct protocol.Account
	if err := batch.Account(u).Main().GetAs(&acct); err != nil {
		return nil, false
	}
	// FullAccount-shaped accounts expose AccountAuth via GetAuth().
	type fullAcct interface {
		GetAuth() *protocol.AccountAuth
	}
	if fa, ok := acct.(fullAcct); ok {
		auth := fa.GetAuth()
		if auth != nil {
			return auth, true
		}
	}
	return nil, false
}
