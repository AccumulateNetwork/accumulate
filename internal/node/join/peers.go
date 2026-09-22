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
// by one (crosschain, "anchorAnswers").
type APIPeers struct {
	Partition string
	Client    *message.Client

	// Network is the network this node belongs to. A service is advertised
	// under its network's key, so a search that does not name one finds
	// nothing (#4296). The node service defaults it, and naming it here
	// keeps a client that does not from searching the wrong key in silence.
	Network string
}

// Validators lists the nodes serving this partition's sequencer.
func (p *APIPeers) Validators(ctx context.Context) ([]*api.FindServiceResult, error) {
	if p.Client == nil {
		return nil, errors.NotReady.With("no network client")
	}
	return p.Client.FindService(ctx, api.FindServiceOptions{
		Network: p.Network,
		Service: private.ServiceTypeSequencer.AddressFor(p.Partition),
	})
}
