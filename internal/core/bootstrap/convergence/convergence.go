// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package convergence is the v2 bootstrap's promotion gate: the
// single equality that decides whether the launcher leaves BOOTING
// for ACTIVE.
//
// The check has one shape:
//
//	UpdateBPT() over locally pulled state ⟹ root R
//	R must equal the StateTreeRoot of a header verified by the
//	headerwalk trust phase.
//
// If they match, every leaf in the local BPT was the network's leaf
// at that height, so every account state and every chain anchor it
// commits is trusted along with it. Fail closed otherwise.
package convergence

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// ErrMismatch is the sentinel returned when the local BPT root does
// not equal the trusted anchor. Distinct from transport / database
// errors so callers can branch on it.
var ErrMismatch = errors.New("convergence: local BPT root does not match verified anchor")

// Verify computes the local BPT root over the data in batch and
// requires it to equal expected. UpdateBPT is called as part of the
// check — callers don't need to invoke it themselves.
//
// The batch must be a writable batch since UpdateBPT mutates the BPT
// tree. The caller commits or discards based on outcome (typical
// pattern: commit on success so the BPT is persisted for serving;
// discard on failure to preserve the pre-attempt state).
func Verify(batch *database.Batch, expected [32]byte) error {
	if batch == nil {
		return errors.New("convergence: nil batch")
	}
	if expected == ([32]byte{}) {
		return errors.New("convergence: refusing to verify against zero anchor — header walk almost certainly didn't produce a real terminal")
	}

	if err := batch.UpdateBPT(); err != nil {
		return fmt.Errorf("update BPT: %w", err)
	}
	got, err := batch.GetBptRootHash()
	if err != nil {
		return fmt.Errorf("read BPT root: %w", err)
	}
	if got != expected {
		return fmt.Errorf("%w: local=%x expected=%x", ErrMismatch, got[:8], expected[:8])
	}
	return nil
}
