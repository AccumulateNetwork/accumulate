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
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
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
	// Hold back the Directory's anchors to BVN0 so BVN0 never says it
	// executed them. The cache releases an anchor once every destination has,
	// and from Kourou the heartbeat keeps the cascade running, so on a
	// network where delivery works the early anchors are acknowledged and let
	// go before anything can ask for them (#4277). A destination that has not
	// acknowledged is also the only case healing exists for, so this is the
	// scenario the answer is supposed to serve.
	opts := []simulator.Option{
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.GenesisWith(GenesisTime, globals),
	}
	opts = append(opts, simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (bool, error) {
		for _, m := range env.Messages {
			blk, ok := m.(*messaging.BlockAnchor)
			if !ok {
				continue
			}
			seq, ok := blk.Anchor.(*messaging.SequencedMessage)
			if !ok {
				continue
			}
			if seq.Destination != nil && protocol.PartitionUrl("BVN0").Equal(seq.Destination) &&
				seq.Source != nil && protocol.DnUrl().Equal(seq.Source) {
				return false, nil
			}
		}
		return true, nil
	}))
	sim := NewSim(t, opts...)
	sim.StepN(30)

	ranger, ok := sim.S.Services().Private().(private.SequenceRanger)
	require.True(t, ok)
	// Ask for a pair the cache still holds rather than anchors 1 and 2. The
	// cache releases an anchor once every destination has said it executed
	// through it, and from Kourou the heartbeat keeps the cascade running, so
	// the early anchors are answered, acknowledged and let go well before
	// this point (#4277). Which pair is served is not what this test is
	// about; that the answer carries the quorum is.
	var records []*api.MessageRecord[messaging.Message]
	var err error
	for start := uint64(1); start < 40; start++ {
		records, err = ranger.SequenceRange(context.Background(), protocol.DnUrl().JoinPath(protocol.AnchorPool), protocol.PartitionUrl("BVN0"), start, start+1, private.SequenceOptions{})
		if err == nil && len(records) == 2 {
			break
		}
		records = nil
	}
	require.NotNil(t, records, "no pair of anchors is both produced and still held")
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
