// Copyright 2026 The Accumulate Authors
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
	"gitlab.com/accumulatenetwork/core/schema/pkg/widget"
)

func wMultiaddr() widget.Widget[*multiaddr.Multiaddr] { return multiaddrWidget{} }

// wPeerID is the schema library's own value widget over [peerID], so this
// package declares no MarshalJSON or UnmarshalJSON with the widget
// signature. Copies, comparisons and both encodings are peer.ID's own.
func wPeerID() widget.Widget[*peer.ID] {
	return widget.ForValue(func(v *peer.ID) *peerID { return (*peerID)(v) })
}

// peerID is peer.ID with the Copy and Equal methods [widget.Value] needs.
// Its encodings are peer.ID's, so the JSON is the base58 string and the
// binary is the multihash.
type peerID string

func (p peerID) Copy() peerID                    { return p }
func (p peerID) Equal(q peerID) bool             { return p == q }
func (p peerID) IsNil() bool                     { return false }
func (p peerID) Empty() bool                     { return p == "" }
func (p peerID) MarshalJSON() ([]byte, error)    { return peer.ID(p).MarshalJSON() }
func (p *peerID) UnmarshalJSON(b []byte) error   { return (*peer.ID)(p).UnmarshalJSON(b) }
func (p peerID) MarshalBinary() ([]byte, error)  { return peer.ID(p).MarshalBinary() }
func (p *peerID) UnmarshalBinary(b []byte) error { return (*peer.ID)(p).UnmarshalBinary(b) }

// multiaddrWidget cannot be a [widget.Value]: multiaddr.Multiaddr is an
// interface, so no defined type with a Copy method can alias the field. The
// method names below are dictated by [widget.Widget]; go vet's stdmethods
// check reports MarshalJSON and UnmarshalJSON for that reason.
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
