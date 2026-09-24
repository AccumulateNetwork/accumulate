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
// What is counted is DISTINCT members of ONE set — the set this node trusts
// — and only after their signatures have been checked. Each of those three
// words was a way in:
//
//   - **One set.** Counting signatures from several sets against one
//     threshold lets a rotated-out key stand in for a current one: with
//     {k0,k1,k2,k3}/3 superseded by {k0,k1,k2}/2, an anchor carrying k3's old
//     signature and k0's new one reaches two, which the new set means two
//     CURRENT keys by (#4301, threat review F2). There is one set and one
//     threshold here — the set this node trusts — and the version a
//     signature declares is not a selector but a floor: a signature made
//     under a set this node has moved PAST is refused, and one declaring a
//     newer version is checked against this set like any other, which is the
//     only way a change is ever crossed (#4301, review finding 1). See Set
//     and Authority.
//
//   - **Distinct.** A second copy from one validator is no second signature.
//     The executor says it in its own words (msg_block_anchor.go, "A second
//     copy from the same validator is no second signature").
//
//   - **Checked.** Membership alone is worthless, because the query API
//     manufactures signature records: loadTransactionSignaturesV1
//     (internal/api/v3/load.go) appends a messaging.BlockAnchor carrying an
//     ED25519Signature with only PublicKey set, one per status.AnchorSigners.
//     Those are a peer's word about who signed, and they carry no signature
//     bytes, which is the only reason they do not reach the threshold on
//     their own.
func (s *Source) verify(producer string, rec *api.MessageRecord[*messaging.TransactionMessage]) error {
	return verifyQuorum(s.Authority, producer, rec)
}

// verifyQuorum is verify for any reader of anchors: the pool reader and the
// collector judge an anchor by one rule.
func verifyQuorum(authority *Authority, producer string, rec *api.MessageRecord[*messaging.TransactionMessage]) error {
	if rec.Sequence == nil {
		return errors.BadRequest.With("the anchor record carries no sequenced message, so there is nothing a signature covers")
	}
	if unsigned(rec) {
		return errors.Unauthenticated.With("the anchor carries no signatures")
	}

	set, err := authority.SetFor(producer)
	if err != nil {
		return errors.Unauthenticated.WithFormat("no validator set for %s: %w", producer, err)
	}
	if set.Threshold == 0 {
		return errors.Unauthenticated.WithFormat("%s has a zero validator threshold", producer)
	}

	// A BlockAnchor signature covers the SequencedMessage wrapping the
	// transaction, not the bare transaction — the executor's own pattern
	// (execute/v2/block/msg_block_anchor.go, checkSignature).
	forms := signedForms(rec)

	signed := map[[32]byte]bool{}
	for _, sigSet := range rec.Signatures.Records {
		if sigSet == nil || sigSet.Signatures == nil {
			continue
		}
		for _, sigMsg := range sigSet.Signatures.Records {
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
			if keySig.GetSignerVersion() < set.Version {
				// Made under a set this node has moved past. Refusing it is
				// what stops an old signature being replayed forward; a
				// retired signer forging a NEW one is stopped by membership.
				continue
			}
			if !set.MaySign(keySig.GetPublicKeyHash()) {
				continue
			}
			if !verifiesAny(keySig, forms) {
				continue
			}
			signed[*(*[32]byte)(keySig.GetPublicKeyHash())] = true
		}
	}

	if uint64(len(signed)) < set.Threshold {
		return errors.Unauthenticated.WithFormat(
			"only %d of %s's validators signed, and %d of the set at network version %d are required",
			len(signed), producer, set.Threshold, set.Version)
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
