// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/interfaces"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Sequencer forwards private sequence requests to a [private.Sequencer].
type Sequencer struct {
	private.Sequencer
}

func (s Sequencer) methods() serviceMethodMap {
	typ, fn := makeServiceMethod(s.sequence)
	return serviceMethodMap{typ: fn}
}

// sequence handles incoming sequence requests
func (s Sequencer) sequence(c *call[*interfaces.PrivateSequenceRequest]) {
	// Use the interface type for sequence options
	opts := interfaces.SequenceOptions{
		NodeID: c.params.SequenceOptions.NodeID,
	}

	// Call the sequencer implementation via the interface
	res, err := s.Sequencer.Sequence(c.context, c.params.Source, c.params.Destination, c.params.SequenceNumber, opts)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}

	// Convert the message record to the appropriate response type
	msgID, err := url.ParseTxID(res.ID)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}

	// Create a MessageRecord with the minimum required fields
	record := &api.MessageRecord[messaging.Message]{
		ID:              res.ID,
		Status:          api.MessageStatusDelivered,
		Source:          res.Source,
		Destination:     res.Destination,
		SequenceNumber:  res.SequenceNumber,
		TransactionHash: res.TransactionHash,
		Message:         res.Message,
	}

	c.Write(&interfaces.PrivateSequenceResponse{Record: res})
}

// PrivateClient is a binary message transport client for private API v3 services.
type PrivateClient struct {
	*addressedClient
}

// Private returns a [PrivateClient].
func (c *Client) Private() private.Sequencer {
	return PrivateClient{c.addressed(ServiceTypePrivate)}
}

// Private returns a [PrivateClient].
func (c *addressedClient) Private() private.Sequencer {
	return PrivateClient{c}
}

// Sequence implements [private.Sequencer.Sequence].
func (c PrivateClient) Sequence(ctx context.Context, src, dst *url.URL, num uint64, opts interfaces.SequenceOptions) (*interfaces.MessageRecord, error) {
	req := &interfaces.PrivateSequenceRequest{Source: src, Destination: dst, SequenceNumber: num, SequenceOptions: opts}
	resp := new(interfaces.PrivateSequenceResponse)
	err := c.Request(ctx, req, resp)
	if err != nil {
		return nil, err
	}
	return resp.Record, nil
}
