// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package interfaces

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// SequenceOptions defines options for sequencing operations
type SequenceOptions struct {
	NodeID p2p.PeerID `json:"nodeID,omitempty"`
}

// MessageRecord represents a record of a message
type MessageRecord struct {
	SequenceNumber uint64 `json:"sequenceNumber,omitempty"`
	ID             string `json:"id,omitempty"`
}

// Sequencer defines the interface for sequencing operations
type Sequencer interface {
	Sequence(ctx context.Context, src, dst *url.URL, num uint64, opts SequenceOptions) (*MessageRecord, error)
}
