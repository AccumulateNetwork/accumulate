// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// The Directory's answer for one of its anchors carries the signature of
// every validator whose copy it executed itself — the quorum — not only the
// answering node's (soak 20260905T225751Z: copies lost in dispatch, one
// signature per answer from the same node, no quorum ever).
func TestAnchorAnswerCarriesQuorum(t *testing.T) {
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)
	sim.StepN(30)

	ranger, ok := sim.S.Services().Private().(private.SequenceRanger)
	require.True(t, ok)
	records, err := ranger.SequenceRange(context.Background(), protocol.DnUrl().JoinPath(protocol.AnchorPool), protocol.PartitionUrl("BVN0"), 1, 2, private.SequenceOptions{})
	require.NoError(t, err)
	require.Len(t, records, 2)
	threshold := int(globals.ValidatorThreshold(protocol.Directory))
	for _, r := range records {
		signers := map[string]bool{}
		for _, set := range r.Signatures.Records {
			for _, s := range set.Signatures.Records {
				sm := s.Message.(*messaging.SignatureMessage)
				signers[string(sm.Signature.(protocol.KeySignature).GetPublicKey())] = true
			}
		}
		require.GreaterOrEqual(t, len(signers), threshold, "anchor %d: the answer carries the quorum", r.Sequence.Number)
	}
}
