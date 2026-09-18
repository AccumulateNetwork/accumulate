// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// APIPeers finds this partition's other validators and reaches each one's
// private API by node, the way the conductor asks a source's validators one
// by one (crosschain, "anchorAnswers"). A joining node must ask a NAMED node,
// not whichever peer the dialer favours: it has to know whose staging it took
// and to move on from one that cannot serve it.
type APIPeers struct {
	Partition string
	Client    *message.Client
}

// Validators lists the nodes serving this partition's sequencer.
func (p *APIPeers) Validators(ctx context.Context) ([]*api.FindServiceResult, error) {
	if p.Client == nil {
		return nil, errors.NotReady.With("no network client")
	}
	return p.Client.FindService(ctx, api.FindServiceOptions{
		Service: private.ServiceTypeSequencer.AddressFor(p.Partition),
	})
}

// Staging is the private API addressed to one node.
func (p *APIPeers) Staging(peer *api.FindServiceResult) private.StagingSnapshotter {
	addr := private.ServiceTypeSequencer.AddressFor(p.Partition).Multiaddr()
	c := p.Client.ForPeer(peer.PeerID).ForAddress(addr).Private()
	s, ok := c.(private.StagingSnapshotter)
	if !ok {
		return refuses{}
	}
	return s
}

// refuses stands for a client that cannot serve staging at all.
type refuses struct{ private.Sequencer }

func (refuses) StagingSnapshot(context.Context, *private.StagingSnapshotRequest) (*private.StagingSnapshot, error) {
	return nil, errors.NotAllowed.With("this client cannot ask for staging")
}
