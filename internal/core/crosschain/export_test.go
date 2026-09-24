// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// SelectionAt is what the conductor's block hook decides about the pull pair
// from the store as batch sees it: the seed it draws from, and whether this
// node is selected. It loads the ledger the way willBeginBlock does.
func (c *Conductor) SelectionAt(batch *database.Batch) (seed []byte, selected bool, err error) {
	var ledger *protocol.SystemLedger
	err = batch.Account(c.Url(protocol.Ledger)).Main().GetAs(&ledger)
	if err != nil {
		return nil, false, err
	}
	seed = c.previousBlockSeed(batch, ledger)
	return seed, c.selectedToPull(seed), nil
}

// HealAnchorSpan runs the anchor healing pull for [first, last] from source
// against ranger — the production gather and envelope building — and hands
// each envelope to sink instead of the dispatcher.
func (c *Conductor) HealAnchorSpan(ctx context.Context, ranger private.SequenceRanger, source *url.URL, first, last uint64, sink func(*messaging.Envelope)) (int, uint64, error) {
	saved := c.Intercept
	defer func() { c.Intercept = saved }()
	c.Intercept = func(_ context.Context, env *messaging.Envelope) (bool, error) {
		sink(env)
		return false, nil
	}
	return c.requestAnchorSpan(ctx, ranger, source, first, last, func(uint64) string { return "applied" })
}
