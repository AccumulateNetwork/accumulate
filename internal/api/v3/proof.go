// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ProofService serves the major-block spine (#4058) on the public v3 surface.
// It adds nothing of its own: both methods delegate to the Sequencer
// implementations the network already runs for node-to-node fast sync, so an
// external verifier runs the same induction the network runs on itself rather
// than a second one that can drift.
//
// Every refusal — a partition that does not serve the spine, a malformed range,
// a range beyond the last major block — comes from the sequencer unchanged.
type ProofService struct {
	// Ranger is whatever serves the spine node-to-node — the local Sequencer, or
	// a client to a directory node. Depending on the interface rather than the
	// concrete sequencer is what keeps this an adapter rather than a second
	// implementation.
	Ranger interface {
		private.MajorHeaderRanger
		private.MinorRootRanger
	}

	// Database is this partition's database. AnchorReceipt reads its bpt chain
	// and root chain to extend a BPT root to a root-chain anchor.
	Database database.Viewer

	// Directory resolves the directory's database, which AnchorReceipt needs to
	// bind that root-chain anchor to a directory root. Every Accumulate node
	// runs the directory alongside its own BVN, so both halves are local; it is
	// a function because the two partitions start independently.
	Directory func() (database.Viewer, error)

	// Partition describes this node.
	Partition config.NetworkUrl
}

var _ apiv3.ProofService = (*ProofService)(nil)

// Type implements [apiv3.Service.Type].
func (s *ProofService) Type() apiv3.ServiceType { return apiv3.ServiceTypeProof }

// MajorHeaderRange implements [apiv3.ProofService.MajorHeaderRange].
func (s *ProofService) MajorHeaderRange(ctx context.Context, opts apiv3.MajorHeaderRangeOptions) ([]*apiv3.MajorHeaderRecord, error) {
	if opts.Partition == "" {
		return nil, errors.BadRequest.With("missing partition")
	}
	r, err := s.Ranger.MajorHeaderRange(ctx, protocol.PartitionUrl(opts.Partition), opts.Start, opts.End, private.SequenceOptions{})
	return r, errors.UnknownError.Wrap(err)
}

// MinorRootRange implements [apiv3.ProofService.MinorRootRange].
func (s *ProofService) MinorRootRange(ctx context.Context, opts apiv3.MinorRootRangeOptions) (*apiv3.MinorRootRecord, error) {
	if opts.Partition == "" {
		return nil, errors.BadRequest.With("missing partition")
	}
	r, err := s.Ranger.MinorRootRange(ctx, protocol.PartitionUrl(opts.Partition), opts.Since, opts.Until, private.SequenceOptions{})
	return r, errors.UnknownError.Wrap(err)
}

// anchorSearchWindow bounds how far AnchorReceipt looks for a root-chain
// position the directory has already received. The directory is normally at
// most a few anchors behind, so the first candidate almost always answers; the
// window exists so a partition the directory has stopped following fails fast
// instead of walking its whole history.
const anchorSearchWindow = 64

