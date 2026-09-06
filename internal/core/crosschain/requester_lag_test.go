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
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A requester for tests: the partition it asks for, and nothing else.
func testConductor() *Conductor {
	return &Conductor{Partition: &protocol.PartitionInfo{ID: "BVN0", Type: protocol.PartitionTypeBlockValidator}}
}

// A destination whose Directory anchors run late holds every package's
// entries collected with the package's proof waiting in anchor staging. That
// is not a gap: the proof has arrived, the anchor is on its way, and asking
// the source again lands the entries twice (#4229; run 20260905T134346Z).
// Over a 30-block delay the requester makes no request. Once the proof is
// dropped, disproved, the entries are unproven and are asked for.
func TestRequester_LaggingDestination(t *testing.T) {
	const entries = 40
	s := execute.NewStaging()
	tx := s.Begin()
	list := merkle.NewReceiptList()
	list.MerkleState = &merkle.State{Count: 0}
	for n := uint64(1); n <= entries; n++ {
		h := reqHeld(n, true)
		tx.Hold(reqStream, n, h)
		list.Elements = append(list.Elements, h.Hash[:])
	}
	tx.StageProof(reqSource, 50, &protocol.AnnotatedReceipt{Anchor: &protocol.AnchorMetadata{SourceBlock: 50}, ReceiptList: list})
	tx.Commit()

	c := testConductor()
	asks := 0
	ask := streamAsk{stream: reqStream, what: "synthetics", healed: func(int) {},
		ask: func(first, last uint64) (int, uint64, error) { asks++; return int(last - first + 1), last, nil }}
	staged := s.Begin()
	defer staged.Discard()
	for block := uint64(healCadence); block <= 30; block += healCadence {
		c.requestStream(context.Background(), staged, block, reqSource, ask)
	}
	require.Zero(t, asks, "entries whose proof is waiting for its anchor are not gaps")

	// The anchor disproves the proof: now the entries are held unproven
	staged.DropProofs(reqSource, 50)
	c.requestStream(context.Background(), staged, 32, reqSource, ask)
	require.Equal(t, 1, asks, "one span for the whole run, once the proof is gone")
}

// A quiet stream is probed — the span above Delivered asked for whole — and
// the source answers "not yet". The probe is remembered like an answer, so it
// fires once per patience window, not every activation (#4229).
func TestRequester_ProbeOncePerPatience(t *testing.T) {
	s := execute.NewStaging()
	c := testConductor()
	asks := 0
	ask := streamAsk{stream: reqStream, what: "synthetics", healed: func(int) {},
		ask: func(first, last uint64) (int, uint64, error) {
			asks++
			return 0, 0, errors.NotReady.With("not produced yet")
		}}
	staged := s.Begin()
	defer staged.Discard()
	const activations = 12
	for i := uint64(1); i <= activations; i++ {
		c.requestStream(context.Background(), staged, i*healCadence, reqSource, ask)
	}
	require.Equal(t, activations/healPatience, asks, "once per patience window")
	require.False(t, c.requester.backedOff(reqSource, activations*healCadence), "not yet is not a failure")
}

// fakeRanger answers a span request with fixed records.
type fakeRanger struct {
	records []*api.MessageRecord[messaging.Message]
}

func (f fakeRanger) Sequence(context.Context, *url.URL, *url.URL, uint64, private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	return nil, errors.NotFound
}

func (f fakeRanger) SequenceRange(context.Context, *url.URL, *url.URL, uint64, uint64, private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	return f.records, nil
}

// An anchor's signatures travel in one envelope, and anchors share it under
// the budget: two anchors with three signatures each are one envelope of six
// BlockAnchors, not six envelopes (#4229).
func TestRequestAnchorSpan_OneEnvelope(t *testing.T) {
	c := testConductor()
	var envelopes []*messaging.Envelope
	c.Intercept = func(_ context.Context, env *messaging.Envelope) (bool, error) {
		envelopes = append(envelopes, env)
		return false, nil
	}
	var records []*api.MessageRecord[messaging.Message]
	for n := uint64(1); n <= 2; n++ {
		txn := &protocol.Transaction{Header: protocol.TransactionHeader{Principal: protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)},
			Body: &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: protocol.DnUrl(), MinorBlockIndex: n}}}
		r := &api.MessageRecord[messaging.Message]{
			Sequence:   &messaging.SequencedMessage{Message: &messaging.TransactionMessage{Transaction: txn}, Source: protocol.DnUrl(), Destination: protocol.PartitionUrl("BVN0"), Number: n},
			Signatures: &api.RecordRange[*api.SignatureSetRecord]{},
		}
		for i := byte(0); i < 3; i++ {
			sig := &protocol.ED25519Signature{PublicKey: append(make([]byte, 31), i+1), Signer: protocol.DnUrl().JoinPath(protocol.Network), TransactionHash: txn.ID().Hash()}
			sm := &messaging.SignatureMessage{Signature: sig, TxID: txn.ID()}
			r.Signatures.Records = append(r.Signatures.Records, &api.SignatureSetRecord{
				Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{Records: []*api.MessageRecord[messaging.Message]{{Message: sm}}},
			})
		}
		records = append(records, r)
	}
	n, served, err := c.requestAnchorSpan(context.Background(), fakeRanger{records}, protocol.DnUrl(), 1, 2)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, uint64(2), served)
	require.Len(t, envelopes, 1, "one envelope for the span")
	require.Len(t, envelopes[0].Messages, 6, "every signature of every anchor")
	for _, m := range envelopes[0].Messages {
		require.IsType(t, &messaging.BlockAnchor{}, m)
	}
}
