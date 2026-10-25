// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A block anchor's ID does not depend on its signature (#4572): validation
// names a refused, unsigned anchor for its status.
func TestBlockAnchor_IDWithoutSignature(t *testing.T) {
	anchor := &messaging.BlockAnchor{
		Anchor: &messaging.SequencedMessage{
			Message: &messaging.TransactionMessage{Transaction: &protocol.Transaction{
				Header: protocol.TransactionHeader{Principal: protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)},
				Body:   &protocol.DirectoryAnchor{},
			}},
			Source:      protocol.DnUrl(),
			Destination: protocol.PartitionUrl("BVN0"),
			Number:      1,
		},
	}
	require.NotPanics(t, func() {
		id := anchor.ID()
		require.NotNil(t, id)
		require.Equal(t, anchor.Hash(), id.Hash())
	})
	require.NotPanics(t, func() { _ = anchor.OldID() })
}
