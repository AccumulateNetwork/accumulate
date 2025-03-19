// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/interfaces"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/model"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/transport"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

var ErrUnexpectedResponse = fmt.Errorf("unexpected response")

// A Message is a message that can be sent over the network.
type Message interface {
	Type() uint32
	CopyAsInterface() interface{}
	MarshalBinary() ([]byte, error)
	UnmarshalBinary(data []byte) error
}

// Client is a client for making requests
type Client struct {
	transport transport.Transport
}

// NewClient creates a new client
func NewClient(t transport.Transport) *Client {
	return &Client{transport: t}
}

// Request sends a request and returns the response
func (c *Client) Request(ctx context.Context, req interfaces.Message) (interfaces.Message, error) {
	var resp []interfaces.Message
	err := c.transport.RoundTrip(ctx, []interfaces.Message{req}, func(msg interfaces.Message) error {
		resp = append(resp, msg)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(resp) == 0 {
		return nil, fmt.Errorf("no response received")
	}
	if len(resp) > 1 {
		return nil, fmt.Errorf("multiple responses received")
	}

	return resp[0], nil
}

// AddressedClient is a client for making addressed requests
type AddressedClient struct {
	*Client
	adapter *interfaces.SequencerAdapter
}

// NewAddressedClient creates a new addressed client
func NewAddressedClient(transport transport.Transport, url *url.URL) *AddressedClient {
	c := &AddressedClient{Client: NewClient(transport)}
	c.adapter = interfaces.NewSequencerAdapter(c, url)
	return c
}

// GetURL returns the URL associated with the client
func (c *AddressedClient) GetURL() *url.URL {
	return c.adapter.GetURL()
}

// Handle implements interfaces.RequestHandler
func (c *AddressedClient) Handle(ctx context.Context, req interfaces.Message) (interfaces.Message, error) {
	return c.Request(ctx, req)
}

// RequestSequence requests a sequence of messages
func (c *AddressedClient) RequestSequence(ctx context.Context, options *model.SequenceOptions) (*model.MessageRecord, error) {
	return c.adapter.RequestSequence(ctx, options)
}
