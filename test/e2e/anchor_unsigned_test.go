// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// An unsigned block anchor is refused, not fatal (#4572): validation
// refused it, then filled the status with the message's ID, which read the
// signer off the nil signature and panicked. Any peer could send one.
func TestUnsignedBlockAnchorIsRefused(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	anchor := &messaging.BlockAnchor{
		Anchor: &messaging.SequencedMessage{
			Message: &messaging.TransactionMessage{Transaction: &Transaction{
				Header: TransactionHeader{Principal: PartitionUrl("BVN0").JoinPath(AnchorPool)},
				Body:   &DirectoryAnchor{},
			}},
			Source:      DnUrl(),
			Destination: PartitionUrl("BVN0"),
			Number:      1,
		},
	}
	statuses, err := sim.SubmitTo("BVN0", &messaging.Envelope{Messages: []messaging.Message{anchor}})
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.Error(t, statuses[0].AsError())
	require.Contains(t, statuses[0].AsError().Error(), "missing signature")
}
