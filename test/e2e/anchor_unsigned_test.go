// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
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

// A block anchor carrying a collection proof but no signature is refused
// whatever the proof says (#4572). The signature is what names the anchor
// and what the anchor pool records under; a proven anchor with none would
// execute and write state under acc://unknown, so validation refuses it
// before any of that.
func TestUnsignedProofBlockAnchorIsRefused(t *testing.T) {
	// Kourou, so the proof is admitted to the check at all
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime).WithVersion(ExecutorVersionV2Kourou),
	)

	txn := &Transaction{
		Header: TransactionHeader{Principal: PartitionUrl("BVN0").JoinPath(AnchorPool)},
		Body:   &DirectoryAnchor{},
	}

	// A proof over the anchor's canonical stored form (the body alone), good
	// on its own terms: the receipt list validates and includes the anchor.
	stored := &Transaction{Body: txn.Body}
	h := stored.GetHash()
	anchor := &messaging.BlockAnchor{
		Anchor: &messaging.SequencedMessage{
			Message:     &messaging.TransactionMessage{Transaction: txn},
			Source:      DnUrl(),
			Destination: PartitionUrl("BVN0"),
			Number:      1,
		},
		Proof: &AnnotatedReceipt{
			Anchor: &AnchorMetadata{
				ChainMetadata: ChainMetadata{Name: "anchor-sequence"},
				Account:       DnUrl().JoinPath(AnchorPool),
				SourceBlock:   1,
			},
			ReceiptList: &merkle.ReceiptList{
				MerkleState: new(merkle.State),
				Elements:    [][]byte{h},
				Receipt:     &merkle.Receipt{Start: h, Anchor: h},
			},
		},
	}
	require.True(t, anchor.Proof.ReceiptList.Validate(nil))
	require.True(t, anchor.Proof.ReceiptList.Included(h))

	statuses, err := sim.SubmitTo("BVN0", &messaging.Envelope{Messages: []messaging.Message{anchor}})
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.Error(t, statuses[0].AsError())
	require.Contains(t, statuses[0].AsError().Error(), "missing signature")
}
