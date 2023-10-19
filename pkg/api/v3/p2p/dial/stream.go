// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dial

import (
	"context"
	"io"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type ConnectionRequest struct {
	Service *api.ServiceAddress
	PeerID  peer.ID
}

type Connector interface {
	Connect(context.Context, *ConnectionRequest) (message.Stream, error)
}

type stream struct {
	peer   peer.ID
	stream message.Stream
}

func openStreamFor(ctx context.Context, host Connector, req *ConnectionRequest) (*stream, error) {
	if req.PeerID == "" {
		return nil, errors.BadRequest.With("missing peer ID")
	}
	if req.Service == nil {
		return nil, errors.BadRequest.With("missing service address")
	}

	conn, err := host.Connect(ctx, req)
	if err != nil {
		// Do not wrap as it will clobber the error
		return nil, err
	}

	s := new(stream)
	s.peer = req.PeerID
	s.stream = conn
	return s, nil
}

func (s *stream) Read() (message.Message, error) {
	m, err := s.stream.Read()
	return m, streamError(err)
}

func (s *stream) Write(msg message.Message) error {
	err := s.stream.Write(msg)
	return streamError(err)
}

// streamError converts network reset and canceled context errors into
// [io.EOF].
func streamError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, io.EOF),
		errors.Is(err, network.ErrReset),
		errors.Is(err, context.Canceled):
		return io.EOF
	default:
		return err
	}
}
