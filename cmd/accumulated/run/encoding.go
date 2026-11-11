// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"unsafe"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/core/schema/pkg/widget"
)

// wMultiaddr is the schema library's own value widget over [maddr], so this
// package declares no MarshalJSON or UnmarshalJSON with the widget signature
// (go vet's stdmethods check rejects that signature, and [widget.Widget] is
// defined in another module so the methods cannot be renamed).
func wMultiaddr() widget.Widget[*multiaddr.Multiaddr] {
	return widget.ForValue(func(v *multiaddr.Multiaddr) *maddr { return (*maddr)(unsafe.Pointer(v)) })
}

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

// maddr is a struct holding a [multiaddr.Multiaddr], with the methods
// [widget.Value] needs. It has the same memory layout as the interface it
// wraps, which lets a *multiaddr.Multiaddr field be viewed as a *maddr. A nil
// multiaddr is the zero value. Binary encoding is not supported.
type maddr struct{ multiaddr.Multiaddr }

func (m maddr) Copy() maddr        { return m }
func (m maddr) Equal(n maddr) bool { return m.Multiaddr.Equal(n.Multiaddr) }
func (m maddr) IsNil() bool        { return m.Multiaddr == nil }
func (m maddr) Empty() bool        { return m.Multiaddr == nil }

func (m maddr) MarshalJSON() ([]byte, error) { return m.Multiaddr.MarshalJSON() }

func (m *maddr) UnmarshalJSON(b []byte) error {
	a := multiaddr.StringCast("/tcp/0")
	if err := a.UnmarshalJSON(b); err != nil {
		return err
	}
	m.Multiaddr = a
	return nil
}

func (m maddr) MarshalBinary() ([]byte, error) { panic("not supported") }
func (m *maddr) UnmarshalBinary([]byte) error  { panic("not supported") }
