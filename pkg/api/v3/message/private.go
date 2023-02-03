// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Sequencer forwards [PrivateSequenceRequest]s to a [private.Sequencer].
type Sequencer struct {
	private.Sequencer
}

func (s *Sequencer) methods() serviceMethodMap {
	typ, fn := makeServiceMethod(s.sequence)
	return serviceMethodMap{typ: fn}
}

func (s *Sequencer) sequence(c *call[*PrivateSequenceRequest]) {
	res, err := s.Sequencer.Sequence(c.context, c.params.Source, c.params.Destination, c.params.SequenceNumber)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}
	c.Write(&PrivateSequenceResponse{Value: res})
}

// PrivateClient is a binary message transport client for private API v3 services.
type PrivateClient Client

// Private returns a [PrivateClient].
func (c *Client) Private() private.Sequencer {
	return (*PrivateClient)(c)
}

// Sequence implements [private.Sequencer.Sequence].
func (c *PrivateClient) Sequence(ctx context.Context, src, dst *url.URL, num uint64) (*api.TransactionRecord, error) {
	req := &PrivateSequenceRequest{Source: src, Destination: dst, SequenceNumber: num}
	return typedRequest[*PrivateSequenceResponse, *api.TransactionRecord]((*Client)(c), ctx, req)
}