// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package pipeline orchestrates the v2-corrected bootstrap end to
// end: the (DN, BVN) pair model.
//
//	Phase A — DN trust:
//	   - Fetch the genesis (major-block 1) header from the source;
//	     reject if its StateTreeAnchor doesn't match the binary pin.
//	   - Walk major blocks 1 → ToMajorBlock verifying DN-validator
//	     quorum on each DA's wrapping SequencedMessage.
//	Phase B — DN data:
//	   - Pull DN's complete account set into DNDatabase.
//	   - UpdateBPT(); root must equal the latest verified DN
//	     StateTreeAnchor.
//	Phase C — BVN data:
//	   - Read BVN's StateTreeAnchor + major-block index out of
//	     trusted DN state via BVNAnchorFromDN. (The BVN→DN anchor
//	     sits in dn.acme/anchors's main chain, committed to DN's
//	     BPT we just verified.)
//	   - Pull BVN's complete account set into BVNDatabase.
//	   - UpdateBPT(); root must equal the BVN's StateTreeAnchor.
//
// All four anchors (DN-genesis pin, DN-verified, BVN-verified, plus
// the local BPT roots) are returned in Result for the caller to
// persist via bootpersist.
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

// Options carries every input Bootstrap orchestrates over.
type Options struct {
	// --- Trust phase (Phase A) ---

	// HeaderSource serves DN-validator-signed major-block headers.
	// In production this is a headerwalk.APISource constructed
	// against the chosen BVN's anchor pool URL.
	HeaderSource headerwalk.HeaderSource

	// ToMajorBlock is the latest DN major block to walk to. The
	// walk runs 1..ToMajorBlock inclusive. Pipeline determines
	// this from the peer's current consensus status before calling
	// Bootstrap.
	ToMajorBlock uint64

	// InitialValidatorSet is the DN operators key page state used
	// to verify the genesis end of the walk. For steady-state
	// networks (no rotations), this is the current operators page
	// fetched from the network.
	InitialValidatorSet headerwalk.ValidatorSet

	// ApplyDelta evolves the validator set across operators-keybook
	// updates. Pass keybookat.ApplyDelta in production. nil
	// substitutes a no-op (steady-state hot path).
	ApplyDelta func(headerwalk.ValidatorSet, []headerwalk.OperatorsDelta) (headerwalk.ValidatorSet, error)

	// QuorumOpts tunes per-major-block validator-quorum
	// verification. Default is the protocol's 2/3 rule.
	QuorumOpts headerwalk.QuorumOptions

	// GenesisStateTreeAnchor is the binary's pinned DN
	// StateTreeAnchor at major-block 1. The pipeline fetches the
	// genesis header from the source, compares its StateTreeAnchor
	// to this value, and aborts with ErrGenesisMismatch if they
	// differ. Zero disables the check (dev-only path; production
	// MUST set this).
	GenesisStateTreeAnchor [32]byte

	// --- Data phases (B + C) ---

	// PullSource serves account state for both phases. In
	// production this is a single pull.APISource over the same
	// peer endpoint.
	PullSource pull.Source

	// DNAccounts is the DN's minimum bootstrap set.
	DNAccounts []*url.URL

	// DNDatabase is the local DN store.
	DNDatabase *database.Database

	// BVN is the BVN partition this node will run (e.g., "Apollo").
	BVN string

	// BVNAccounts is the BVN's minimum bootstrap set.
	BVNAccounts []*url.URL

	// BVNDatabase is the local BVN store.
	BVNDatabase *database.Database

	// BVNAnchorFromDN extracts the BVN's StateTreeAnchor +
	// major-block index out of trusted DN state, after Phase B has
	// converged. It runs against the DN database (already committed
	// at the time of the call), looks up the BVN→DN anchor on
	// dn.acme/anchors, and returns the BVN's StateTreeAnchor.
	BVNAnchorFromDN func(ctx context.Context, dnDB *database.Database, bvn string) (anchor [32]byte, majorBlock uint64, err error)
}

// Result reports what every phase produced on full success.
type Result struct {
	// TerminalStep is the last verified header from the trust
	// phase — i.e., the major-block ToMajorBlock header.
	TerminalStep *headerwalk.Step

	// DNVerifiedAnchor is DN's StateTreeAnchor from the latest
	// verified DA. Local DN BPT must equal this for ACTIVE.
	DNVerifiedAnchor [32]byte

	// DNVerifiedMajorBlock is the major-block index the latest
	// verified DA was at — equal to ToMajorBlock on success.
	DNVerifiedMajorBlock uint64

	// DNLocalBPTRoot is what UpdateBPT(DN) produced. Equal to
	// DNVerifiedAnchor on success.
	DNLocalBPTRoot [32]byte

	// DNAccountsPulled counts the DN URLs successfully pulled.
	DNAccountsPulled int

	// BVNVerifiedAnchor is the BVN's StateTreeAnchor as read out
	// of trusted DN state. Local BVN BPT must equal this.
	BVNVerifiedAnchor [32]byte

	// BVNVerifiedMajorBlock is the BVN major-block index the
	// trusted BVN→DN anchor was at.
	BVNVerifiedMajorBlock uint64

	// BVNLocalBPTRoot is what UpdateBPT(BVN) produced. Equal to
	// BVNVerifiedAnchor on success.
	BVNLocalBPTRoot [32]byte

	// BVNAccountsPulled counts the BVN URLs successfully pulled.
	BVNAccountsPulled int
}

