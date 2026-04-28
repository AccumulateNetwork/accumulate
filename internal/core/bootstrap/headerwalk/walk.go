// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package headerwalk

import (
	"context"
	"fmt"
)

// HeaderSource is the read-only surface the walker needs from the
// network. Production wraps an api.Querier2; tests use a fake.
type HeaderSource interface {
	// Header returns the block header at the given height. Returns
	// ErrNoSuchHeight if the height is outside the source's range.
	Header(ctx context.Context, height uint64) (*Header, error)

	// Signatures returns the validator signatures attesting to the
	// header at the given height.
	Signatures(ctx context.Context, height uint64) ([]HeaderSignature, error)

	// OperatorsDeltaAt returns the operators-keybook deltas applied
	// in the block at the given height, in the order they appear.
	// An empty slice means the block did not modify the operators
	// key book (the common case once a network is running steady).
	OperatorsDeltaAt(ctx context.Context, height uint64) ([]OperatorsDelta, error)
}

// OperatorsDelta is a single change to the operators key book recorded
// in a block. The walker applies these to evolve ValidatorSet across
// blocks. Concrete delta semantics are defined by the production
// observer's handling of UpdateKeyPage / AddCredits / etc; this struct
// is a placeholder enum-like that the keybookat integration (next
// commit on this branch) fills in.
type OperatorsDelta struct {
	// Kind names the delta operation. Concrete values come from the
	// keybookat integration; for now the walker is agnostic.
	Kind string

	// Payload is the raw operation, opaque to the walker — keybookat
	// is responsible for interpreting it.
	Payload []byte
}

// Step is one verified record from the walker: a header that met
// quorum at the validator set the walker held when it was processed.
type Step struct {
	Header *Header

	// ValidatorSetBefore is the validator set used to verify this
	// header's signatures. Useful for replay / audit.
	ValidatorSetBefore ValidatorSet

	// ValidatorSetAfter is the validator set after applying this
	// block's operators-keybook deltas. Equal to ValidatorSetBefore
	// when the block contained no operators-keybook updates.
	ValidatorSetAfter ValidatorSet
}

// ErrNoSuchHeight is returned by sources when a height is outside
// their addressable range.
var ErrNoSuchHeight = fmt.Errorf("headerwalk: no header at requested height")

// Walk verifies headers from `from` to `to` (inclusive on both ends),
// driving the validator set forward via OperatorsDeltaAt. The
// validator set at `from` is supplied as `initial` — typically the
// pinned set the binary was built with, or the set verified at the
// previous bootstrap.
//
// Direction is encoded by `from <= to`. Reverse walks (back-walking
// from current to a pin) reverse the iteration order at the source
// boundary; for the simple case here the loop always increments.
//
// The walker returns the verified terminal Step on success. On
// failure (any step's quorum check fails), it returns the last
// successful Step plus the error so callers can record progress.
func Walk(
	ctx context.Context,
	src HeaderSource,
	from, to uint64,
	initial ValidatorSet,
	opts QuorumOptions,
	applyDelta func(ValidatorSet, []OperatorsDelta) (ValidatorSet, error),
) (*Step, error) {
	if from > to {
		return nil, fmt.Errorf("headerwalk: from=%d > to=%d (use a reversed source for back-walks)", from, to)
	}
	if applyDelta == nil {
		applyDelta = noOpDelta
	}

	current := initial
	var last *Step

	for h := from; h <= to; h++ {
		hdr, err := src.Header(ctx, h)
		if err != nil {
			return last, fmt.Errorf("fetch header %d: %w", h, err)
		}
		sigs, err := src.Signatures(ctx, h)
		if err != nil {
			return last, fmt.Errorf("fetch signatures %d: %w", h, err)
		}
		if err := VerifyQuorum(hdr, current, sigs, opts); err != nil {
			return last, err
		}

		deltas, err := src.OperatorsDeltaAt(ctx, h)
		if err != nil {
			return last, fmt.Errorf("fetch operators deltas %d: %w", h, err)
		}
		next, err := applyDelta(current, deltas)
		if err != nil {
			return last, fmt.Errorf("apply operators deltas %d: %w", h, err)
		}

		last = &Step{
			Header:             hdr,
			ValidatorSetBefore: current,
			ValidatorSetAfter:  next,
		}
		current = next
	}

	return last, nil
}

// noOpDelta is the default applyDelta: no operators-keybook updates
// take effect. Useful for tests and for the steady-state case where
// the network's operators key book hasn't rotated. Real deployments
// pass the keybookat-backed implementation in.
func noOpDelta(set ValidatorSet, _ []OperatorsDelta) (ValidatorSet, error) {
	return set, nil
}
