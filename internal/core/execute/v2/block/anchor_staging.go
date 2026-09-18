// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"strings"
)

// Anchor staging (executor spec, "Anchor staging"). A collection proof names
// the Directory anchor block it terminates in. If that anchor has executed
// here the proof is decided at intake; otherwise it waits under that block
// until the anchor executes, and the anchor validates or disproves it. A
// validated proof marks its range proven; a disproved one is discarded and
// counted. Nothing about a proof is recorded as pending anywhere else.

// intakeProof is the intake of one collection proof from source. It refuses an
// invalid list, decides a proof whose anchor already executed, and stages the
// rest. A proof that does not name its anchor is left to its message executor
// (the pre-#4217 paths, retired with H8).
func (b *Block) intakeProof(source *url.URL, proof *protocol.AnnotatedReceipt, siblings [][]byte) error {
	list := proof.ReceiptList
	if list == nil || proof.Receipt != nil ||
		len(list.Elements) > protocol.MaxReceiptListElements ||
		!list.Validate(nil) {
		mExecStagedProofs.WithLabelValues("invalid").Inc()
		return errors.BadRequest.With("proof is invalid")
	}
	if proof.Anchor == nil || proof.Anchor.SourceBlock == 0 {
		return nil
	}

	// A proof is bound to the source it is staged under by the messages it
	// travels with: it must cover the hash of a sequenced message FROM that
	// source in the same envelope. A sequenced message's hash commits to its
	// source, so a proof lifted from another partition's package cannot be
	// made to cover one, and cannot poison this source's proven set.
	bound := false
	for _, h := range siblings {
		if list.Included(h) {
			bound = true
			break
		}
	}
	if !bound {
		mExecStagedProofs.WithLabelValues("unbound").Inc()
		return errors.BadRequest.WithFormat("proof covers no message from %v in its envelope", source)
	}

	_, ok, err := b.Executor.provingAnchorIndex(b.Batch, proof)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if ok {
		return b.proofValidated(source, proof)
	}

	executed, err := b.directoryAnchorBlock()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if proof.Anchor.SourceBlock <= executed {
		// The anchor it names has executed and does not carry its root:
		// "never", not "not yet".
		mExecStagedProofs.WithLabelValues("disproved").Inc()
		return nil
	}
	if proof.Anchor.SourceBlock > executed+maxAnchorAhead {
		// Further ahead than the Directory could plausibly be: refused, not
		// held (the sanity horizon, executor spec "Validity").
		mExecStagedProofs.WithLabelValues("refused").Inc()
		return errors.BadRequest.WithFormat("proof names Directory block %d, %d past the newest executed", proof.Anchor.SourceBlock, proof.Anchor.SourceBlock-executed)
	}

	if n := b.staging.StagedProofBytes(source); n >= maxStagedProofBytes() {
		mExecStagedProofs.WithLabelValues("refused").Inc()
		if b.proofBudgetBound == nil {
			b.proofBudgetBound = map[string]bool{}
		}
		b.proofBudgetBound[strings.ToLower(source.String())] = true
		return errors.BadRequest.WithFormat("anchor staging for %v already holds %d bytes of proofs", source, n)
	}
	if !b.staging.StageProof(source, proof.Anchor.SourceBlock, proof) {
		// The same proof already waits under that block: every copy of a
		// package's members carries it, and one is enough
		mExecStagedProofs.WithLabelValues("duplicate").Inc()
		return nil
	}
	mExecStagedProofs.WithLabelValues("staged").Inc()
	return nil
}

// maxAnchorAhead bounds how far past the newest executed Directory anchor a
// proof may claim to be anchored: about an hour of Directory blocks. Beyond it
// the proof is refused rather than held.
const maxAnchorAhead = 3600

// maxStagedProofBytes is execute.MaxStagedProofBytes: the budget lives with
// staging because both ends of it are staging's — this node's intake bounds
// what a source may stage here, and Staging.Load bounds what a peer's
// snapshot may bring in (#4291 review).
func maxStagedProofBytes() int { return execute.MaxStagedProofBytes }

