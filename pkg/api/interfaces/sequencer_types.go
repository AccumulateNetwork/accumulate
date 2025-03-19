// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package interfaces

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// MessageRecordExt extends MessageRecord with additional fields needed for full message processing
type MessageRecordExt struct {
	*MessageRecord
	Source          *url.URL `json:"source,omitempty"`
	Destination     *url.URL `json:"destination,omitempty"`
	TransactionHash []byte   `json:"transactionHash,omitempty"`
	Message         []byte   `json:"message,omitempty"`
}

// PrivateSequenceRequest defines a request to sequence a message between source and destination
type PrivateSequenceRequest struct {
	Source         *url.URL `json:"source,omitempty" validate:"required"`
	Destination    *url.URL `json:"destination,omitempty" validate:"required"`
	SequenceNumber uint64   `json:"sequenceNumber,omitempty" validate:"required"`
	SequenceOptions
}

// PrivateSequenceResponse defines the response to a sequence request
type PrivateSequenceResponse struct {
	Record *MessageRecordExt `json:"record,omitempty"`
}
