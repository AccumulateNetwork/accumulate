// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// A Dialer dials an address, returning a message stream.
type Dialer interface {
	// Dial returns a stream for the given address. The caller must cancel the
	// context once the stream is no longer needed. The dialer is responsible
	// for cleaning up the stream once the context has been canceled.
	Dial(context.Context, multiaddr.Multiaddr) (Stream, error)
}

// MultiDialer is a dialer that accepts error reports and may be called multiple
// times for one request.
type MultiDialer interface {
	Dialer

	// BadDial is used by Transport to report stream errors. The address will be
	// the same address passed to Dial, and the stream will be the exact value
	// returned from Dial.
	BadDial(context.Context, multiaddr.Multiaddr, Stream, error) bool
}

// BatchDialer returns a dialer that reuses streams. BatchDialer ignores the
// contexts passed to Dial and BadDial and uses the context passed to
// BatchDialer instead. Thus the context must be canceled to close all the
// dialed streams.
//
// If the dialer is a MultiDialer, calls to BadDial will be forwarded.
func BatchDialer(ctx context.Context, dialer Dialer) Dialer {
	b := new(batchDialer)
	b.context = ctx
	b.streams = map[string]Stream{}
	b.dialer = dialer
	b.multi, _ = b.dialer.(MultiDialer)
	return b
}

// See [BatchDialer].
type batchDialer struct {
	context context.Context
	dialer  Dialer
	multi   MultiDialer
	streams map[string]Stream
}

// Dial implements [Dialer.Dial].
func (b *batchDialer) Dial(_ context.Context, addr multiaddr.Multiaddr) (Stream, error) {
	// Must have an address
	if addr == nil {
		return nil, errors.BadRequest.With("cannot batch dial without an address")
	}

	// Check the dictionary
	if s, ok := b.streams[addr.String()]; ok {
		return s, nil
	}

	// Nothing in the dictionary so dial a new stream
	s, err := b.dialer.Dial(b.context, addr)
	if err != nil {
		return nil, err
	}

	// Remember it
	b.streams[addr.String()] = s
	return s, nil
}

// BadDial implements [MultiDialer.BadDial].
func (b *batchDialer) BadDial(_ context.Context, addr multiaddr.Multiaddr, s Stream, err error) bool {
	// Forget the bad stream
	delete(b.streams, addr.String())

	// If the inner dialer is a multi dialer, inform it of the bad stream
	if b.multi == nil {
		return false
	}
	return b.multi.BadDial(b.context, addr, s, err)
}
