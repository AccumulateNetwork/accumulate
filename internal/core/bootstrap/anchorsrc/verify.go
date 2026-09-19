// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package anchorsrc

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// verify returns nil iff a quorum of the producing partition's validators
// signed this anchor transaction.
//
// What is counted is DISTINCT members of the set, and only after their
// signatures have been checked. Neither half is optional:
//
//   - Membership alone is worthless here, because the query API manufactures
//     signature records. loadTransactionSignaturesV1 (internal/api/v3/load.go)
//     appends a messaging.BlockAnchor carrying an ED25519Signature with only
//     PublicKey set, one per status.AnchorSigners, and those are a peer's word
//     about who signed. They carry no signature bytes, so they fail Verify —
//     which is the only reason they do not reach the threshold on their own.
//
//   - Distinctness matters because a second copy from one validator is no
//     second signature. The executor says the same thing in its own words
//     (msg_block_anchor.go, "A second copy from the same validator is no
//     second signature").
//
// The set is the one of the signature's time: a signature declares the
// network definition version it was made under, and the version is hashed
// into the signature, so it names a set rather than asserting one. A version
// the node's walk has not reached is refused (see Authority).
func (s *Source) verify(producer string, rec *api.MessageRecord[*messaging.TransactionMessage]) error {
	if rec.Sequence == nil {
		return errors.BadRequest.With("the anchor record carries no sequenced message, so there is nothing a signature covers")
	}
	if rec.Signatures == nil || len(rec.Signatures.Records) == 0 {
		return errors.Unauthenticated.With("the anchor carries no signatures")
	}

	// A BlockAnchor signature covers the SequencedMessage wrapping the
	// transaction, not the bare transaction — the executor's own pattern
	// (execute/v2/block/msg_block_anchor.go, checkSignature).
	forms := signedForms(rec)

	signed := map[[32]byte]bool{}
	var threshold uint64
	var haveSet bool
	var refusals []error

	for _, set := range rec.Signatures.Records {
		if set == nil || set.Signatures == nil {
			continue
		}
		for _, sigMsg := range set.Signatures.Records {
			if sigMsg == nil || sigMsg.Message == nil {
				continue
			}
			var keySig protocol.KeySignature
			switch m := sigMsg.Message.(type) {
			case *messaging.BlockAnchor:
				keySig = m.Signature
			case *messaging.SignatureMessage:
				keySig, _ = m.Signature.(protocol.KeySignature)
			}
			if keySig == nil {
				continue
			}

			// The set of the signature's time, named by the signature.
			vs, err := s.Authority.SetFor(producer, keySig.GetSignerVersion())
			if err != nil {
				refusals = append(refusals, err)
				continue
			}
			threshold, haveSet = vs.Threshold, true

			if !vs.MaySign(keySig.GetPublicKeyHash()) {
				continue
			}
			if !verifiesAny(keySig, forms) {
				continue
			}
			signed[*(*[32]byte)(keySig.GetPublicKeyHash())] = true
		}
	}

	if !haveSet {
		if len(refusals) > 0 {
			return errors.Unauthenticated.WithFormat("no validator set covers this anchor's signatures: %w", refusals[0])
		}
		return errors.Unauthenticated.With("the anchor carries no validator signature")
	}
	if threshold == 0 {
		return errors.Unauthenticated.WithFormat("%s has a zero validator threshold", producer)
	}
	if uint64(len(signed)) < threshold {
		return errors.Unauthenticated.WithFormat(
			"only %d of %s's validators signed, and %d are required", len(signed), producer, threshold)
	}
	return nil
}

// signedForms is what a validator may have signed over.
//
// The first is the record's own. The second exists because the Directory
// signs one canonical anchor and every BVN reuses it: post-Vandenberg a
// DirectoryAnchor's signature is made with the destination set to the
// Directory and the principal rewritten under dn.acme, and the copy sitting
// in bvn-X.acme/anchors carries the BVN's destination and principal
// (msg_block_anchor.go, "Allow reusing signatures from the DN"). Without this
// form the Directory's own root — the one that has to come out of a BVN's
// pool — can never be verified (#4301).
func signedForms(rec *api.MessageRecord[*messaging.TransactionMessage]) []*messaging.SequencedMessage {
	own := *rec.Sequence
	own.Message = rec.Message
	forms := []*messaging.SequencedMessage{&own}

	txn := rec.Message.Transaction
	if txn == nil || txn.Body == nil || txn.Header.Principal == nil {
		return forms
	}
	if txn.Body.Type() != protocol.TransactionTypeDirectoryAnchor {
		return forms
	}
	if part, ok := protocol.ParsePartitionUrl(txn.Header.Principal); !ok || strings.EqualFold(part, protocol.Directory) {
		return forms
	}

	asDn := *rec.Sequence
	asDn.Destination = protocol.DnUrl()
	rewritten := txn.Copy()
	rewritten.Header.Principal = protocol.DnUrl().JoinPath(txn.Header.Principal.Path)
	asDn.Message = &messaging.TransactionMessage{Transaction: rewritten}
	return append(forms, &asDn)
}

func verifiesAny(sig protocol.KeySignature, forms []*messaging.SequencedMessage) bool {
	for _, f := range forms {
		if sig.Verify(nil, f) {
			return true
		}
	}
	return false
}
