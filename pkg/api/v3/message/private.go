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

// ServiceTypePrivate defines the service type ID for private services
const ServiceTypePrivate uint32 = 0xF000

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

	// Convert interfaces.MessageRecord to api.MessageRecord for the response
	txID, err := url.ParseTxID(res.ID)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}
	
	apiRecord := &api.MessageRecord[messaging.Message]{
		ID:     txID,
		Status: errors.Pending, // Use pending status as default
	}
	
	// Use the proper message type for the response with the converted record
	c.Write(&PrivateSequenceResponse{Value: apiRecord})
}

// PrivateClient implements private.Sequencer by adapting an AddressedClient
type PrivateClient struct {
	AddressedClient
}

// Request implements interfaces.RequestHandler
func (c PrivateClient) Request(ctx context.Context, req interface{}, resp interface{}) error {
	return c.AddressedClient.Request(ctx, req, resp)
}

// Private returns a [private.Sequencer].
func (c *Client) Private() private.Sequencer {
	client := PrivateClient{c.ForAddress(nil)}
	return &interfaces.SequencerAdapter{Handler: client}
}

// Private returns a [private.Sequencer].
func (c AddressedClient) Private() private.Sequencer {
	client := PrivateClient{c}
	return &interfaces.SequencerAdapter{Handler: client}
}