// ErrGenesisMismatch is returned when the genesis-major-block
// header's StateTreeAnchor doesn't match the binary's pinned value.
// Sentinel so callers can branch on it (dev networks may want a
// fallback; production aborts).
var ErrGenesisMismatch = errors.New("pipeline: DN genesis StateTreeAnchor mismatch — chain or peer is not the expected network")

// Bootstrap runs all three phases. On any phase failure it returns
// an error and discards any partially pulled state in either
// database.
func Bootstrap(ctx context.Context, opts Options) (*Result, error) {
	if err := validate(opts); err != nil {
		return nil, err
	}

	// --- Phase A: trust ---

	// Verify the genesis pin first. Without this, a malicious peer
	// could fabricate the entire history and we'd accept whatever
	// state they served.
	if opts.GenesisStateTreeAnchor != ([32]byte{}) {
		genesisHdr, err := opts.HeaderSource.Header(ctx, 1)
		if err != nil {
			return nil, fmt.Errorf("trust phase: fetch genesis header: %w", err)
		}
		if genesisHdr == nil {
			return nil, errors.New("trust phase: nil genesis header")
		}
		if genesisHdr.StateTreeRoot != opts.GenesisStateTreeAnchor {
			return nil, fmt.Errorf("%w: header=%x pin=%x",
				ErrGenesisMismatch, genesisHdr.StateTreeRoot[:8], opts.GenesisStateTreeAnchor[:8])
		}
	}

	// Walk major blocks 1..ToMajorBlock verifying validator quorum.
	step, err := headerwalk.Walk(
		ctx,
		opts.HeaderSource,
		1, opts.ToMajorBlock,
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
	dnExpected := step.Header.StateTreeRoot

	// --- Phase B: DN data ---

	dnBatch := opts.DNDatabase.Begin(true)
	dnPulled := 0
	for _, u := range opts.DNAccounts {
		if err := pull.Account(ctx, opts.PullSource, dnBatch, u); err != nil {
			dnBatch.Discard()
			return nil, fmt.Errorf("DN data phase: pull %s: %w", u, err)
		}
		dnPulled++
	}

	if err := convergence.Verify(dnBatch, dnExpected); err != nil {
		dnBatch.Discard()
		return nil, fmt.Errorf("DN convergence: %w", err)
	}
	dnLocalRoot, err := dnBatch.GetBptRootHash()
	if err != nil {
		dnBatch.Discard()
		return nil, fmt.Errorf("read DN local BPT root: %w", err)
	}
	if err := dnBatch.Commit(); err != nil {
		return nil, fmt.Errorf("commit DN: %w", err)
	}

	// --- Phase C: BVN data ---

	bvnExpected, bvnMajor, err := opts.BVNAnchorFromDN(ctx, opts.DNDatabase, opts.BVN)
	if err != nil {
		return nil, fmt.Errorf("read BVN anchor from DN: %w", err)
	}
	if bvnExpected == ([32]byte{}) {
		return nil, errors.New("BVN anchor extraction returned zero StateTreeAnchor")
	}

	bvnBatch := opts.BVNDatabase.Begin(true)
	bvnPulled := 0
	for _, u := range opts.BVNAccounts {
		if err := pull.Account(ctx, opts.PullSource, bvnBatch, u); err != nil {
			bvnBatch.Discard()
			return nil, fmt.Errorf("BVN data phase: pull %s: %w", u, err)
		}
		bvnPulled++
	}

	if err := convergence.Verify(bvnBatch, bvnExpected); err != nil {
		bvnBatch.Discard()
		return nil, fmt.Errorf("BVN convergence: %w", err)
	}
	bvnLocalRoot, err := bvnBatch.GetBptRootHash()
	if err != nil {
		bvnBatch.Discard()
		return nil, fmt.Errorf("read BVN local BPT root: %w", err)
	}
	if err := bvnBatch.Commit(); err != nil {
		return nil, fmt.Errorf("commit BVN: %w", err)
	}

	return &Result{
		TerminalStep:          step,
		DNVerifiedAnchor:      dnExpected,
		DNVerifiedMajorBlock:  step.Header.Height,
		DNLocalBPTRoot:        dnLocalRoot,
		DNAccountsPulled:      dnPulled,
		BVNVerifiedAnchor:     bvnExpected,
		BVNVerifiedMajorBlock: bvnMajor,
		BVNLocalBPTRoot:       bvnLocalRoot,
		BVNAccountsPulled:     bvnPulled,
	}, nil
}

func validate(opts Options) error {
	switch {
	case opts.HeaderSource == nil:
		return errors.New("pipeline: HeaderSource required")
	case opts.PullSource == nil:
		return errors.New("pipeline: PullSource required")
	case opts.DNDatabase == nil:
		return errors.New("pipeline: DNDatabase required")
	case opts.BVNDatabase == nil:
		return errors.New("pipeline: BVNDatabase required")
	case opts.BVN == "":
		return errors.New("pipeline: BVN required")
	case opts.BVNAnchorFromDN == nil:
		return errors.New("pipeline: BVNAnchorFromDN required")
	case opts.ToMajorBlock == 0:
		return errors.New("pipeline: ToMajorBlock must be ≥ 1")
	}
	return nil
}
