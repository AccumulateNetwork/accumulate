// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package private types.yml

const ServiceTypeSequencer api.ServiceType = 0xF001

type Sequencer interface {
	Sequence(ctx context.Context, src, dst *url.URL, num uint64, opts SequenceOptions) (*api.MessageRecord[messaging.Message], error)
}

// SequenceRanger is an optional extension of [Sequencer] that serves a
// contiguous range of messages with a single collection proof (#4087). The
// proof — a merkle.ReceiptList covering every message of the range — is set as
// SourceReceiptList on the last record.
//
// It is optional so that a peer which does not serve ranges simply does not
// implement it, and the caller falls back to per-message [Sequencer.Sequence]
// calls rather than failing. That is what lets range recovery roll out without
// requiring every node to support it first.
type SequenceRanger interface {
	Sequencer
	SequenceRange(ctx context.Context, src, dst *url.URL, start, end uint64, opts SequenceOptions) ([]*api.MessageRecord[messaging.Message], error)
}

// MajorHeaderRanger is an optional extension of [Sequencer] that serves the
// major-block spine (#4058): for each major block in the range, the
// major-block index chain entry plus the partition's self-anchor for the
// minor block that closed it, with the archived validator-quorum signatures.
// A fast-syncing node walks these records from its trust anchor to the
// present, verifying each quorum against the validator set tracked by
// induction. Only the directory partition serves this — it is the only
// partition that anchors to itself.
type MajorHeaderRanger interface {
	Sequencer
	MajorHeaderRange(ctx context.Context, partition *url.URL, start, end uint64, opts SequenceOptions) ([]*MajorHeaderRecord, error)
}

// MinorRootRanger is an optional extension of [Sequencer] that binds minor
// blocks past the spine to it (#4058). Since is the client's last verified
// minor block — the block of its last verified self-anchor. Until is the
// target block, or zero for as far as possible. The server returns a record
// for the furthest anchored block it can prove in one receipt list; the
// client verifies and calls again from the new position until it reaches the
// tip. Only the directory serves this.
type MinorRootRanger interface {
	Sequencer
	MinorRootRange(ctx context.Context, partition *url.URL, since, until uint64, opts SequenceOptions) (*MinorRootRecord, error)
}
