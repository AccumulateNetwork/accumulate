// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
	_, ok, err := x.provingAnchorIndex(batch, proof)
	return ok, err
}

// provingAnchorIndex is isAdmissible with the evidence: where in the directory
// anchor chain the proof's terminal anchor sits, if it does. Staging uses the
// index to tell an anchor applied THIS block from one applied earlier (#4169
// step 0c). A nil proof is admissible and has no anchor; the index is -1.
func (x *Executor) provingAnchorIndex(batch *database.Batch, proof *protocol.AnnotatedReceipt) (int64, bool, error) {
	if proof == nil {
		return -1, true, nil
	}

	anchor := proof.TerminalAnchor()
	i, err := batch.Account(x.Describe.AnchorPool()).
		AnchorChain(protocol.Directory).
		Root().
		IndexOf(anchor)
	switch {
	case err == nil:
		return i, true, nil
	case errors.Is(err, errors.NotFound):
		return -1, false, nil
	default:
		return -1, false, errors.UnknownError.WithFormat("search for directory anchor %x: %w", anchor, err)
	}
}

