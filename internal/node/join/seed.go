// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bytes"
	"context"
	"crypto/sha256"
	"log/slog"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A Seeder is a Buffer whose consensus may be told the round to order from
// when the join starts, before anything is pulled (consensus spec, "Restart";
// #4530). A node told only at its first proven state is told a round that
// state is inside the rescue window of, so its first handoff is always
// refused and every join pays another pull round.
type Seeder interface {
	// AwaitingSeed reports whether consensus is waiting for a round and may
	// be given one now: the node joined with no checkpoint and is in no
	// committee.
	AwaitingSeed() bool

	// SeedAt seeds consensus at round, a leader round the join has proven
	// the network committed, and reports whether it did.
	SeedAt(round uint64) bool
}

// A RoundProver is a State that can prove, with nothing pulled, the leader
// round that committed a block near the partition's newest.
type RoundProver interface {
	// ProvenRound is that block and its leader round. NotReady: nothing
	// proves one yet, and the join asks again before its next round.
	ProvenRound(ctx context.Context) (block, round uint64, err error)
}

// seedBeforePulling seeds a follower's consensus at a proven round near the
// frontier, so the groups committed while the node pulls are collected and
// the state it proves at the end of the pull is one it can hand off at.
// Nothing proven seeds nothing: the node is asked again before its next
// round, and one still not seeded when its state is proven is seeded from
// that state, as a validator is (Buffer.StageThrough).
func seedBeforePulling(ctx context.Context, opts Options, log *slog.Logger, wait *waiting) {
	seeder, ok := opts.Buffer.(Seeder)
	if !ok || !seeder.AwaitingSeed() {
		return
	}
	prover, ok := opts.State.(RoundProver)
	if !ok {
		return
	}
	wait.on("proving the round consensus orders from")
	block, round, err := prover.ProvenRound(ctx)
	if err != nil {
		log.Info("No round is proven for consensus to order from yet; asking again before the next pull round", "error", err)
		return
	}
	if seeder.SeedAt(round) {
		log.Info("Consensus is seeded before the pull, at the leader round of a block a signed anchor proves",
			"block", block, "round", round)
	}
}

// SeedWithin is how far behind the answering peer's own block an anchored
// block may be and still seed consensus: well inside what every peer still
// holds of its DAG (DAGGCDepth, 2,000 rounds, about a thousand blocks) and of
// its batches (ten minutes). The newest signed anchor is a block or two old
// unless the anchor source is held at an anchor no quorum signed, and a round
// that old is one consensus could not order from.
const SeedWithin = 128

// ProvenRound implements [RoundProver] from the system ledger ProvenLedger
// proves. A ledger that records no round was written by no leader, or before
// v2-kourou, and names nothing to order from.
func (s *PulledState) ProvenRound(ctx context.Context) (block, round uint64, err error) {
	ledger, err := s.ProvenLedger(ctx)
	if err != nil {
		return 0, 0, err
	}
	if ledger.LeaderRound == 0 {
		return 0, 0, errors.NotReady.WithFormat("%v's ledger at block %d records no leader round", s.partition, ledger.Index)
	}
	return ledger.Index, ledger.LeaderRound, nil
}

// ProvenLedger is the partition's system ledger as of the newest block a
// signed anchor has been collected for, proven and not kept: a peer serves the
// ledger as of that block with a receipt from its body to the block's state
// tree root (#4361), and that root is the one the anchor a quorum of the
// partition's validators signed carries. Nothing is pulled first and nothing
// is written: the ledger the node keeps is the one the pull brings.
//
// Each of the partition's peers is asked in turn until one answer proves. An
// answer that does not prove is that peer's failure and is said; NotReady
// when no anchor is collected yet or no peer's answer proves.
func (s *PulledState) ProvenLedger(ctx context.Context) (*protocol.SystemLedger, error) {
	if err := s.readAnchors(ctx); err != nil && !(errors.Is(err, errors.NotReady) && s.spineStalled()) {
		s.log.Debug("This partition's anchors could not be read", "partition", s.partition, "error", err)
	}
	block := s.tracker.LatestObservedBlock()
	root, ok := s.tracker.AnchorAt(block)
	if block == 0 || !ok {
		return nil, errors.NotReady.WithFormat("no signed anchor of %v has been collected yet", s.partition)
	}

	u := s.partition.JoinPath(protocol.Ledger)
	query := &api.DefaultQuery{IncludeReceipt: &api.ReceiptOptions{ForHeight: block}}
	var last error
	for i, src := range s.partitionSources(ctx) {
		rec, err := src.QueryAccount(ctx, u, query)
		if err == nil {
			var ledger *protocol.SystemLedger
			ledger, err = provenLedger(rec, u, block, root)
			if err == nil {
				return ledger, nil
			}
		}
		if ctx.Err() != nil {
			return nil, errors.UnknownError.Wrap(ctx.Err())
		}
		last = err
		s.log.Info("A peer's ledger as of a signed anchor's block did not prove; asking the next",
			"partition", s.partition, "block", block, "peer", i, "error", err)
	}
	if last == nil {
		return nil, errors.NotReady.WithFormat("no peer of %v was found to ask for its ledger as of block %d", s.partition, block)
	}
	return nil, errors.NotReady.WithFormat("no peer of %v proved its ledger as of block %d: %w", s.partition, block, last)
}

// provenLedger checks a peer's answer for the system ledger u as of block
// against root, the state tree root block's signed anchor carries, and
// returns the ledger. The receipt must be for that block, start at the hash
// of the body served, check, and end at root; the body must be the ledger of
// that block, served by a peer that is not far past it (SeedWithin).
func provenLedger(rec *api.AccountRecord, u *url.URL, block uint64, root [32]byte) (*protocol.SystemLedger, error) {
	if rec == nil || rec.Account == nil || rec.Receipt == nil {
		return nil, errors.Conflict.WithFormat("%v: the peer served no body or no receipt", u)
	}
	ledger, ok := rec.Account.(*protocol.SystemLedger)
	if !ok || ledger.Url == nil || !ledger.Url.Equal(u) {
		return nil, errors.Conflict.WithFormat("%v: the peer served the body of %v, a %v", u, rec.Account.GetUrl(), rec.Account.Type())
	}
	r := rec.Receipt
	if r.ForHeight != block || ledger.Index != block {
		return nil, errors.Conflict.WithFormat("%v: asked as of block %d, the peer served a receipt for block %d and the ledger of block %d",
			u, block, r.ForHeight, ledger.Index)
	}
	body, err := ledger.MarshalBinary()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("marshal the served ledger: %w", err)
	}
	h := sha256.Sum256(body)
	if !r.StartsAtMainState || !bytes.Equal(h[:], r.Start) {
		return nil, errors.Conflict.WithFormat("%v: the receipt for block %d does not start at the body served", u, block)
	}
	if !r.Validate(nil) || !bytes.Equal(r.Anchor, root[:]) {
		return nil, errors.Conflict.WithFormat("%v: the receipt for block %d does not end at the root that block's signed anchor carries", u, block)
	}
	if r.LocalBlock > block+SeedWithin {
		return nil, errors.NotReady.WithFormat("%v: the newest signed anchor collected is of block %d and the peer is at block %d, more than %d past it",
			u, block, r.LocalBlock, SeedWithin)
	}
	return ledger, nil
}
