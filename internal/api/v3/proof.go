// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
}

var _ apiv3.ProofService = (*ProofService)(nil)

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
