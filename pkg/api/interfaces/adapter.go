// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package interfaces

import (
	"context"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// RequestHandler defines a generic interface for handling API requests
type RequestHandler interface {
	Request(ctx context.Context, req interface{}, resp interface{}) error
}

// SequencerAdapter adapts a RequestHandler to implement the Sequencer interface
type SequencerAdapter struct {
	Handler RequestHandler
}

// Sequence implements the Sequencer interface by forwarding the request to the Handler
func (s *SequencerAdapter) Sequence(ctx context.Context, src, dst *url.URL, num uint64, opts interface{}) (*interface{}, error) {
	req := &struct {
		Source          *url.URL
		Destination     *url.URL
		SequenceNumber  uint64
		SequenceOptions interface{}
	}{
		Source:          src,
		Destination:     dst,
		SequenceNumber:  num,
		SequenceOptions: opts,
	}
	
	resp := new(struct {
		Record *interface{}
	})
	err := s.Handler.Request(ctx, req, resp)
	if err != nil {
		return nil, err
	}
	
	return resp.Record, nil
}
