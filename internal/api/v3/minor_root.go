// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var _ private.MinorRootRanger = (*Sequencer)(nil)

// MinorRootRange implements [private.MinorRootRanger.MinorRootRange]: it
// binds minor blocks past the client's verified position to the spine
// (#4058). Since must be the block of a self-anchor the client has verified.
// The response carries the self-anchor of the furthest block provable in one
// receipt list (capped by until if nonzero), its archived quorum, the
// window's network updates, and a receipt list proving the root chain
// extends the root at since to this anchor's root.
func (s *Sequencer) MinorRootRange(ctx context.Context, partition *url.URL, since, until uint64, _ private.SequenceOptions) (*private.MinorRootRecord, error) {
	if partition == nil {
		return nil, errors.BadRequest.With("missing partition")
	}
	if until != 0 && until <= since {
		return nil, errors.BadRequest.WithFormat("invalid range (%d, %d]", since, until)
	}
	if !s.partition.URL.Equal(partition) {
		return nil, errors.BadRequest.WithFormat("requested partition is %s but this partition is %s", partition, s.partitionID)
	}
	if !protocol.DnUrl().Equal(partition) {
		return nil, errors.BadRequest.With("minor roots are only served for the directory")
	}

	var r *private.MinorRootRecord
	var err error
	return r, s.db.View(func(batch *database.Batch) error {
		r, err = s.getMinorRootRange(batch, since, until)
		return err
	})
}

func (s *Sequencer) getMinorRootRange(batch *database.Batch, since, until uint64) (*private.MinorRootRecord, error) {
	account := batch.Account(s.partition.AnchorPool())
	seqChain, err := account.AnchorSequenceChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor sequence chain: %w", err)
	}
	height := uint64(seqChain.Height())
	if height == 0 {
		return nil, errors.NotFound.With("no anchors exist")
	}

	// The client's position must be an anchored block it verified. Zero
	// means the client is starting from genesis — a young network with no
	// major blocks yet (#4058): the proof then covers the root chain from
	// its beginning and the anchor's quorum carries the trust alone.
	var sinceNum uint64
	sinceRoot := int64(-1)
	if since > 0 {
		var sinceBody protocol.AnchorBody
		var err error
		sinceBody, sinceNum, err = s.findSelfAnchorForBlock(batch, seqChain, since)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("locate anchor for block %d: %w", since, err)
		}
		if sinceBody.GetPartitionAnchor().MinorBlockIndex != since {
			return nil, errors.BadRequest.WithFormat("block %d is not an anchored block", since)
		}
		sinceRoot = int64(sinceBody.GetPartitionAnchor().RootChainIndex)
	}

	// Resolve the target anchor
	var endBody protocol.AnchorBody
	var endNum uint64
	if until == 0 {
		// The newest anchors have not been executed and signed yet — serve
		// the newest one whose archived quorum is complete
		endNum, endBody, err = s.findLatestQuorumAnchor(batch, seqChain, sinceNum)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	} else {
		endBody, endNum, err = s.findSelfAnchorForBlock(batch, seqChain, until)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("locate anchor for block %d: %w", until, err)
		}
	}
	if endBody.GetPartitionAnchor().MinorBlockIndex <= since {
		return nil, errors.NotReady.WithFormat("no anchored block past %d", since)
	}

	// Cap the chunk at one receipt list. RootChainIndex increases
	// monotonically along the sequence chain, so a binary search finds the
	// furthest anchor within the cap.
	if int64(endBody.GetPartitionAnchor().RootChainIndex)-sinceRoot > protocol.MaxReceiptListElements {
		limit := uint64(sinceRoot + protocol.MaxReceiptListElements)
		var searchErr error
		i := sort.Search(int(height), func(i int) bool {
			if searchErr != nil {
				return true
			}
			body, err := s.loadAnchorBody(batch, seqChain, uint64(i)+1)
			if err != nil {
				searchErr = err
				return true
			}
			return body.GetPartitionAnchor().RootChainIndex > limit
		})
		if searchErr != nil {
			return nil, errors.UnknownError.Wrap(searchErr)
		}
		endNum = uint64(i) // 1-based: anchors 1..i are within the limit
		if endNum <= sinceNum {
			return nil, errors.Conflict.WithFormat("cannot advance: the block after %d exceeds %d root entries", since, protocol.MaxReceiptListElements)
		}
		endBody, err = s.loadAnchorBody(batch, seqChain, endNum)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		endBody = endBody.CopyAsInterface().(protocol.AnchorBody)
	}
	end := endBody.GetPartitionAnchor()

	// Prove the root chain extends the client's verified root to this
	// anchor's root
	rootChain := batch.Account(s.partition.Ledger()).RootChain()
	proof, err := merkle.GetReceiptList(rootChain.Inner(), sinceRoot+1, int64(end.RootChainIndex))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("build root proof (%d, %d]: %w", sinceRoot, end.RootChainIndex, err)
	}

	updates, err := s.getNetworkUpdatesInWindow(batch, since, end)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("collect network updates: %w", err)
	}

	seq, sigs, err := s.buildSelfAnchor(batch, endBody, endNum)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Do not serve a record the client cannot verify — recently executed
	// anchors accumulate their quorum over a few blocks
	if globals := s.globals.Load().(*core.GlobalValues); globals != nil {
		if uint64(len(sigs)) < globals.ValidatorThreshold(s.partitionID) {
			return nil, errors.NotReady.WithFormat("the anchor for block %d does not have a quorum yet", end.MinorBlockIndex)
		}
	}

	return &private.MinorRootRecord{
		Anchor:     seq,
		Signatures: sigs,
		Updates:    updates,
		RootProof:  proof,
	}, nil
}

// findLatestQuorumAnchor walks back from the newest anchor to the newest one
// whose archived validator signatures meet the current threshold. Anchors are
// executed (and signed) a few blocks after they are produced, so only the
// last few lack a quorum.
func (s *Sequencer) findLatestQuorumAnchor(batch *database.Batch, seqChain *database.Chain, after uint64) (uint64, protocol.AnchorBody, error) {
	globals := s.globals.Load().(*core.GlobalValues)
	if globals == nil {
		return 0, nil, errors.NotReady
	}
	threshold := globals.ValidatorThreshold(s.partitionID)

	for num := uint64(seqChain.Height()); num > after; num-- {
		body, err := s.loadAnchorBody(batch, seqChain, num)
		if err != nil {
			return 0, nil, err
		}
		body = body.CopyAsInterface().(protocol.AnchorBody)
		_, sigs, err := s.buildSelfAnchor(batch, body.CopyAsInterface().(protocol.AnchorBody), num)
		if err != nil {
			return 0, nil, err
		}
		if uint64(len(sigs)) >= threshold {
			return num, body, nil
		}
	}
	return 0, nil, errors.NotReady.WithFormat("no anchor past block %d has a quorum yet", after)
}

// loadAnchorBody loads the body of the numbered anchor from the sequence
// chain.
func (s *Sequencer) loadAnchorBody(batch *database.Batch, seqChain *database.Chain, num uint64) (protocol.AnchorBody, error) {
	hash, err := seqChain.Entry(int64(num) - 1)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor sequence chain entry %d: %w", num-1, err)
	}
	var msg messaging.MessageWithTransaction
	err = batch.Message2(hash).Main().GetAs(&msg)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor %d: %w", num, err)
	}
	body, ok := msg.GetTransaction().Body.(protocol.AnchorBody)
	if !ok {
		return nil, errors.InternalError.WithFormat("anchor %d is not an anchor body", num)
	}
	return body, nil
}
