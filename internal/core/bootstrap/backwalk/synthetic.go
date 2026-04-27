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

// signerForTransaction returns the URL we should use as the signer when
// verifying user signatures on `txn`. For most user-signed transactions
// the signer's keybook owns the principal account (the principal's
// authority list points to its keybook). The principal's URL itself is
// not necessarily a key page; it might be a token account or identity.
//
// For tonight's slice the simplification is: principal IS the signer
// (works for transactions whose principal is itself a keypage, like
// UpdateKeyPage on a page). External-signer flows (a parent identity
// signs for a child account) require following the principal's
// AccountAuth — deferred to a follow-up.
func signerForTransaction(txn *protocol.Transaction) (*url.URL, error) {
	if txn == nil || txn.Header.Principal == nil {
		return nil, fmt.Errorf("nil transaction or principal")
	}
	return txn.Header.Principal, nil
}
