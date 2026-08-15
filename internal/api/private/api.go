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
