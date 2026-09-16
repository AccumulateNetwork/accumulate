// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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

	// A proof that terminates in the SOURCE's own root, not a Directory
	// root: sent at begin(N+1) without waiting for the Directory (#4253).
	// It cannot be decided against the Directory anchor chain yet -- it has
	// to be extended by the Directory's receipt for that source block
	// first. Held under the SOURCE block, in its own space.
	if !protocol.DnUrl().Equal(proof.Anchor.Account) {
		return b.intakeSourceProof(source, proof)
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

	if blocks := b.staging.ProofBlocks(source); len(blocks) >= maxStagedProofBlocks {
		mExecStagedProofs.WithLabelValues("refused").Inc()
		return errors.BadRequest.WithFormat("anchor staging for %v already waits on %d blocks", source, len(blocks))
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

// intakeSourceProof holds a proof that ends at the source's own root until a
// Directory anchor receipting that source block arrives to extend it. The
// proof is not verifiable yet -- its terminal is a value this partition has
// no reason to know -- so nothing is decided here beyond the shape checks the
// caller already made and the bound below.
//
// The annotation must name the partition the messages came from. A proof
// claiming to end in some OTHER partition's root would be extended with that
// partition's receipt, which is a proof of nothing about this source.
func (b *Block) intakeSourceProof(source *url.URL, proof *protocol.AnnotatedReceipt) error {
	if !source.LocalTo(proof.Anchor.Account) {
		mExecStagedProofs.WithLabelValues("unbound").Inc()
		return errors.BadRequest.WithFormat("proof from %v claims to be anchored in %v", source, proof.Anchor.Account)
	}
	if blocks := b.staging.SourceProofBlocks(source); len(blocks) >= maxStagedProofBlocks {
		mExecStagedProofs.WithLabelValues("refused").Inc()
		return errors.BadRequest.WithFormat("source-anchor staging for %v already waits on %d blocks", source, len(blocks))
	}
	if !b.staging.StageSourceProof(source, proof.Anchor.SourceBlock, proof) {
		mExecStagedProofs.WithLabelValues("duplicate").Inc()
		return nil
	}
	mExecStagedProofs.WithLabelValues("staged-source").Inc()
	return nil
}

// extendSourceProofs completes the source-anchored proofs a Directory anchor
// decides. The anchor carries one receipt per source block it anchored, from
// that block's root to the Directory root; appending it to a proof that ends
// at the same root yields a proof to the Directory root, which is what this
// partition can verify (#4253).
//
// The receipt is taken from an anchor this node executed and validated
// itself, so the completion adds no trust the destination did not already
// place in that anchor.
func (b *Block) extendSourceProofs(anchor *protocol.DirectoryAnchor) error {
	for _, r := range anchor.Receipts {
		src := r.Anchor.Source
		if src == nil || r.RootChainReceipt == nil {
			continue
		}
		srcBlock := r.Anchor.MinorBlockIndex
		staged := b.staging.SourceProofs(src, srcBlock)
		if len(staged) == 0 {
			continue
		}
		for _, p := range staged {
			done, err := b.completeSourceProof(p, r)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			if done == nil {
				continue
			}
			err = b.proofValidated(src, done)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}
		b.staging.DropSourceProofs(src, srcBlock)
	}
	return nil
}

// completeSourceProof appends the Directory's receipt for the source block to
// a proof that ends at that block's root, and answers with the completed
// proof when the result is valid and lands on an anchor this partition has
// executed.
//
// It does NOT modify the staged proof. A staged proof may live in committed
// staging, shared by every block that runs after it, and a block that is
// discarded must leave nothing behind: an extension written in place would
// survive the discard, and the next block's attempt to extend it would find
// a proof that no longer continues from the receipt and disprove it -- losing
// entries that were never in doubt. The extension is built on a copy.
func (b *Block) completeSourceProof(p *protocol.AnnotatedReceipt, r *protocol.PartitionAnchorReceipt) (*protocol.AnnotatedReceipt, error) {
	if p.ReceiptList == nil || p.ReceiptList.ContinuedReceipt == nil {
		mExecStagedProofs.WithLabelValues("disproved").Inc()
		return nil, nil
	}
	extended, err := p.ReceiptList.ContinuedReceipt.Combine(r.RootChainReceipt)
	if err != nil {
		// The proof does not continue from this receipt: it claimed a block
		// whose root is not the one the Directory anchored.
		mExecStagedProofs.WithLabelValues("disproved").Inc()
		return nil, nil
	}

	list := *p.ReceiptList
	list.ContinuedReceipt = extended
	cp := &protocol.AnnotatedReceipt{ReceiptList: &list, Anchor: p.Anchor}
	if !list.Validate(nil) {
		mExecStagedProofs.WithLabelValues("disproved").Inc()
		return nil, nil
	}
	// The extended proof must land on an anchor this partition has actually
	// executed; the receipt came from one, so this is the check that the two
	// are the same anchor.
	_, ok, err := b.Executor.provingAnchorIndex(b.Batch, cp)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !ok {
		mExecStagedProofs.WithLabelValues("disproved").Inc()
		return nil, nil
	}
	mExecStagedProofs.WithLabelValues("extended").Inc()
	return cp, nil
}

// maxAnchorAhead bounds how far past the newest executed Directory anchor a
// proof may claim to be anchored: about an hour of Directory blocks. Beyond it
// the proof is refused rather than held.
const maxAnchorAhead = 3600

// maxStagedProofBlocks bounds how many Directory blocks one source may have
// proofs waiting on. Honest traffic waits on a handful — the anchors a block
// or two ahead — so the bound only ever binds on a flood.
const maxStagedProofBlocks = 256

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
		through := r.Body.GetPartitionAnchor().MinorBlockIndex

		// Read the sources BEFORE anything is decided and dropped. A proof
		// that validates here makes its stream runnable, and the stream is
		// only re-evaluated this block if it is named now: deciding first
		// would drop the proof, take the source off this list, and leave
		// the entries it just proved held until something else happened to
		// touch that stream.
		sources := b.staging.ProofSources()
		if c != nil {
			// A validated proof can make a held stream runnable, so every
			// stream with proofs waiting is evaluated this block
			for _, source := range sources {
				c.addStream(stream{kind: streamSynthetic, ledger: b.Executor.Describe.Synthetic(), source: source})
			}
		}

		// This anchor carries a receipt for each source block it anchored;
		// those receipts complete the source-anchored proofs waiting on
		// them (#4253).
		if da, ok := r.Body.(*protocol.DirectoryAnchor); ok {
			err := b.extendSourceProofs(da)
			if err != nil {
				return errors.UnknownError.Wrap(err)
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
