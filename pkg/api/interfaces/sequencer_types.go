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

// MessageRecord represents a basic message record
type MessageRecord struct {
	ID             *url.TxID `json:"id,omitempty"`
	SequenceNumber uint64    `json:"sequenceNumber,omitempty"`
}

// SequenceOptions defines options for sequence operations
type SequenceOptions struct {
	NodeID p2p.PeerID `json:"nodeID,omitempty"`
}

// PrivateSequenceRequest represents a request to sequence a private message
type PrivateSequenceRequest struct {
	Source          *url.URL       `json:"source,omitempty"`
	Destination     *url.URL       `json:"destination,omitempty"`
	SequenceNumber  uint64         `json:"sequenceNumber,omitempty"`
	SequenceOptions SequenceOptions `json:"options,omitempty"`
}

// PrivateSequenceResponse represents a response to a private sequence request
type PrivateSequenceResponse struct {
	Record *MessageRecord `json:"record,omitempty"`
}

// Sequencer defines the interface for private message sequencing
type Sequencer interface {
	Sequence(ctx context.Context, src, dst *url.URL, num uint64, opts SequenceOptions) (*MessageRecord, error)
}
