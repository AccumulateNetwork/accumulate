// Copyright 2025 The Accumulate Authors
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
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type ConnectionRequest struct {
	Service  *api.ServiceAddress
	PeerID   peer.ID
	PeerAddr multiaddr.Multiaddr
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

	// Use the batch's context to avoid closing the stream early
	if batch := api.GetBatchData(ctx); batch != nil {
		ctx = batch.Context()
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
	return m, streamError(err, s.peer)
}

func (s *stream) Write(msg message.Message) error {
	err := s.stream.Write(msg)
	return streamError(err, s.peer)
}

// streamError converts network reset and canceled context errors into
// [io.EOF].
func streamError(err error, peer peer.ID) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, io.EOF),
		errors.Is(err, network.ErrReset),
		errors.Is(err, context.Canceled):
		return &peerError{io.EOF, peer}
	default:
		return &peerError{err, peer}
	}
}

// peerError attaches a peer ID to an error, to support better tracking and
// logging.
type peerError struct {
	err  error
	peer peer.ID
}

func (p *peerError) Peer() peer.ID { return p.peer }
func (p *peerError) Error() string { return p.err.Error() }
func (p *peerError) Unwrap() error { return p.err }
