// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// isAdmissible answers "is this proven to have come from its source" for both
// the executor and staging (#4169 step 3). Staging needs it because a stream
// must not advance past a message that did not execute, and an unproven
// message does not execute.

// admissibleFixture returns a block and a hash that IS in the directory anchor
// chain.
func admissibleFixture(t *testing.T) (*Executor, *database.Batch, []byte) {
	t.Helper()
	x := streamTestExec(t)
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	known := make([]byte, 32)
	known[0] = 0xAB
	chain, err := batch.Account(x.Describe.AnchorPool()).
		AnchorChain(protocol.Directory).Root().Get()
	require.NoError(t, err)
	require.NoError(t, chain.AddEntry(known, false))

	return x, batch, known
}

func TestIsAdmissible_AnchorIsHeld(t *testing.T) {
	x, batch, known := admissibleFixture(t)

	ok, err := x.isAdmissible(batch, &protocol.AnnotatedReceipt{
		Receipt: &merkle.Receipt{Anchor: known},
	})
	require.NoError(t, err)
	assert.True(t, ok, "the proof terminates at an anchor we hold")
}

func TestIsAdmissible_AnchorHasNotArrived(t *testing.T) {
	x, batch, _ := admissibleFixture(t)

	missing := make([]byte, 32)
	missing[0] = 0xCD
	ok, err := x.isAdmissible(batch, &protocol.AnnotatedReceipt{
		Receipt: &merkle.Receipt{Anchor: missing},
	})
	require.NoError(t, err, "not-yet-anchored is an ANSWER, not an error — both callers have to act on it")
	assert.False(t, ok)
}

// A replica-accepted message (#4140) carries no proof of its own. Its proof was
// checked and absorbed into the stream's replica when it first arrived.
func TestIsAdmissible_NoProofIsReplicaAccepted(t *testing.T) {
	x, batch, _ := admissibleFixture(t)

	ok, err := x.isAdmissible(batch, nil)
	require.NoError(t, err)
	assert.True(t, ok, "no proof means replica-accepted, which is admissible by construction")
}

// A collection proof terminates at the same trust root as an individual one,
// so one check covers either form — including the continued-receipt case,
// where the terminal anchor is NOT the list's own receipt.
func TestIsAdmissible_CollectionProof(t *testing.T) {
	x, batch, known := admissibleFixture(t)

	ok, err := x.isAdmissible(batch, &protocol.AnnotatedReceipt{
		ReceiptList: &merkle.ReceiptList{Receipt: &merkle.Receipt{Anchor: known}},
	})
	require.NoError(t, err)
	assert.True(t, ok, "a collection proof terminates at the same trust root")

	other := make([]byte, 32)
	other[0] = 0xEF
	ok, err = x.isAdmissible(batch, &protocol.AnnotatedReceipt{
		ReceiptList: &merkle.ReceiptList{
			Receipt:          &merkle.Receipt{Anchor: other},
			ContinuedReceipt: &merkle.Receipt{Anchor: known},
		},
	})
	require.NoError(t, err)
	assert.True(t, ok, "when the list is continued, the CONTINUED receipt's anchor is the terminal one")
}
