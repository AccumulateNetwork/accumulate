// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"
	"errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/generated"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/interfaces"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/model"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/transport"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

var ErrUnexpectedResponse = errors.New("unexpected response type")

// PrivateClient is a client for making private requests
type PrivateClient struct {
	*Client
	url *url.URL
}

// NewPrivateClient creates a new private client
func NewPrivateClient(transport transport.Transport, url *url.URL) *PrivateClient {
	return &PrivateClient{
		Client: NewClient(transport),
		url:    url,
	}
}

// GetURL returns the URL associated with the client
func (c *PrivateClient) GetURL() *url.URL {
	return c.url
}

// Request sends a request to the private client's URL
func (c *PrivateClient) Request(ctx context.Context, req interfaces.Message) (interfaces.Message, error) {
	// Set destination URL if request supports it
	if req, ok := req.(interfaces.AddressedMessage); ok {
		req.SetDestination(c.url)
	}

	return c.Client.Request(ctx, req)
}

// RequestSequence requests a sequence of messages
func (c *PrivateClient) RequestSequence(ctx context.Context, options *model.SequenceOptions) (*model.MessageRecord, error) {
	req := &generated.PrivateSequenceRequest{Options: options}
	resp, err := c.Request(ctx, req)
	if err != nil {
		return nil, err
	}

	seqResp, ok := resp.(*generated.PrivateSequenceResponse)
	if !ok {
		return nil, ErrUnexpectedResponse
	}

	return seqResp.GetRecord(), nil
}