// AnchorReceipt implements [apiv3.ProofService.AnchorReceipt] — the second of
// the two calls an account proof takes (#4274).
//
// A BPT is a tree of current state: every account that changes rewrites the
// path to the root, so an account cannot be proved against a past BPT. The
// proof is built against the current one, and that root reaches a directory
// root only by two hops, both of which this does:
//
//	extend  the root is an entry on the partition's bpt chain, and that chain
//	        is anchored into the partition's root chain, so the root proves
//	        into a root-chain anchor the partition actually sent
//	bind    that anchor is an entry on the directory's anchor(P)-root chain,
//	        so it proves into a directory root
//
// The extension is why this does not require the caller's root to have been
// anchored itself. Only some blocks send anchors, but every block's root is on
// the bpt chain, so every root is provable (#4276). Before the bpt chain was
// anchored, a root from a block that sent no anchor was unbindable forever.
//
// Both hops are local: every Accumulate node runs the directory alongside its
// own BVN.
//
// By default it returns the oldest receipt that works, which is stable — the
// same root asked for later returns the same receipt. AtOrAfter asks for one
// terminating at a later directory root.
func (s *ProofService) AnchorReceipt(ctx context.Context, opts apiv3.AnchorReceiptOptions) (*apiv3.AnchorReceiptRecord, error) {
	if opts.Partition == "" {
		return nil, errors.BadRequest.With("missing partition")
	}
	if opts.BptRoot == ([32]byte{}) {
		return nil, errors.BadRequest.With("missing BPT root")
	}
	if !strings.EqualFold(opts.Partition, s.Partition.PartitionID()) {
		return nil, errors.BadRequest.WithFormat(
			"cannot extend a %s root: this node serves %s", opts.Partition, s.Partition.PartitionID())
	}
	if s.Directory == nil {
		return nil, errors.NotReady.With("this node has no directory database")
	}
	dir, err := s.Directory()
	if err != nil {
		return nil, errors.NotReady.WithFormat("the directory is not running here: %w", err)
	}

	rec := new(apiv3.AnchorReceiptRecord)
	err = s.Database.View(func(pb *database.Batch) error {
		return dir.View(func(db *database.Batch) error {
			r, block, ok, err := s.extendAndBind(pb, db, opts)
			if err != nil || !ok {
				return err // Anchored stays false: the anchor has not arrived
			}
			rec.Receipt, rec.Anchored, rec.DirectoryBlock = r, true, block
			return nil
		})
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return rec, nil
}

// extendAndBind does the two hops. ok is false when the directory has not yet
// received an anchor covering the root -- a wait, not a failure.
func (s *ProofService) extendAndBind(pb, db *database.Batch, opts apiv3.AnchorReceiptOptions) (*merkle.Receipt, uint64, bool, error) {
	dn := config.NetworkUrl{URL: protocol.DnUrl()}

	// Whether this network anchors its bpt chain at all is asked FIRST: below
	// Kourou the chain is written but never anchored, so there is no answer to
	// wait for, and every later branch would report "not yet" forever -- the
	// same lie this call exists to stop telling (#4276).
	bpt := pb.Account(s.Partition.Ledger()).BptChain()
	bptIndex, err := bpt.Index().Get()
	if err != nil {
		return nil, 0, false, errors.UnknownError.WithFormat("load the bpt index chain: %w", err)
	}
	if bptIndex.Height() == 0 {
		return nil, 0, false, errors.NotReady.WithFormat(
			"%s does not anchor its bpt chain yet; proofs need Kourou", opts.Partition)
	}

	// The root must be one this partition produced.
	index, err := bpt.IndexOf(opts.BptRoot[:])
	switch {
	case err == nil:
		// Found
	case errors.Is(err, errors.NotFound):
		// The bpt chain records a block's root in the FOLLOWING block, so a
		// root that has only just been handed out is not an entry yet.
		return nil, 0, false, nil
	default:
		return nil, 0, false, errors.UnknownError.WithFormat("search the bpt chain: %w", err)
	}

	// The root-chain height this entry was anchored at. Anything earlier cannot
	// terminate the extension; the search starts here and runs forward, so the
	// answer is the oldest that works and therefore stable.
	_, be, err := indexing.SearchIndexChain(bptIndex, uint64(bptIndex.Height())-1,
		indexing.MatchAfter, indexing.SearchIndexChainBySource(uint64(index)))
	if err != nil {
		return nil, 0, false, errors.UnknownError.WithFormat("locate the bpt index entry: %w", err)
	}

	rootIndex, err := pb.Account(s.Partition.Ledger()).RootChain().Index().Get()
	if err != nil {
		return nil, 0, false, errors.UnknownError.WithFormat("load the root index chain: %w", err)
	}
	from, _, err := indexing.SearchIndexChain(rootIndex, uint64(rootIndex.Height())-1,
		indexing.MatchAfter, indexing.SearchIndexChainBySource(be.Anchor))
	if err != nil {
		return nil, 0, false, errors.UnknownError.WithFormat("locate the root index entry: %w", err)
	}

	// AtOrAfter names a DIRECTORY block, so it constrains the bind hop, not the
	// extension. Resolving it here rather than filtering the partition's own
	// blocks, which are a different numbering entirely.
	var bindTarget *uint64
	if opts.AtOrAfter > 0 {
		dnRootIndex, err := db.Account(dn.Ledger()).RootChain().Index().Get()
		if err != nil {
			return nil, 0, false, errors.UnknownError.WithFormat("load the directory root index chain: %w", err)
		}
		_, e, err := indexing.SearchIndexChain(dnRootIndex, uint64(dnRootIndex.Height())-1,
			indexing.MatchAfter, indexing.SearchIndexChainByBlock(opts.AtOrAfter))
		if err != nil {
			return nil, 0, false, errors.UnknownError.WithFormat(
				"no directory root at or after block %d: %w", opts.AtOrAfter, err)
		}
		bindTarget = &e.Source
	}

	received := db.Account(dn.AnchorPool()).AnchorChain(opts.Partition).Root()
	for n := uint64(0); n < anchorSearchWindow && from+n < uint64(rootIndex.Height()); n++ {
		entry := new(protocol.IndexEntry)
		raw, err := rootIndex.Entry(int64(from + n))
		if err != nil {
			return nil, 0, false, errors.UnknownError.WithFormat("load root index entry: %w", err)
		}
		if err := entry.UnmarshalBinary(raw); err != nil {
			return nil, 0, false, errors.UnknownError.WithFormat("decode root index entry: %w", err)
		}
		// EXTEND. The terminus is the partition's root-chain anchor at this
		// position -- the same value the anchor for that block carried.
		_, _, extend, err := indexing.ReceiptForChainIndex(s.Partition, pb, bpt, index, &entry.Source)
		if err != nil {
			return nil, 0, false, errors.UnknownError.WithFormat("extend to the root chain: %w", err)
		}

		// Has the directory received it? Its anchor(P)-root chain holds exactly
		// the root-chain anchors the partition has sent.
		at, err := received.IndexOf(extend.Anchor)
		switch {
		case err == nil:
			// Found
		case errors.Is(err, errors.NotFound):
			continue // Try the next position
		default:
			return nil, 0, false, errors.UnknownError.WithFormat("search anchor(%s)-root: %w", opts.Partition, err)
		}

		// BIND.
		block, _, bind, err := indexing.ReceiptForChainIndex(dn, db, received, at, bindTarget)
		if err != nil {
			return nil, 0, false, errors.UnknownError.WithFormat("bind to a directory root: %w", err)
		}
		joined, err := extend.Combine(bind)
		if err != nil {
			return nil, 0, false, errors.UnknownError.WithFormat("join the two hops: %w", err)
		}
		var at2 uint64
		if block != nil {
			at2 = block.BlockIndex
		}
		return joined, at2, true, nil
	}
	return nil, 0, false, nil
}
