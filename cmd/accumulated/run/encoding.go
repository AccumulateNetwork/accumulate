// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
	"gitlab.com/accumulatenetwork/core/schema/pkg/json"
)

var wMultiaddr = multiaddrWidget{}
var wPeerID = peerIdWidget{}

type multiaddrWidget struct{}

func (multiaddrWidget) IsNil(v *multiaddr.Multiaddr) bool                         { return *v == nil }
func (multiaddrWidget) Empty(v *multiaddr.Multiaddr) bool                         { return *v == nil }
func (multiaddrWidget) CopyTo(dst, src *multiaddr.Multiaddr)                      { *dst = *src }
func (multiaddrWidget) Equal(a, b *multiaddr.Multiaddr) bool                      { return (*a).Equal(*b) }
func (multiaddrWidget) MarshalJSON(e *json.Encoder, v *multiaddr.Multiaddr) error { return e.Encode(v) }

func (multiaddrWidget) UnmarshalJSON(d *json.Decoder, v *multiaddr.Multiaddr) error {
	m := multiaddr.StringCast("/tcp/0")
	*v = m
	return d.Decode(m)
}

func (multiaddrWidget) MarshalBinary(e *binary.Encoder, v *multiaddr.Multiaddr) error {
	panic("not supported")
}

func (multiaddrWidget) UnmarshalBinary(d *binary.Decoder, v *multiaddr.Multiaddr) error {
	panic("not supported")
}

type peerIdWidget struct{}

func (peerIdWidget) IsNil(v *peer.ID) bool                           { return false }
func (peerIdWidget) Empty(v *peer.ID) bool                           { return *v == "" }
func (peerIdWidget) CopyTo(dst, src *peer.ID)                        { *dst = *src }
func (peerIdWidget) Equal(a, b *peer.ID) bool                        { return *a == *b }
func (peerIdWidget) MarshalJSON(e *json.Encoder, v *peer.ID) error   { return e.Encode(v) }
func (peerIdWidget) UnmarshalJSON(d *json.Decoder, v *peer.ID) error { return d.Decode(v) }

func (peerIdWidget) MarshalBinary(e *binary.Encoder, v *peer.ID) error {
	panic("not supported")
}

func (peerIdWidget) UnmarshalBinary(d *binary.Decoder, v *peer.ID) error {
	panic("not supported")
}
