// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// isAdmissible reports whether a cross-partition message is proven to have come
// from its source: its proof terminates at a directory anchor we already hold.
//
// A message with NO proof is replica-accepted (#4140) — its proof was checked,
// anchored, and absorbed into the stream's replica when it first arrived — and
// is admissible by construction.
//
// Extracted so staging can ask the question before executing anything (#4169
// step 3). Staging needs it because admissibility is not an ordering property
// but it bounds the ordering: a stream must not advance past a message that
// did not execute, and an unproven message does not execute. Advancing over it
// would mark it delivered without ever running it — a lost delivery.
//
// Returns false, nil for "not yet". The CALLER decides what that means: the
// executor turns it into errors.Pending so the message is retried when the
// anchor arrives, and staging turns it into the end of the run. Reporting it
// as an error here would force both callers to unpick the same error.
func (x *Executor) isAdmissible(batch *database.Batch, proof *protocol.AnnotatedReceipt) (bool, error) {
	if proof == nil {
		return true, nil
	}

	anchor := proof.TerminalAnchor()
	_, err := batch.Account(x.Describe.AnchorPool()).
		AnchorChain(protocol.Directory).
		Root().
		IndexOf(anchor)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, errors.NotFound):
		return false, nil
	default:
		return false, errors.UnknownError.WithFormat("search for directory anchor %x: %w", anchor, err)
	}
}

// anchorIsAdmissible reports whether an anchor is authorized to execute.
//
// An anchor's gate is not a proof-to-a-DN-root like a synthetic's — it is a
// validator signature quorum, with one shortcut. Extracted alongside
// isAdmissible (#4169 step 3b) because staging needs ONE answer per message
// regardless of which kind of stream carries it, and because an anchor that is
// not authorized never reaches the sequence check: it records pending, so its
// stream does not advance. Staging must not advance over it either.
//
// The shortcut first: a collection proof under a known directory root
// authorizes the anchor by itself (#4056), and that is the same terminal-anchor
// test a synthetic's proof gets, so it is the same function. If the proof's
// anchor has not arrived the anchor is not rejected — it falls through to the
// quorum, because the healing loop resubmits until a current anchor extends
// the destination's directory-root knowledge past the proven range.
func (x *Executor) anchorIsAdmissible(batch *database.Batch, proof *protocol.AnnotatedReceipt, txn *protocol.Transaction, source *url.URL) (bool, error) {
	if proof != nil {
		ok, err := x.isAdmissible(batch, proof)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}
		if ok {
			return true, nil
		}
		// Not yet anchored — fall through to the signature quorum.
	}

	sigs, err := batch.Account(txn.Header.Principal).
		Transaction(txn.ID().Hash()).
		ValidatorSignatures().
		Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load anchor signatures: %w", err)
	}

	partition, ok := protocol.ParsePartitionUrl(source)
	if !ok {
		return false, errors.BadRequest.WithFormat("source %v is not a partition", source)
	}
	return uint64(len(sigs)) >= x.globals().Active.ValidatorThreshold(partition), nil
}
