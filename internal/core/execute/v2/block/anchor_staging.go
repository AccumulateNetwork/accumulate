// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
func (b *Block) intakeProof(source *url.URL, proof *protocol.AnnotatedReceipt) error {
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

	acct := b.Batch.Account(b.Executor.Describe.Synthetic())
	err = acct.StagedProofs(source, proof.Anchor.SourceBlock).Add(proof)
	if err != nil {
		return errors.UnknownError.WithFormat("stage proof: %w", err)
	}
	err = acct.StagedProofBlocks(source).Add(proof.Anchor.SourceBlock)
	if err != nil {
		return errors.UnknownError.WithFormat("stage proof block: %w", err)
	}
	err = acct.StagedSources().Add(source)
	if err != nil {
		return errors.UnknownError.WithFormat("record staged source: %w", err)
	}
	mExecStagedProofs.WithLabelValues("staged").Inc()
	return nil
}

// validateStagedProofs runs after the anchor group has executed: every
// Directory anchor this block executed decides the proofs waiting on its
// block and on any earlier block still waiting (anchors arrive in order).
func (b *Block) validateStagedProofs() error {
	for ; b.proofsValidatedThrough < len(b.State.ReceivedAnchors); b.proofsValidatedThrough++ {
		r := b.State.ReceivedAnchors[b.proofsValidatedThrough]
		if r.Partition != protocol.Directory || r.Body == nil {
			continue
		}
		through := r.Body.GetPartitionAnchor().MinorBlockIndex
		acct := b.Batch.Account(b.Executor.Describe.Synthetic())

		sources, err := acct.StagedSources().Get()
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		for _, source := range sources {
			blocks, err := acct.StagedProofBlocks(source).Get()
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			for _, blk := range blocks {
				if blk > through {
					continue
				}
				proofs, err := acct.StagedProofs(source, blk).Get()
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
				for _, p := range proofs {
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
				err = acct.StagedProofs(source, blk).Put(nil)
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
				err = acct.StagedProofBlocks(source).Remove(blk)
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
			}
		}
	}
	return nil
}

// proofValidated marks a proof's range proven. Until proven ranges by index
// replace it (E8 step 3) the proven set is the synthetic replica.
func (b *Block) proofValidated(source *url.URL, proof *protocol.AnnotatedReceipt) error {
	err := b.Executor.seedSyntheticReplica(b.Batch, source, proof.ReceiptList)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	mExecStagedProofs.WithLabelValues("validated").Inc()
	return nil
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
