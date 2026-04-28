// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package pipeline orchestrates the v2 bootstrap end to end:
//
//  1. Trust phase: walk block headers, verify the validator quorum
//     on each, evolve the operators key book across blocks.
//  2. Data phase: pull every account in the bootstrap set into a
//     local database via the v2 puller.
//  3. Convergence: local UpdateBPT() must equal the verified
//     terminal header's StateTreeRoot. Fail closed.
//
// The orchestration here is deliberately framework-poor: it takes
// already-constructed sources and a database, runs the three phases
// in order, and returns a Result. Wiring sources to a live network
// (api.Querier2 for the puller, anchor-pool/signature messages for
// the header source) is the next slice on this branch.
package pipeline

import (
	"context"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/convergence"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/headerwalk"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Options carries the inputs Bootstrap orchestrates over.
type Options struct {
	// --- Trust phase inputs ---

	// HeaderSource serves block headers + signatures + operators
	// deltas. In production this wraps an api.Querier2 view of the
	// peer's anchor pool and signature messages.
	HeaderSource headerwalk.HeaderSource

	// StartHeight, EndHeight bound the header walk. Both inclusive.
	StartHeight, EndHeight uint64

	// InitialValidatorSet is the validator set assumed at
	// StartHeight. Typically the binary's pinned set, or the set
	// recorded in a previous bootstrap that is being resumed.
	InitialValidatorSet headerwalk.ValidatorSet

	// ApplyDelta evolves the validator set across an
	// operators-keybook update. nil means no rotation expected
	// during the walk (steady-state networks); production passes
	// the keybookat-backed implementation.
	ApplyDelta func(headerwalk.ValidatorSet, []headerwalk.OperatorsDelta) (headerwalk.ValidatorSet, error)

	// QuorumOpts tunes the per-header verification (default 2/3).
	QuorumOpts headerwalk.QuorumOptions

	// --- Data phase inputs ---

	// PullSource serves account state for the puller. In production
	// this is a pull.APISource over the same peer endpoint.
	PullSource pull.Source

	// Accounts is the minimum bootstrap set the launcher must pull.
	Accounts []*url.URL

	// --- Storage ---

	// Database is the local store the launcher is filling. It must
	// have the production observer wired (default after construction
	// today; pinned by completeness tests).
	Database *database.Database
}

// Result reports a successful bootstrap.
type Result struct {
	// VerifiedAnchor is the StateTreeRoot from the terminal verified
	// header. After convergence this is also the root of the local
	// BPT — they're equal by definition of a successful bootstrap.
	VerifiedAnchor [32]byte

	// LocalBPTRoot is what the local UpdateBPT produced. Equal to
	// VerifiedAnchor on success.
	LocalBPTRoot [32]byte

	// AccountsPulled counts the URLs in opts.Accounts that the data
	// phase processed without error.
	AccountsPulled int

	// TerminalStep is the last verified header from the trust phase
	// (height, validator set after, etc.).
	TerminalStep *headerwalk.Step
}

// Bootstrap runs the three phases and returns Result on full success.
// On any phase failure it returns an error and discards any partially
// pulled state. Persistence of the bootstrap artifact (#3965-style)
// is layered on top by the caller — this function only owns the
// proof itself.
func Bootstrap(ctx context.Context, opts Options) (*Result, error) {
	if opts.HeaderSource == nil {
		return nil, errors.New("pipeline: HeaderSource required")
	}
	if opts.PullSource == nil {
		return nil, errors.New("pipeline: PullSource required")
	}
	if opts.Database == nil {
		return nil, errors.New("pipeline: Database required")
	}
	if opts.StartHeight > opts.EndHeight {
		return nil, fmt.Errorf("pipeline: StartHeight (%d) > EndHeight (%d)", opts.StartHeight, opts.EndHeight)
	}

	// 1. Trust phase.
	step, err := headerwalk.Walk(
		ctx,
		opts.HeaderSource,
		opts.StartHeight, opts.EndHeight,
		opts.InitialValidatorSet,
		opts.QuorumOpts,
		opts.ApplyDelta,
	)
	if err != nil {
		return nil, fmt.Errorf("trust phase: %w", err)
	}
	if step == nil || step.Header == nil {
		return nil, errors.New("trust phase: nil terminal step (no headers walked?)")
	}
	expected := step.Header.StateTreeRoot

	// 2. Data phase.
	batch := opts.Database.Begin(true)
	pulled := 0
	for _, u := range opts.Accounts {
		if err := pull.Account(ctx, opts.PullSource, batch, u); err != nil {
			batch.Discard()
			return nil, fmt.Errorf("data phase: pull %s: %w", u, err)
		}
		pulled++
	}

	// 3. Convergence.
	if err := convergence.Verify(batch, expected); err != nil {
		batch.Discard()
		return nil, fmt.Errorf("convergence: %w", err)
	}

	// Read the now-equal local root for the result, then commit.
	localRoot, err := batch.GetBptRootHash()
	if err != nil {
		batch.Discard()
		return nil, fmt.Errorf("read local BPT root: %w", err)
	}
	if err := batch.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &Result{
		VerifiedAnchor: expected,
		LocalBPTRoot:   localRoot,
		AccountsPulled: pulled,
		TerminalStep:   step,
	}, nil
}
