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
		// Not in the batch's view. A store with a window answers a shallow
		// read from its newest blocks only and calls anything older absent
		// (database spec, "Windowed stores"), and the block's batch is
		// shallow. If the Directory anchor the proof names has executed
		// here, its root was recorded and the question is whether history
		// holds it: one deep read, for that case alone -- a proof that
		// arrives before its anchor misses here too, and that miss must
		// stay the cheap one it is. Run 20261002T090546Z: a proof pulled
		// for an entry undelivered longer than the window was "disproved"
		// and the stream held behind it for the rest of the run (#4505).
		if proof.Anchor == nil {
			return -1, false, nil
		}
		executed, err := x.executedDirectoryBlock(batch)
		if err != nil {
			return -1, false, err
		}
		if proof.Anchor.SourceBlock > executed || x.Database == nil {
			return -1, false, nil
		}
		return x.provingAnchorIndexDeep(anchor)
	default:
		return -1, false, errors.UnknownError.WithFormat("search for directory anchor %x: %w", anchor, err)
	}
}

// provingAnchorIndexDeep is provingAnchorIndex's lookup through a reader
// that reaches past the store's window.
func (x *Executor) provingAnchorIndexDeep(anchor []byte) (int64, bool, error) {
	index, found := int64(-1), false
	err := x.deepView(func(batch *database.Batch) error {
		i, err := batch.Account(x.Describe.AnchorPool()).
			AnchorChain(protocol.Directory).
			Root().
			IndexOf(anchor)
		switch {
		case err == nil:
			index, found = i, true
			return nil
		case errors.Is(err, errors.NotFound):
			return nil
		default:
			return err
		}
	})
	if err != nil {
		return -1, false, errors.UnknownError.WithFormat("search history for directory anchor %x: %w", anchor, err)
	}
	return index, found, nil
}

// executedDirectoryBlock is the newest Directory block whose anchor this
// partition has executed, as the anchor pool records it; zero before the
// first.
func (x *Executor) executedDirectoryBlock(batch *database.Batch) (uint64, error) {
	v, err := batch.Account(x.Describe.AnchorPool()).DirectoryAnchorBlock().Get()
	switch {
	case errors.Is(err, errors.NotFound):
		return 0, nil
	case err != nil:
		return 0, errors.UnknownError.WithFormat("load directory anchor block: %w", err)
	}
	return v, nil
}
