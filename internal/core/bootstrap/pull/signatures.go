// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// signature is one entry of an account's signature chain that is a validator's
// signature of an anchor, and what executing it wrote beside the entry.
//
// A node that executes an anchor records each distinct validator's copy on the
// principal's signature chain and indexes it under the transaction
// (msg_block_anchor.go, RecordHistory): the chain entry, the transaction's
// history index into the chain, and the principal among the message's signers.
// It then executes the sequenced message the copies carry, which stores that
// message and names it as the transaction's cause (msg_common.go,
// recordMessageAndStatus). The query API reads an anchor's signatures from the
// history index and the sequence they cover from the cause
// (internal/api/v3/load.go, loadMessage), and from nowhere else. The pull
// takes the chain with the message behind each entry (#4400); neither the
// index nor the cause is on a chain, and a node that holds the chain without
// them serves every anchor it pulled with no signatures (#4413), or with
// signatures over a sequence it cannot name (#4416). So the pull rebuilds both
// from the entries it took, the way execution writes them, and never from a
// peer's word about them.
type signature struct {
	account *url.URL
	index   uint64
	txn     [32]byte

	// sequence is the sequenced message the signature covers, in the form
	// the peer stored it (its transaction by reference), and sequenceID its
	// ID as executed -- taken from the full form, whose hash the stored form
	// does not have.
	sequence   messaging.Message
	sequenceID *url.TxID
}

// signatureOf is the signature a signature chain entry is, if it is an
// anchor's: stored is the message as the peer stored it, full the same with
// its transaction put back.
func signatureOf(account *url.URL, index uint64, stored, full messaging.Message) (signature, bool) {
	sba, ok1 := stored.(*messaging.BlockAnchor)
	fba, ok2 := full.(*messaging.BlockAnchor)
	if !ok1 || !ok2 {
		return signature{}, false
	}
	seq, ok := fba.Anchor.(*messaging.SequencedMessage)
	if !ok {
		return signature{}, false
	}
	tm, ok := seq.Message.(*messaging.TransactionMessage)
	if !ok || tm.Transaction == nil {
		return signature{}, false
	}
	return signature{
		account:    account,
		index:      index,
		txn:        *(*[32]byte)(tm.Transaction.GetHash()),
		sequence:   sba.Anchor,
		sequenceID: seq.ID(),
	}, true
}

// checkAnchorSignature refuses a BlockAnchor, in its full form, whose signature
// is not a signature of the anchor it carries. It is the executor's check
// (msg_block_anchor.go, check and checkSignature) less the one thing a joining
// node cannot know: which validators were active when the anchor executed.
// That is not needed to keep the entry -- the chain it sits on is under the
// root the account is settled against, which a quorum of the partition signed
// -- and it is what every reader checks when it is served the anchor, against
// the set that reader trusts (anchorsrc.verify).
//
// An anchor authorized by a collection proof carries no signature and is kept
// as the chain holds it; no production path makes one (#4416).
func checkAnchorSignature(ba *messaging.BlockAnchor) error {
	if ba.Signature == nil {
		if ba.Proof == nil {
			return errors.Unauthenticated.With("a signature chain entry is an anchor with neither a signature nor a proof")
		}
		return nil
	}
	seq, ok := ba.Anchor.(*messaging.SequencedMessage)
	if !ok {
		return errors.BadRequest.WithFormat("a signed anchor carries a %T, not a sequenced message", ba.Anchor)
	}
	tm, ok := seq.Message.(*messaging.TransactionMessage)
	if !ok || tm.Transaction == nil || tm.Transaction.Body == nil {
		return errors.BadRequest.With("a signed anchor carries no transaction")
	}
	txn := tm.Transaction
	if !txn.Body.Type().IsAnchor() {
		return errors.BadRequest.WithFormat("a %v is signed as an anchor", txn.Body.Type())
	}
	if ba.Signature.Verify(nil, seq) {
		return nil
	}

	// The Directory signs one canonical anchor and every BVN reuses its
	// signatures: the copy in a BVN's pool carries the BVN's destination and
	// principal, and the signature was made over the Directory's
	// (checkSignature, "Allow reusing signatures from the DN").
	part, _ := protocol.ParsePartitionUrl(txn.Header.Principal)
	if txn.Body.Type() == protocol.TransactionTypeDirectoryAnchor && txn.Header.Principal != nil && !strings.EqualFold(part, protocol.Directory) {
		asDn := *seq
		asDn.Destination = protocol.DnUrl()
		rewritten := txn.Copy()
		rewritten.Header.Principal = protocol.DnUrl().JoinPath(txn.Header.Principal.Path)
		asDn.Message = &messaging.TransactionMessage{Transaction: rewritten}
		if ba.Signature.Verify(nil, &asDn) {
			return nil
		}
	}
	return errors.Unauthenticated.WithFormat("the signature of %x by %x does not verify",
		txn.GetHash()[:4], ba.Signature.GetPublicKey())
}

// storeSignatures writes, for each pulled signature, what executing it wrote
// beside its chain entry -- the history index and the signer as RecordHistory
// writes them, the sequenced message and the cause as recordMessageAndStatus
// writes them -- without appending to the chain: the pull already put the
// entry there.
func storeSignatures(batch *database.Batch, sigs []signature) error {
	for _, s := range sigs {
		if err := batch.Account(s.account).Transaction(s.txn).History().Add(s.index); err != nil {
			return fmt.Errorf("store the history of %x: %w", s.txn[:4], err)
		}
		if err := batch.Message(s.txn).Signers().Add(s.account); err != nil {
			return fmt.Errorf("store the signers of %x: %w", s.txn[:4], err)
		}
		if err := batch.Message(s.sequenceID.Hash()).Main().Put(s.sequence); err != nil {
			return fmt.Errorf("store the sequence of %x: %w", s.txn[:4], err)
		}
		if err := batch.Message(s.txn).Cause().Add(s.sequenceID); err != nil {
			return fmt.Errorf("store the cause of %x: %w", s.txn[:4], err)
		}
	}
	return nil
}
