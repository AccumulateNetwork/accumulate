// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
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
