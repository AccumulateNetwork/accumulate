// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"io"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// stream is a [message.Stream] with an associated [peerState].
type stream struct {
	peer   peer.ID
	conn   io.ReadWriteCloser
	stream message.Stream
}

func openStreamFor(ctx context.Context, host dialerHost, peer peer.ID, sa *api.ServiceAddress) (*stream, error) {
	conn, err := host.getPeerService(ctx, peer, sa)
	if err != nil {
		// Do not wrap as it will clobber the error
		return nil, err
	}

	// Close the stream when the context is canceled
	go func() { <-ctx.Done(); _ = conn.Close() }()

	s := new(stream)
	s.peer = peer
	s.conn = conn
	s.stream = message.NewStream(conn)
	return s, nil
}

func (s *stream) Read() (message.Message, error) {
	// Convert ErrReset and Canceled into EOF
	m, err := s.stream.Read()
	switch {
	case err == nil:
		return m, nil
	case isEOF(err):
		return nil, io.EOF
	default:
		return nil, err
	}
}

func (s *stream) Write(msg message.Message) error {
	// Convert ErrReset and Canceled into EOF
	err := s.stream.Write(msg)
	switch {
	case err == nil:
		return nil
	case isEOF(err):
		return io.EOF
	default:
		return err
	}
}

func isEOF(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, network.ErrReset) ||
		errors.Is(err, context.Canceled)
}