// validateStagedProofs runs after the anchor group has executed: every
// Directory anchor this block executed decides the proofs waiting on its
// block and on any earlier block still waiting (anchors arrive in order).
//
// A Directory anchor also makes every stream that holds anything worth
// re-evaluating this block: a collected entry it proves, or an entry held on
// its own receipt under that anchor, must drain now even if nothing new
// arrived on its stream (executor spec, "Sort, then four groups").
func (b *Block) validateStagedProofs(c *classified) error {
	for ; b.proofsValidatedThrough < len(b.State.ReceivedAnchors); b.proofsValidatedThrough++ {
		r := b.State.ReceivedAnchors[b.proofsValidatedThrough]
		if r.Partition != protocol.Directory || r.Body == nil {
			continue
		}
		err := b.decideProofs(c, r.Body.GetPartitionAnchor().MinorBlockIndex)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	return nil
}

// decideProofs decides every proof waiting on a Directory anchor block at or
// below `through`: validated against the anchor chain as it stands, or
// discarded. It is the anchor group's work in a block, and the join's at the
// block whose state it pulled (#4292, SettleStaging) -- one statement of the
// rule, so the two cannot decide a proof differently.
//
// `c` is this block's classification, or nil outside a block: a validated
// proof can make a held stream runnable, so a block re-evaluates every stream
// with proofs waiting, and a join runs nothing at all.
func (b *Block) decideProofs(c *classified, through uint64) error {
	sources := b.staging.ProofSources()
	if c != nil {
		for _, source := range sources {
			c.addStream(stream{kind: streamSynthetic, ledger: b.Executor.Describe.Synthetic(), source: source})
		}
	}
	for _, source := range sources {
		for _, blk := range b.staging.ProofBlocks(source) {
			if blk > through {
				continue
			}
			for _, p := range b.staging.Proofs(source, blk) {
				_, ok, err := b.Executor.provingAnchorIndex(b.Batch, p)
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
				if !ok {
					mExecStagedProofs.WithLabelValues("disproved").Inc()
					continue
				}
				err = b.proofValidated(source, p)
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
			}
			b.staging.DropProofs(source, blk)
		}
	}
	return nil
}

func (b *Block) proofValidated(source *url.URL, proof *protocol.AnnotatedReceipt) error {
	err := b.staging.Prove(b.Executor.synthStream(source), proof.ReceiptList)
	switch {
	case errors.Is(err, errors.Conflict):
		// Anchored, and contradicting what an earlier proof proved: an attack
		// on the stream, not an error the block acts on. Counted; a validator
		// signature on proofs is the eventual answer (executor spec, "Proof").
		mExecStagedProofs.WithLabelValues("conflict").Inc()
		return nil
	case err != nil:
		return errors.UnknownError.Wrap(err)
	}
	mExecStagedProofs.WithLabelValues("validated").Inc()
	return nil
}

// noteAnchorDelivered takes an anchor copy's word on what its source has
// executed of this partition's anchor stream to it, and releases those
// anchors from the producer cache when the block commits (healing spec, "The
// cache"). Called where the copy's signature is recorded, so the signer has
// already been checked as a current validator of the source and the signature
// verified (BlockAnchor.check): the word is taken on the signer's authority,
// as a synthetic's Delivered is. A produced anchor goes to every destination
// under one number, so the cache releases it when the last of them has said
// so: one destination for a BVN, every partition for the Directory.
func noteAnchorDelivered(ctx *MessageContext, seq *messaging.SequencedMessage, delivered uint64) {
	if ctx.Block == nil || ctx.Block.cache == nil || seq == nil || seq.Source == nil || delivered == 0 {
		return
	}
	fanout := 1
	if ctx.Executor.Describe.NetworkType == protocol.PartitionTypeDirectory {
		if g := ctx.GetActiveGlobals(); g != nil && g.Network != nil {
			fanout = len(g.Network.Partitions)
		}
	}
	ctx.Block.cache.ReleaseAnchors(seq.Source, delivered, fanout)
}

// directoryAnchorBlock is the Directory block of the newest Directory anchor
// executed here, or zero.
func (b *Block) directoryAnchorBlock() (uint64, error) {
	v, err := b.Batch.Account(b.Executor.Describe.AnchorPool()).DirectoryAnchorBlock().Get()
	switch {
	case errors.Is(err, errors.NotFound):
		return 0, nil
	case err != nil:
		return 0, errors.UnknownError.WithFormat("load directory anchor block: %w", err)
	}
	return v, nil
}
