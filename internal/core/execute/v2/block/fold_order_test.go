// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The order a bundle folds its states in is consensus-visible: it is the
// order of State.ReceivedAnchors, so of the receipts in a DirectoryAnchor,
// which is signed and in the block hash. This pins that order to EXECUTION
// order, with hashes chosen to sort the other way, so a change back to
// hash order -- or to anything else -- fails here rather than in a
// partition that can no longer agree on a block (#4279 review; executor
// spec, "Versioning").
func TestFoldOrder_IsExecutionOrderNotHashOrder(t *testing.T) {
	b := &Block{positions: new(positionCache)}
	d := &bundle{Block: b}

	// Executed first, but its hash sorts LAST.
	first := new(chain.ProcessTransactionState)
	first.DidReceiveAnchor("BVN1", &protocol.BlockValidatorAnchor{}, 2754)
	// Executed second, hash sorts FIRST.
	second := new(chain.ProcessTransactionState)
	second.DidReceiveAnchor("BVN1", &protocol.BlockValidatorAnchor{}, 2755)
	third := new(chain.ProcessTransactionState)
	third.DidReceiveAnchor("BVN1", &protocol.BlockValidatorAnchor{}, 2756)

	d.state.Set([32]byte{0xff}, first)
	d.state.Set([32]byte{0x00}, second)
	d.state.Set([32]byte{0x80}, third)
	d.mergeIntoBlock()

	var got []int64
	for _, r := range b.State.ReceivedAnchors {
		got = append(got, r.Index)
	}
	require.Equal(t, []int64{2754, 2755, 2756}, got,
		"receipts fold in the order the anchors executed; hash order was [2755, 2756, 2754]")
}
