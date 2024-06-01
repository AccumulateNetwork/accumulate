// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dial

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

var FakeTracker fakeTracker

type fakeTracker struct{}

var _ Tracker = fakeTracker{}

func (fakeTracker) Mark(peer.ID, multiaddr.Multiaddr, api.KnownPeerStatus)        {}
func (fakeTracker) Status(peer.ID, multiaddr.Multiaddr) api.KnownPeerStatus       { return 0 }
func (fakeTracker) Next(multiaddr.Multiaddr, api.KnownPeerStatus) (peer.ID, bool) { return "", false }
func (fakeTracker) All(multiaddr.Multiaddr, api.KnownPeerStatus) []peer.ID        { return nil }
