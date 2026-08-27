// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// streamKind says which kind of ordered stream governs a message, if any.
type streamKind int

const (
	// streamNone is not a stream at all. User envelopes carry no position, so
	// nothing orders them beyond the block they arrive in.
	streamNone streamKind = iota

	// streamAnchor and streamSynthetic are SEPARATE streams even between the
	// same pair of partitions: anchors are tracked by the anchor pool and
	// synthetics by the synthetic account. Conflating them would let an
	// anchor's position gate a synthetic's.
	streamAnchor
	streamSynthetic
)

// stream names the ordered stream that governs a message — the account whose
// sequence ledger tracks it, and the partition it came from. The zero value is
// "no stream".
type stream struct {
	kind   streamKind
	ledger *url.URL
	source *url.URL
}

func (s stream) ok() bool { return s.kind != streamNone }

// resolveTxn loads a transaction by hash. See streamOf.
type resolveTxn func(hash [32]byte) (*protocol.Transaction, error)

// streamOf reports which ordered stream governs a message, unwrapping whatever
// carries it, and returns the sequenced message inside.
//
// This is the single statement of a rule that was previously made in two
// places that could disagree — SequencedMessage.isAnchor, which resolves a
// remote stub, and the readiness pre-pass's isAnchorBody, which did not. A
// message classified as an anchor by one and a synthetic by the other would be
// gated by two different ledgers.
//
// `resolve` supplies the lookup for the one case that needs the database: a
// sequenced message can carry a REMOTE placeholder whose real body decides
// whether it is an anchor. Callers pass their own — the executor searches its
// bundle before the database, staging searches the block — so the lookup varies
// while the CLASSIFICATION does not. It is only consulted for a remote body
// under V2Baikonur, so callers with no way to resolve may pass nil and get the
// same answer for every other message.
func (x *Executor) streamOf(msg messaging.Message, resolve resolveTxn) (stream, *messaging.SequencedMessage, error) {
	seq, ok := unwrapSequenced(msg)
	if !ok || seq.Source == nil {
		return stream{}, nil, nil
	}

	isAnchor, err := x.sequencedIsAnchor(seq, resolve)
	if err != nil {
		return stream{}, nil, errors.UnknownError.Wrap(err)
	}

	s := stream{kind: streamSynthetic, ledger: x.Describe.Synthetic(), source: seq.Source}
	if isAnchor {
		s = stream{kind: streamAnchor, ledger: x.Describe.AnchorPool(), source: seq.Source}
	}
	return s, seq, nil
}

// sequencedIsAnchor reports whether a sequenced message carries an anchor.
// Kept verbatim from SequencedMessage.isAnchor, with the context dependency
// replaced by the executor's globals (GetActiveGlobals returns exactly that)
// and a supplied lookup.
func (x *Executor) sequencedIsAnchor(seq *messaging.SequencedMessage, resolve resolveTxn) (bool, error) {
	msg, ok := seq.Message.(*messaging.TransactionMessage)
	switch {
	case ok && msg.Transaction.Body.Type().IsAnchor():
		return true, nil

	case !ok,
		!x.globals.Active.ExecutorVersion.V2BaikonurEnabled(),
		msg.Transaction.Body.Type() != protocol.TransactionTypeRemote:
		return false, nil
	}

	if resolve == nil {
		// No way to see the real body. Not an anchor is the answer the
		// executor gives for every non-remote message, and a caller without a
		// lookup has no business claiming otherwise.
		return false, nil
	}
	txn, err := resolve(msg.Hash())
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}
	return txn.Body.Type().IsAnchor(), nil
}

// unwrapSequenced finds the sequenced message inside whatever carries it.
//
// BlockAnchor is here and was not in the pre-pass's copy: an anchor travels as
// BlockAnchor{ SequencedMessage{ TransactionMessage } }, so a pass that does
// not open it sees no anchors at all.
func unwrapSequenced(msg messaging.Message) (*messaging.SequencedMessage, bool) {
	switch m := msg.(type) {
	case *messaging.SequencedMessage:
		return m, true
	case *messaging.SyntheticMessage:
		return unwrapSequenced2(m.Message)
	case *messaging.BadSyntheticMessage:
		return unwrapSequenced2(m.Message)
	case *messaging.BlockAnchor:
		return unwrapSequenced2(m.Anchor)
	}
	return nil, false
}

// unwrapSequenced2 opens one more layer, without recursing indefinitely on a
// crafted message.
func unwrapSequenced2(msg messaging.Message) (*messaging.SequencedMessage, bool) {
	seq, ok := msg.(*messaging.SequencedMessage)
	return seq, ok
}

// resolveFromBatch is the database half of the executor's lookup, for callers
// with no bundle to search first.
func resolveFromBatch(batch *database.Batch) resolveTxn {
	return func(hash [32]byte) (*protocol.Transaction, error) {
		var txn messaging.MessageWithTransaction
		err := batch.Message(hash).Main().GetAs(&txn)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		return txn.GetTransaction(), nil
	}
}
