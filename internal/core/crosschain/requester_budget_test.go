// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	dagconfig "gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// Every envelope a healed anchor span is submitted in fits the destination's
// batch limit, on the wire, whatever the anchor's size: an anchor whose
// signatures do not fit one envelope has them spread over several. The first
// Directory anchor after a whole-network restart carried 126 receipts, 245
// KB; its signatures travelled together, nine of them a 2.2 MB envelope
// against a limit of 512 KB; the destination refused it, the requester asked
// for it again, for ever, and no Directory anchor after it reached any BVN
// (run 20261006T214005Z, #4544).
func TestRequestAnchorSpan_EveryEnvelopeFitsTheBatchLimit(t *testing.T) {
	c := testConductor()
	const signers = 12
	g := new(network.GlobalValues)
	g.Network = &protocol.NetworkDefinition{}
	for i := byte(0); i < signers; i++ {
		g.Network.AddValidator(append(make([]byte, 31), i+1), protocol.Directory, true)
	}
	c.Globals.Store(g)
	var envelopes []*messaging.Envelope
	c.Intercept = func(_ context.Context, env *messaging.Envelope) (bool, error) {
		envelopes = append(envelopes, env)
		return false, nil
	}

	var records []*api.MessageRecord[messaging.Message]
	for n := uint64(1); n <= 9; n++ {
		body := &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: protocol.DnUrl(), MinorBlockIndex: n}}
		receipts := 12
		if n == 1 {
			receipts = 126 // the first anchor after a restart: every BVN anchor that arrived meanwhile
		}
		for p := 0; p < receipts; p++ {
			rcpt := &protocol.PartitionAnchorReceipt{Anchor: &protocol.PartitionAnchor{Source: protocol.PartitionUrl("BVN1"), MinorBlockIndex: n*100 + uint64(p)}, RootChainReceipt: &merkle.Receipt{}}
			for e := 0; e < 28; e++ {
				rcpt.RootChainReceipt.Entries = append(rcpt.RootChainReceipt.Entries, &merkle.ReceiptEntry{Hash: make([]byte, 32)})
			}
			body.Receipts = append(body.Receipts, rcpt)
		}
		txn := &protocol.Transaction{Header: protocol.TransactionHeader{Principal: protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)}, Body: body}
		r := &api.MessageRecord[messaging.Message]{
			Sequence:   &messaging.SequencedMessage{Message: &messaging.TransactionMessage{Transaction: txn}, Source: protocol.DnUrl(), Destination: protocol.PartitionUrl("BVN0"), Number: n},
			Signatures: &api.RecordRange[*api.SignatureSetRecord]{},
		}
		for i := byte(0); i < signers; i++ {
			sig := &protocol.ED25519Signature{PublicKey: append(make([]byte, 31), i+1), Signer: protocol.DnUrl().JoinPath(protocol.Network), TransactionHash: txn.ID().Hash(), Signature: make([]byte, 64)}
			sm := &messaging.SignatureMessage{Signature: sig, TxID: txn.ID()}
			r.Signatures.Records = append(r.Signatures.Records, &api.SignatureSetRecord{
				Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{Records: []*api.MessageRecord[messaging.Message]{{Message: sm}}},
			})
		}
		records = append(records, r)
	}
	one, err := records[0].Sequence.MarshalBinary()
	require.NoError(t, err)
	t.Logf("the first anchor marshals to %d bytes; its %d signatures together are %d", len(one), signers, len(one)*signers)
	require.Greater(t, len(one)*signers, dagconfig.DefaultMaxBatchBytes, "precondition: the first anchor's signatures do not fit one envelope")

	n, served, err := c.requestAnchorSpan(context.Background(), fakeRanger{records}, protocol.DnUrl(), 1, 9, acceptAll)
	require.NoError(t, err)
	require.Equal(t, 9, n)
	require.Equal(t, uint64(9), served)
	require.NotEmpty(t, envelopes)
	total := 0
	for i, env := range envelopes {
		b, err := env.MarshalBinary()
		require.NoError(t, err)
		t.Logf("envelope %d: %d messages, %d bytes on the wire", i, len(env.Messages), len(b))
		require.LessOrEqual(t, len(b), dagconfig.DefaultMaxBatchBytes, "envelope %d of %d is refused by the destination", i, len(envelopes))
		total += len(env.Messages)
	}
	require.Equal(t, 9*signers, total, "every signature of every anchor was sent")
}
