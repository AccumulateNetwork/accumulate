// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package interfaces

import (
	"context"
	"fmt"
	"io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/model"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// RequestHandler defines the interface for handling requests
type RequestHandler interface {
	// Handle processes a request and returns a response
	Handle(ctx context.Context, req Message) (Message, error)
}

// MessageAdapter adapts message types to implement the Message interface
type MessageAdapter struct {
	MessageType         uint32
	Value              interface{}
	MarshalFunc        func() ([]byte, error)
	UnmarshalFunc      func([]byte) error
	UnmarshalFromFunc  func(io.Reader) error
}

func (m *MessageAdapter) Type() uint32                        { return m.MessageType }
func (m *MessageAdapter) CopyAsInterface() interface{}        { return m.Value }
func (m *MessageAdapter) MarshalBinary() ([]byte, error)     { return m.MarshalFunc() }
func (m *MessageAdapter) UnmarshalBinary(data []byte) error  { return m.UnmarshalFunc(data) }
func (m *MessageAdapter) UnmarshalBinaryFrom(r io.Reader) error {
	if m.UnmarshalFromFunc != nil {
		return m.UnmarshalFromFunc(r)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return m.UnmarshalBinary(data)
}

// SequencerAdapter adapts the request handler interface for sequence operations
type SequencerAdapter struct {
	handler RequestHandler
	url     *url.URL
}

// NewSequencerAdapter creates a new sequencer adapter
func NewSequencerAdapter(handler RequestHandler, url *url.URL) *SequencerAdapter {
	return &SequencerAdapter{handler: handler, url: url}
}

// RequestSequence requests a sequence of messages
func (a *SequencerAdapter) RequestSequence(ctx context.Context, options *model.SequenceOptions) (*model.MessageRecord, error) {
	req := &MessageAdapter{
		MessageType:  TypePrivateSequenceRequest,
		Value:       &PrivateSequenceRequest{SequenceOptions: *options},
		MarshalFunc: func() ([]byte, error) {
			return options.MarshalBinary()
		},
		UnmarshalFunc: func(data []byte) error {
			return options.UnmarshalBinary(data)
		},
	}

	resp, err := a.handler.Handle(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sequence request failed: %w", err)
	}

	seqResp, ok := resp.(*PrivateSequenceResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}

	return seqResp.Record, nil
}

// GetURL returns the URL associated with the adapter
func (a *SequencerAdapter) GetURL() *url.URL {
	return a.url
}
